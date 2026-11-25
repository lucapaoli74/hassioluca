package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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

// FoundValue è un valore candidato trovato nella pagina o nei suoi dati.
type FoundValue struct {
	Value   string `json:"value"`
	Unit    string `json:"unit"`
	Context string `json:"context"` // etichetta (testo prima del valore o nome del campo)
	Pattern string `json:"pattern"` // espressione o percorso JSON per rileggerlo
	Kind    string `json:"kind"`    // "html" (testo), "raw" (sorgente/script), "json"
	Source  string `json:"source"`  // indirizzo da cui leggere
}

// FindPageValues elenca i valori meteo della pagina. Se il testo visibile non
// ne contiene (pagina che carica i dati con JavaScript) cerca anche negli
// script, nei riquadri incorporati e nei file di dati collegati.
// PageScan è il risultato della ricerca, con le informazioni per capire cosa
// ha visto il programma quando non trova nulla.
type PageScan struct {
	Values     []FoundValue `json:"values"`
	TextLength int          `json:"text_length"`
	Browser    string       `json:"browser"`     // browser usato ("" = non trovato)
	BrowserErr string       `json:"browser_err"` // errore del browser
	Rendered   bool         `json:"rendered"`    // pagina aperta col browser
	Excerpt    string       `json:"excerpt"`     // testo letto (per l'assistenza)
}

// FindPageValues cerca i valori meteo della pagina. label (facoltativo) è una
// parola scelta dall'utente: si prende il numero che la segue.
func FindPageValues(ctx context.Context, rawURL, label string) (*PageScan, error) {
	body, err := getBody(ctx, rawURL, "")
	if err != nil {
		return nil, err
	}
	scan := &PageScan{Browser: findBrowser()}
	text := pageText(body)
	scan.TextLength = len(text)
	var out []FoundValue
	if label != "" {
		out = valuesAfterLabel(text, rawURL, label, "html")
	} else {
		out = meaningful(valuesInText(text, rawURL))
		out = append(out, meaningful(valuesInScripts(string(body), rawURL))...)
		out = append(out, valuesAfterMeteoWords(text, rawURL, "html")...)
	}
	rendered := ""
	if !hasTemperature(out) && !(label != "" && len(out) > 0) {
		// la pagina carica i dati con JavaScript: la si apre con il browser
		dom, netURLs, err := renderPage(ctx, rawURL)
		// i file di dati scaricati dalla pagina: si leggono direttamente (più leggero del browser)
		if label == "" {
			out = append(out, meaningful(valuesInURLs(ctx, rawURL, dataLike(rawURL, netURLs)))...)
		}
		if err == nil {
			scan.Rendered = true
			rendered = pageText(dom)
			var bv []FoundValue
			if label != "" {
				bv = valuesAfterLabel(rendered, rawURL, label, "browser")
			} else {
				for _, v := range meaningful(valuesInText(rendered, rawURL)) {
					v.Kind = "browser"
					bv = append(bv, v)
				}
				bv = append(bv, valuesAfterMeteoWords(rendered, rawURL, "browser")...)
			}
			out = append(out, bv...)
		} else {
			scan.BrowserErr = err.Error()
		}
	}
	if label == "" && !hasTemperature(out) {
		out = append(out, meaningful(valuesInLinked(ctx, rawURL, string(body)))...)
	}
	if label == "" && !hasTemperature(out) {
		// indirizzi dei dati scritti dentro i file JavaScript del sito
		out = append(out, meaningful(valuesInURLs(ctx, rawURL, dataURLsInScripts(ctx, rawURL, string(body))))...)
	}
	sortByImportance(out)
	scan.Values = dedupe(out)
	if scan.Values == nil {
		scan.Values = []FoundValue{}
	}
	src := rendered
	if src == "" {
		src = text
	}
	scan.Excerpt = excerpt(src)
	return scan, nil
}

