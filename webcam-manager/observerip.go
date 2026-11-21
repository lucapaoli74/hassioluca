package main

import (
	"context"
	"errors"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Stazioni con pagina web locale dei dati attuali (livedata.htm), come i
// "Weather Logger"/ObserverIP di Fine Offset venduti da Froggit, Ambient,
// Bresser, Renkforce…: i valori sono campi <input name="outTemp" value="…">
// nelle unità scelte nella pagina della stazione (station.htm).

var (
	inputTagRe = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	attrRe     = regexp.MustCompile(`(?is)\b(name|value)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	selectRe   = regexp.MustCompile(`(?is)<select\b[^>]*name\s*=\s*["']?([\w-]+)[^>]*>(.*?)</select>`)
	selOptRe   = regexp.MustCompile(`(?is)<option\b[^>]*\bselected\b[^>]*>([^<]*)`)
	devTitleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// htmlInputs restituisce i campi name=value della pagina.
func htmlInputs(page string) map[string]string {
	out := map[string]string{}
	for _, tag := range inputTagRe.FindAllString(page, -1) {
		var name, value string
		for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
			v := m[3] + m[4] + m[5]
			if strings.EqualFold(m[1], "name") {
				name = v
			} else {
				value = v
			}
		}
		if name != "" {
			out[name] = html.UnescapeString(strings.TrimSpace(value))
		}
	}
	return out
}

// observerUnits legge le unità scelte nella pagina della stazione.
type observerUnits struct{ temp, wind, press, rain, light string }

func parseObserverUnits(page string) observerUnits {
	u := observerUnits{}
	for _, m := range selectRe.FindAllStringSubmatch(page, -1) {
		name := strings.ToLower(m[1])
		sel := selOptRe.FindStringSubmatch(m[2])
		if sel == nil {
			continue
		}
		val := strings.ToLower(strings.TrimSpace(html.UnescapeString(sel[1])))
		switch {
		case strings.Contains(name, "temp"):
			u.temp = val
		case strings.Contains(name, "wind"):
			u.wind = val
		case strings.Contains(name, "press"):
			u.press = val
		case strings.Contains(name, "rain"):
			u.rain = val
		case strings.Contains(name, "solar") || strings.Contains(name, "light"):
			u.light = val
		}
	}
	return u
}

// ParseObserverLive converte i campi di livedata.htm in unità metriche.
func ParseObserverLive(page string, u observerUnits) map[string]float64 {
	in := htmlInputs(page)
	out := map[string]float64{}
	num := func(k string) (float64, bool) {
		s := strings.ReplaceAll(strings.TrimSpace(in[k]), ",", ".")
		if s == "" || strings.HasPrefix(s, "-") && strings.Trim(s, "-.") == "" {
			return 0, false
		}
		x, err := strconv.ParseFloat(s, 64)
		if err != nil || x <= -999 {
			return 0, false
		}
		return x, true
	}
	temp := func(x float64) float64 {
		if strings.Contains(u.temp, "f") {
			return round1((x - 32) * 5 / 9)
		}
		return x
	}
	wind := func(x float64) float64 {
		w := u.wind
		switch {
		case strings.Contains(w, "mph"):
			return round1(x * 1.609344)
		case strings.Contains(w, "km"):
			return x
		case strings.Contains(w, "knot"):
			return round1(x * 1.852)
		case strings.Contains(w, "ft"):
			return round1(x * 1.09728)
		case strings.Contains(w, "bft"):
			return round1(0.836 * math.Pow(x, 1.5) * 3.6)
		}
		return round1(x * 3.6) // m/s, l'impostazione predefinita
	}
	press := func(x float64) float64 {
		switch {
		case strings.Contains(u.press, "inhg") || (u.press == "" && x < 40):
			return round1(x * 33.8639)
		case strings.Contains(u.press, "mmhg"):
			return round1(x * 1.33322)
		}
		return x
	}
	rain := func(x float64) float64 {
		if strings.HasPrefix(u.rain, "in") {
			return round1(x * 25.4)
		}
		return x
	}
	set := func(key string, conv func(float64) float64, names ...string) {
		for _, n := range names {
			if x, ok := num(n); ok {
				if conv != nil {
					x = conv(x)
				}
				out[key] = x
				return
			}
		}
	}
	set("temp", temp, "outTemp")
	set("temp_in", temp, "inTemp")
	set("humidity", nil, "outHumi")
	set("humidity_in", nil, "inHumi")
	set("pressure", press, "RelPress")
	set("pressure_abs", press, "AbsPress")
	set("winddir", nil, "windir")
	set("wind", wind, "avgwind")
	set("gust", wind, "gustspeed")
	set("gust_max", wind, "dailygust")
	set("rain_rate", rain, "rainofhourly", "rainrate")
	set("rain_event", rain, "eventrain")
	set("rain_day", rain, "rainofdaily")
	set("rain_week", rain, "rainofweekly")
	set("rain_month", rain, "rainofmonthly")
	set("rain_year", rain, "rainofyearly")
	set("uv", nil, "uvi")
	if x, ok := num("solarrad"); ok {
		switch {
		case strings.Contains(u.light, "lux"):
			out["solar"] = float64(int(x/126.7 + 0.5))
		case strings.Contains(u.light, "fc"):
			out["solar"] = float64(int(x*10.764/126.7 + 0.5))
		default:
			out["solar"] = x
		}
	}
	return out
}

// readObserverLive legge http://host/livedata.htm (e le unità da station.htm).
func readObserverLive(ctx context.Context, host string) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := getBody(ctx, "http://"+host+"/livedata.htm", "")
	if err != nil {
		return nil, err
	}
	page := string(body)
	if !strings.Contains(page, "outTemp") && !strings.Contains(page, "inTemp") {
		return nil, errors.New("la pagina livedata.htm non contiene dati meteo")
	}
	units := observerUnits{}
	if sb, err := getBody(ctx, "http://"+host+"/station.htm", ""); err == nil {
		units = parseObserverUnits(string(sb))
	}
	vals := ParseObserverLive(page, units)
	if len(vals) == 0 {
		return nil, errors.New("nessun valore in livedata.htm")
	}
	return vals, nil
}

// pageTitle restituisce il titolo della pagina principale del dispositivo.
func pageTitle(ctx context.Context, host string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body, err := getBody(ctx, "http://"+host+"/", "")
	if err != nil {
		return ""
	}
	if m := devTitleRe.FindSubmatch(body); m != nil {
		return strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	return ""
}
