package main

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Dati live letti da una pagina web qualsiasi (es. la pagina meteo di un
// rifugio): la pagina viene ridotta a testo e il valore si estrae con
// un'espressione regolare costruita dal pannello ("Cerca i valori nella pagina").

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</\s*(script|style|noscript)\s*>`)
	reTags        = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpaces      = regexp.MustCompile(`\s+`)
	// numero seguito da un'unità di misura tipica dei dati meteo
	reMeasure = regexp.MustCompile(`(-?\d{1,4}(?:[.,]\d{1,2})?)\s*(°\s?[CF]|°|%|km/h|kmh|m/s|nodi|hPa|mbar|mb|mm/h|mm|W/m²|W/m2|UV)`)
)

// pageText riduce l'HTML al solo testo visibile, su una riga.
func pageText(body []byte) string {
	s := reScriptStyle.ReplaceAllString(string(body), " ")
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

// FoundValue è un valore candidato trovato nella pagina.
type FoundValue struct {
	Value   string `json:"value"`
	Unit    string `json:"unit"`
	Context string `json:"context"` // testo che precede il valore
	Pattern string `json:"pattern"` // espressione per rileggerlo
}

// FindPageValues elenca i valori con unità di misura presenti nella pagina.
func FindPageValues(ctx context.Context, rawURL string) ([]FoundValue, int, error) {
	body, err := getBody(ctx, rawURL, "")
	if err != nil {
		return nil, 0, err
	}
	text := pageText(body)
	var out []FoundValue
	seen := map[string]bool{}
	for _, m := range reMeasure.FindAllStringSubmatchIndex(text, 200) {
		value, unit := text[m[2]:m[3]], strings.ReplaceAll(text[m[4]:m[5]], " ", "")
		start := max(0, m[0]-60)
		before := strings.TrimSpace(text[start:m[0]])
		// l'etichetta sono le parole subito prima del numero (es. "Temperatura aria")
		words := labelWords(strings.Fields(before))
		label := strings.Join(words, " ")
		pattern := buildPattern(words, text[m[4]:m[5]])
		key := label + "|" + value + unit
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, FoundValue{Value: value, Unit: unit, Context: label, Pattern: pattern})
	}
	return out, len(text), nil
}

var reUnitWord = regexp.MustCompile(`(?i)^(°c?|°f|%|km/h|kmh|m/s|nodi|hpa|mbar|mb|mm/h|mm|w/m²|w/m2|uv|m)$`)

// labelWords tiene le ultime parole (al massimo 4) prima del valore, fermandosi
// al valore precedente: numeri e unità di misura non fanno parte dell'etichetta.
func labelWords(words []string) []string {
	var out []string
	for i := len(words) - 1; i >= 0 && len(out) < 4; i-- {
		w := strings.Trim(words[i], ":=|-–")
		if w == "" {
			continue
		}
		if strings.ContainsAny(w, "0123456789") || reUnitWord.MatchString(w) {
			break
		}
		out = append([]string{w}, out...)
	}
	// se c'è una parola meteo nota, l'etichetta parte da lì (toglie titoli e nomi di luogo)
	for i := len(out) - 1; i >= 0; i-- {
		if reMeteoWord.MatchString(out[i]) {
			return out[i:]
		}
	}
	return out
}

var reMeteoWord = regexp.MustCompile(`(?i)^(temp|umid|vento|raffic|press|piogg|precip|neve|rugiad|radiaz|uv|humid|wind|rain|dew|altezza)`)

// buildPattern crea un'espressione che trova il numero dopo l'etichetta
// (le ultime 1-2 parole che non sono numeri), così il valore viene riletto
// anche quando cambia.
func buildPattern(words []string, unit string) string {
	var label []string
	for i := len(words) - 1; i >= 0 && len(label) < 2; i-- {
		label = append([]string{regexp.QuoteMeta(words[i])}, label...)
	}
	u := regexp.QuoteMeta(strings.ReplaceAll(unit, " ", ""))
	u = strings.ReplaceAll(u, "°", `°\s?`)
	if len(label) == 0 {
		return `(-?\d+(?:[.,]\d+)?)\s*` + u
	}
	return `(?i)` + strings.Join(label, `\s+`) + `\D{0,40}?(-?\d+(?:[.,]\d+)?)\s*` + u
}

// readPageValue legge il valore dalla pagina con l'espressione salvata.
func readPageValue(ctx context.Context, ds DataSource) (any, error) {
	body, err := getBody(ctx, ds.URL, ds.Token)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(ds.Path)
	if err != nil {
		return nil, fmt.Errorf("espressione non valida: %w", err)
	}
	m := re.FindStringSubmatch(pageText(body))
	if m == nil {
		return nil, fmt.Errorf("valore non trovato nella pagina (la pagina è cambiata?)")
	}
	v := m[0]
	if len(m) > 1 {
		v = m[1]
	}
	if f, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64); err == nil {
		return f, nil
	}
	return v, nil
}