// excerpt restituisce il testo intorno alle parole meteo (o l'inizio della pagina).
func excerpt(text string) string {
	low := strings.ToLower(text)
	for _, w := range []string{"temperatura", "temp", "°"} {
		if i := strings.Index(low, w); i >= 0 {
			start := max(0, i-600)
			return text[start:min(len(text), i+1400)]
		}
	}
	return text[:min(len(text), 2000)]
}

var reLabelNumber = `\D{0,30}?(-?\d{1,3}(?:[.,]\d{1,2})?)`

// valuesAfterLabel prende i numeri che seguono la parola indicata.
func valuesAfterLabel(text, source, label, kind string) []FoundValue {
	re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(strings.TrimSpace(label)) + reLabelNumber)
	if err != nil {
		return nil
	}
	var out []FoundValue
	for _, m := range re.FindAllStringSubmatch(text, 5) {
		out = append(out, FoundValue{Value: m[1], Unit: unitForKey(label), Context: strings.TrimSpace(label),
			Pattern: re.String(), Kind: kind, Source: source})
	}
	return out
}

// valuesAfterMeteoWords riconosce "Temperatura 3,8" anche senza unità di misura.
var reMeteoLabelNum = regexp.MustCompile(`(?i)\b(temperatura(?:\s+(?:aria|esterna|attuale))?|umidit[aà](?:\s+relativa)?|vento|pressione|pioggia)\b[^0-9\-\p{L}]{0,15}(-?\d{1,4}(?:[.,]\d{1,2})?)`)

func valuesAfterMeteoWords(text, source, kind string) []FoundValue {
	var out []FoundValue
	for _, m := range reMeteoLabelNum.FindAllStringSubmatch(text, 20) {
		lbl := m[1]
		out = append(out, FoundValue{Value: m[2], Unit: unitForKey(lbl), Context: lbl,
			Pattern: `(?i)` + regexp.QuoteMeta(lbl) + reLabelNumber, Kind: kind, Source: source})
	}
	return out
}

func hasTemperature(v []FoundValue) bool {
	for _, f := range v {
		if strings.HasPrefix(f.Unit, "°") && (len(f.Unit) > len("°") || firstMeteoWord(f.Context) != "") {
			return true
		}
	}
	return false
}

func firstMeteoWord(s string) string {
	for _, w := range strings.Fields(s) {
		if reMeteoWord.MatchString(w) {
			return w
		}
	}
	return ""
}

// meaningful scarta i numeri che non sono dati meteo: percentuali e gradi
// senza un'etichetta meteo vicina (es. "-25 %" in un file grafico, "1° incontro").
func meaningful(v []FoundValue) []FoundValue {
	var out []FoundValue
	for _, f := range v {
		label := strings.ToLower(f.Context)
		meteo := firstMeteoWord(f.Context) != "" || reMeteoKey.MatchString(label)
		switch {
		case f.Kind == "json" || f.Kind == "raw":
			// già filtrati per nome del campo
		case f.Unit == "°" && !regexp.MustCompile(`(?i)temp|rugiad|dew`).MatchString(label):
			continue // "1° incontro": un ° da solo vale solo vicino a "Temperatura"
		case strings.HasPrefix(f.Unit, "°") && len(f.Unit) > len("°"):
			// °C / °F: è una temperatura anche senza etichetta
		case !meteo:
			continue
		}
		out = append(out, f)
	}
	return out
}

func dedupe(v []FoundValue) []FoundValue {
	seen := map[string]bool{}
	var out []FoundValue
	for _, f := range v {
		k := f.Context + "|" + f.Value + f.Unit
		if !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	return out
}

func valuesInText(text, source string) []FoundValue {
	var out []FoundValue
	seen := map[string]bool{}
	for _, m := range reMeasure.FindAllStringSubmatchIndex(text, 200) {
		value, unit := text[m[2]:m[3]], strings.ReplaceAll(text[m[4]:m[5]], " ", "")
		start := max(0, m[0]-60)
		before := strings.TrimSpace(text[start:m[0]])
		// l'etichetta sono le parole subito prima del numero (es. "Temperatura aria")
		words := labelWords(strings.Fields(before))
		label := strings.Join(words, " ")
		key := label + "|" + value + unit
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, FoundValue{Value: value, Unit: unit, Context: label, Pattern: buildPattern(words, text[m[4]:m[5]]), Kind: "html", Source: source})
	}
	return out
}

// variabili negli script, es.  temperatura = 3.8  oppure  "temp_c": "3.8"
var reScriptVar = regexp.MustCompile(`(?i)["']?([a-z_]*(?:temp|umid|humid|hum|vento|wind|press|baro|piogg|rain|prec|dew|rugiad|uv|solar|radiaz)[a-z0-9_]*)["']?\s*[:=]\s*["']?(-?\d{1,4}(?:[.,]\d{1,3})?)["']?`)

func valuesInScripts(body, source string) []FoundValue {
	var out []FoundValue
	seen := map[string]bool{}
	for _, m := range reScriptVar.FindAllStringSubmatch(body, 100) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, FoundValue{Value: m[2], Unit: unitForKey(m[1]), Context: m[1],
			Pattern: `["']?` + regexp.QuoteMeta(m[1]) + `["']?\s*[:=]\s*["']?(-?\d+(?:[.,]\d+)?)`, Kind: "raw", Source: source})
	}
	return out
}

func unitForKey(k string) string {
	k = strings.ToLower(k)
	switch {
	case strings.Contains(k, "temp") || strings.Contains(k, "dew") || strings.Contains(k, "rugiad"):
		return "°C"
	case strings.Contains(k, "hum") || strings.Contains(k, "umid"):
		return "%"
	case strings.Contains(k, "dir"):
		return "°"
	case strings.Contains(k, "wind") || strings.Contains(k, "vento") || strings.Contains(k, "gust") || strings.Contains(k, "raffic"):
		return "km/h"
	case strings.Contains(k, "press") || strings.Contains(k, "baro"):
		return "hPa"
	case strings.Contains(k, "rain") || strings.Contains(k, "piogg") || strings.Contains(k, "prec"):
		return "mm"
	case strings.Contains(k, "solar") || strings.Contains(k, "radiaz"):
		return "W/m²"
	}
	return ""
}

var (
	reFrame   = regexp.MustCompile(`(?i)<(?:iframe|frame|embed)[^>]+src=["']([^"']+)["']`)
	reScript  = regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+)["']`)
	reDataURL = regexp.MustCompile(`(?i)["'\(]((?:https?:)?//[^"'\s)]+|/?[\w./-]+)\.(json|txt|xml|csv|php|asp|aspx|cgi|htm|html)(\?[^"'\s)]*)?["'\)]`)
)

// dataLike tiene gli indirizzi che sembrano dati (non immagini, stili, script, font).
func dataLike(page string, urls []string) []string {
	var out []string
	for _, u := range urls {
		l := strings.ToLower(u)
		if u == page || strings.Contains(l, "google") || strings.Contains(l, "facebook") || strings.Contains(l, "cookie") {
			continue
		}
		path := l
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
		skip := false
		for _, ext := range []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".woff", ".woff2", ".ttf", ".mp4", ".map"} {
			if strings.HasSuffix(path, ext) {
				skip = true
			}
		}
		if !skip && len(out) < 25 {
			out = append(out, u)
		}
	}
	return out
}

// dataURLsInScripts cerca, nei file JavaScript dello stesso sito, indirizzi di
// file di dati (json, txt, xml, csv) o di API.
func dataURLsInScripts(ctx context.Context, pageURL, body string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range reScript.FindAllStringSubmatch(body, 20) {
		u, err := base.Parse(m[1])
		l := strings.ToLower(m[1])
		if err != nil || u.Host != base.Host || strings.Contains(l, "jquery") || strings.Contains(l, "bootstrap") {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		js, err := getBody(cctx, u.String(), "")
		cancel()
		if err != nil {
			continue
		}
		for _, d := range reDataURL.FindAllStringSubmatch(string(js), -1) {
			ext := strings.ToLower(d[2])
			lp := strings.ToLower(d[1])
			if ext != "json" && ext != "txt" && ext != "xml" && ext != "csv" && !strings.Contains(lp, "api") && !strings.Contains(lp, "meteo") && !strings.Contains(lp, "realtime") {
				continue
			}
			if r, err := base.Parse(d[1] + "." + d[2] + d[3]); err == nil && !seen[r.String()] && len(out) < 15 {
				seen[r.String()] = true
				out = append(out, r.String())
			}
		}
	}
	return out
}

// valuesInURLs scarica gli indirizzi e cerca i valori (JSON per nome del campo, testo per etichetta).
func valuesInURLs(ctx context.Context, page string, urls []string) []FoundValue {
	var mu sync.Mutex
	var out []FoundValue
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, l := range urls {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			b, err := getBody(cctx, l, "")
			if err != nil {
				return
			}
			var found []FoundValue
			trimmed := strings.TrimSpace(string(b))
			var doc any
			if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Unmarshal(b, &doc) == nil {
				found = valuesInJSON(doc, l)
			} else {
				t := pageText(b)
				found = append(append(valuesInText(t, l), valuesInScripts(string(b), l)...), valuesAfterMeteoWords(t, l, "html")...)
			}
			mu.Lock()
			out = append(out, found...)
			mu.Unlock()
		}(l)
	}
	wg.Wait()
	return out
}

// valuesInLinked cerca nei file collegati: riquadri, script dello stesso sito e
// indirizzi di dati presenti nella pagina (al massimo 15, 4 alla volta).
func valuesInLinked(ctx context.Context, pageURL, body string) []FoundValue {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var links []string
	add := func(raw string) {
		u, err := base.Parse(html.UnescapeString(raw))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return
		}
		s := u.String()
		for _, l := range links {
			if l == s {
				return
			}
		}
		if s != pageURL && len(links) < 15 {
			links = append(links, s)
		}
	}
	for _, m := range reFrame.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	for _, m := range reDataURL.FindAllStringSubmatch(body, -1) {
		l := strings.ToLower(m[1])
		ext := strings.ToLower(m[2])
		dataFile := ext == "json" || ext == "txt" || ext == "xml" || ext == "csv"
		dataPath := strings.Contains(l, "meteo") || strings.Contains(l, "api") || strings.Contains(l, "data") ||
			strings.Contains(l, "ajax") || strings.Contains(l, "station") || strings.Contains(l, "stazion") || strings.Contains(l, "weather")
		if !dataFile && !dataPath {
			continue // pagine normali del sito (contatti, cerca…): non sono dati
		}
		add(m[1] + "." + m[2] + m[3])
	}
	var mu sync.Mutex
	var out []FoundValue
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, l := range links {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			b, err := getBody(cctx, l, "")
			if err != nil {
				return
			}
			var found []FoundValue
			trimmed := strings.TrimSpace(string(b))
			var doc any
			if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Unmarshal(b, &doc) == nil {
				found = valuesInJSON(doc, l)
			} else {
				found = append(valuesInText(pageText(b), l), valuesInScripts(string(b), l)...)
			}
			mu.Lock()
			out = append(out, found...)
			mu.Unlock()
		}(l)
	}
	wg.Wait()
	return out
}

// valuesInJSON elenca i numeri dei campi con nomi meteo, con il loro percorso.
func valuesInJSON(doc any, source string) []FoundValue {
	var out []FoundValue
	var walk func(v any, path string)
	walk = func(v any, path string) {
		if len(out) >= 60 {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(t[k], p)
			}
		case []any:
			for i, x := range t {
				if i >= 3 {
					break // nelle serie basta il primo elemento (di solito il più recente)
				}
				walk(x, path+"."+strconv.Itoa(i))
			}
		case float64, string:
			s := fmt.Sprint(t)
			if _, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err != nil {
				return
			}
			last := path[strings.LastIndex(path, ".")+1:]
			if reMeteoKey.MatchString(last) {
				p := strings.TrimPrefix(path, ".")
				out = append(out, FoundValue{Value: s, Unit: unitForKey(last), Context: friendlyKey(last) + " (" + p + ")", Pattern: p, Kind: "json", Source: source})
			}
		}
	}
	walk(doc, "")
	return out
}

// friendlyKey traduce il nome di un campo in un'etichetta leggibile.
func friendlyKey(k string) string {
	l := strings.ToLower(k)
	switch {
	case strings.Contains(l, "dew") || strings.Contains(l, "rugiad"):
		return "Punto di rugiada"
	case strings.Contains(l, "temp"):
		return "Temperatura"
	case strings.Contains(l, "hum") || strings.Contains(l, "umid"):
		return "Umidità"
	case strings.Contains(l, "gust") || strings.Contains(l, "raffic"):
		return "Raffiche"
	case strings.Contains(l, "dir"):
		return "Direzione vento"
	case strings.Contains(l, "wind") || strings.Contains(l, "vento"):
		return "Vento"
	case strings.Contains(l, "press") || strings.Contains(l, "baro"):
		return "Pressione"
	case strings.Contains(l, "rain") || strings.Contains(l, "piogg") || strings.Contains(l, "prec"):
		return "Pioggia"
	case strings.Contains(l, "uv"):
		return "Indice UV"
	case strings.Contains(l, "solar") || strings.Contains(l, "radiaz"):
		return "Radiazione solare"
	}
	return k
}

// sortByImportance mette per prime le temperature, poi umidità, vento, ecc.
func sortByImportance(v []FoundValue) {
	rank := func(f FoundValue) int {
		l := strings.ToLower(f.Context)
		for i, w := range []string{"temp", "umid", "hum", "vento", "wind", "raffic", "press", "piogg", "rain"} {
			if strings.Contains(l, w) && !strings.Contains(l, "rugiad") && !strings.Contains(l, "dew") {
				return i
			}
		}
		return 99
	}
	sort.SliceStable(v, func(i, j int) bool { return rank(v[i]) < rank(v[j]) })
}

var reMeteoKey = regexp.MustCompile(`(?i)(temp|umid|humid|hum|vento|wind|gust|raffic|press|baro|piogg|rain|prec|dew|rugiad|uv|solar|radiaz)`)

var reUnitWord = regexp.MustCompile(`(?i)^(°c?|°f|c|f|%|km/h|kmh|m/s|nodi|hpa|mbar|mb|mm/h|mm|w/m²|w/m2|uv|m)$`)

// labelWords tiene le ultime parole (al massimo 4) prima del valore, fermandosi
// al valore precedente: numeri e unità di misura non fanno parte dell'etichetta.
func labelWords(words []string) []string {
	var out []string
	for i := len(words) - 1; i >= 0 && len(out) < 4; i-- {
		if strings.Trim(words[i], "-–—_.") == "" {
			break // "--", "—": segnaposto di un valore mancante, l'etichetta non è nostra
		}
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
	var body []byte
	var err error
	if ds.Variable == "browser" {
		body, _, err = renderPage(ctx, ds.URL)
	} else {
		body, err = getBody(ctx, ds.URL, ds.Token)
	}
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(ds.Path)
	if err != nil {
		return nil, fmt.Errorf("espressione non valida: %w", err)
	}
	text := pageText(body)
	if ds.Variable == "raw" { // valore scritto negli script o nel sorgente
		text = string(body)
	}
	m := re.FindStringSubmatch(text)
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
