package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Lettura della stazione da un indirizzo web della rete locale, con
// riconoscimento automatico del formato:
//   - Davis WeatherLink Live: /v1/current_conditions (API locale ufficiale)
//   - Cumulus MX, WeeWX (estensioni realtime/gauges), Meteobridge:
//     realtime.txt e realtimegauges.txt / gauge-data.txt
//   - Ecowitt /get_livedata_info e pagine livedata.htm

// ReadStationURL legge l'indirizzo e restituisce valori metrici e formato.
func ReadStationURL(ctx context.Context, u string) (map[string]float64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	body, err := getBody(ctx, u, "")
	if err != nil {
		return nil, "", err
	}
	return ParseStationDocument(body)
}

// ParseStationDocument riconosce il formato e converte.
func ParseStationDocument(body []byte) (map[string]float64, string, error) {
	t := strings.TrimSpace(string(body))
	if strings.HasPrefix(t, "{") {
		var doc map[string]any
		if err := json.Unmarshal([]byte(t), &doc); err == nil {
			if d, ok := doc["data"].(map[string]any); ok {
				if _, ok := d["conditions"]; ok {
					v, err := parseWLL(d)
					return v, "Davis WeatherLink Live", err
				}
			}
			if _, ok := doc["common_list"]; ok {
				v, err := ParseEcowittHTTP([]byte(t))
				return v, "Ecowitt", err
			}
			if _, ok := doc["temp"]; ok {
				v, err := parseGauges(doc)
				return v, "realtimegauges (Cumulus/WeeWX)", err
			}
		}
	}
	if strings.Contains(t, "outTemp") && strings.Contains(strings.ToLower(t), "<input") {
		v := ParseObserverLive(t, observerUnits{})
		if len(v) > 0 {
			return v, "livedata.htm", nil
		}
	}
	if f := strings.Fields(t); len(f) >= 20 && strings.Count(f[0], "/") == 2 && strings.Count(f[1], ":") >= 1 {
		v, err := parseRealtimeTxt(f)
		return v, "realtime.txt (Cumulus/WeeWX/Meteobridge)", err
	}
	return nil, "", errors.New("formato dei dati non riconosciuto")
}

func anyNum(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(x, ",", "."))
		if s == "" || s == "-" || s == "--" || strings.EqualFold(s, "n/a") {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

// unità ------------------------------------------------------------------

func tempToC(x float64, unit string) float64 {
	if strings.Contains(strings.ToLower(unit), "f") {
		return round1((x - 32) * 5 / 9)
	}
	return round1(x)
}

func windToKmh(x float64, unit string) float64 {
	u := strings.ToLower(unit)
	switch {
	case strings.Contains(u, "mph"):
		return round1(x * 1.609344)
	case strings.Contains(u, "m/s"):
		return round1(x * 3.6)
	case strings.Contains(u, "kt") || strings.Contains(u, "kn"):
		return round1(x * 1.852)
	}
	return round1(x)
}

func pressToHPa(x float64, unit string) float64 {
	u := strings.ToLower(unit)
	switch {
	case strings.Contains(u, "in"):
		return round1(x * 33.8639)
	case strings.Contains(u, "mmhg"):
		return round1(x * 1.33322)
	case strings.Contains(u, "kpa"):
		return round1(x * 10)
	}
	return round1(x)
}

func rainToMM(x float64, unit string) float64 {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(unit)), "in") {
		return round1(x * 25.4)
	}
	return round1(x)
}

// Davis WeatherLink Live ----------------------------------------------------

func parseWLL(d map[string]any) (map[string]float64, error) {
	conds, _ := d["conditions"].([]any)
	out := map[string]float64{}
	gotISS := false
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := anyNum(m["data_structure_type"])
		get := func(k string) (float64, bool) { return anyNum(m[k]) }
		switch int(typ) {
		case 1: // ISS: temperatura, vento, pioggia, sole
			if t, ok := get("temp"); ok && !gotISS {
				gotISS = true
				out["temp"] = tempToC(t, "F")
				if x, ok := get("hum"); ok {
					out["humidity"] = round1(x)
				}
				if x, ok := get("dew_point"); ok {
					out["dewpt"] = tempToC(x, "F")
				}
				if x, ok := get("thw_index"); ok {
					out["feels"] = tempToC(x, "F")
				}
				if x, ok := get("wind_speed_avg_last_1_min"); ok {
					out["wind"] = windToKmh(x, "mph")
				} else if x, ok := get("wind_speed_last"); ok {
					out["wind"] = windToKmh(x, "mph")
				}
				if x, ok := get("wind_speed_hi_last_10_min"); ok {
					out["gust"] = windToKmh(x, "mph")
				}
				if x, ok := get("wind_dir_last"); ok {
					out["winddir"] = x
				}
				size := 0.254 // mm per scatto (0.01")
				if rs, ok := get("rain_size"); ok {
					switch int(rs) {
					case 2:
						size = 0.2
					case 3:
						size = 0.1
					case 4:
						size = 0.0254
					}
				}
				for key, k := range map[string]string{"rain_rate": "rain_rate_last", "rain_day": "rainfall_daily", "rain_month": "rainfall_monthly", "rain_year": "rainfall_year", "rain_event": "rain_storm"} {
					if x, ok := get(k); ok {
						out[key] = round1(x * size)
					}
				}
				if x, ok := get("solar_rad"); ok {
					out["solar"] = x
				}
				if x, ok := get("uv_index"); ok {
					out["uv"] = x
				}
			}
		case 3: // barometro
			if x, ok := get("bar_sea_level"); ok {
				out["pressure"] = pressToHPa(x, "inHg")
			}
			if x, ok := get("bar_absolute"); ok {
				out["pressure_abs"] = pressToHPa(x, "inHg")
			}
		case 4: // interno
			if x, ok := get("temp_in"); ok {
				out["temp_in"] = tempToC(x, "F")
			}
			if x, ok := get("hum_in"); ok {
				out["humidity_in"] = round1(x)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("WeatherLink Live: nessun sensore con dati")
	}
	return out, nil
}

// realtimegauges.txt (SteelSeries gauges, Cumulus/WeeWX) -------------------------

func parseGauges(doc map[string]any) (map[string]float64, error) {
	str := func(k string) string { s, _ := doc[k].(string); return s }
	tu, wu, pu, ru := str("tempunit"), str("windunit"), str("pressunit"), str("rainunit")
	out := map[string]float64{}
	set := func(key string, conv func(float64, string) float64, unit string, names ...string) {
		for _, n := range names {
			if x, ok := anyNum(doc[n]); ok {
				if conv != nil {
					x = conv(x, unit)
				}
				out[key] = x
				return
			}
		}
	}
	set("temp", tempToC, tu, "temp")
	set("humidity", nil, "", "hum")
	set("dewpt", tempToC, tu, "dew")
	set("feels", tempToC, tu, "feelslike", "apptemp")
	set("wind", windToKmh, wu, "wspeed", "wlatest")
	set("gust", windToKmh, wu, "wgust")
	set("gust_max", windToKmh, wu, "wgustTM")
	set("winddir", nil, "", "bearing")
	set("pressure", pressToHPa, pu, "press")
	set("rain_day", rainToMM, ru, "rfall")
	set("rain_rate", rainToMM, ru, "rrate")
	set("solar", nil, "", "SolarRad")
	set("uv", nil, "", "UV")
	set("temp_in", tempToC, tu, "intemp")
	set("humidity_in", nil, "", "inhum")
	if len(out) == 0 {
		return nil, errors.New("nessun valore")
	}
	return out, nil
}

// realtime.txt (Cumulus) -------------------------------------------------------

func parseRealtimeTxt(f []string) (map[string]float64, error) {
	num := func(i int) (float64, bool) {
		if i >= len(f) {
			return 0, false
		}
		return anyNum(f[i])
	}
	unit := func(i int) string {
		if i < len(f) {
			return f[i]
		}
		return ""
	}
	wu, tu, pu, ru := unit(13), unit(14), unit(15), unit(16)
	out := map[string]float64{}
	if x, ok := num(2); ok {
		out["temp"] = tempToC(x, tu)
	}
	if x, ok := num(3); ok {
		out["humidity"] = x
	}
	if x, ok := num(4); ok {
		out["dewpt"] = tempToC(x, tu)
	}
	if x, ok := num(5); ok {
		out["wind"] = windToKmh(x, wu)
	}
	if x, ok := num(7); ok {
		out["winddir"] = x
	}
	if x, ok := num(8); ok {
		out["rain_rate"] = rainToMM(x, ru)
	}
	if x, ok := num(9); ok {
		out["rain_day"] = rainToMM(x, ru)
	}
	if x, ok := num(10); ok {
		out["pressure"] = pressToHPa(x, pu)
	}
	if x, ok := num(19); ok {
		out["rain_month"] = rainToMM(x, ru)
	}
	if x, ok := num(20); ok {
		out["rain_year"] = rainToMM(x, ru)
	}
	if x, ok := num(22); ok {
		out["temp_in"] = tempToC(x, tu)
	}
	if x, ok := num(23); ok {
		out["humidity_in"] = x
	}
	if x, ok := num(40); ok {
		out["gust"] = windToKmh(x, wu)
	}
	if x, ok := num(43); ok {
		out["uv"] = x
	}
	if x, ok := num(45); ok {
		out["solar"] = x
	}
	if x, ok := num(54); ok {
		out["feels"] = tempToC(x, tu)
	}
	if _, ok := out["temp"]; !ok {
		return nil, errors.New("realtime.txt senza temperatura")
	}
	return out, nil
}

// Ricerca dei dati su un dispositivo ------------------------------------------------

// stationPaths sono gli indirizzi dove stazioni e programmi meteo pubblicano i dati.
var stationPaths = []string{
	"/v1/current_conditions",
	"/realtimegauges.txt", "/realtime.txt", "/gauge-data.txt",
	"/weewx/realtimegauges.txt", "/weewx/realtime.txt", "/weewx/gauge-data.txt",
	"/cumulus/realtime.txt", "/cumulus/realtimegauges.txt", "/web/realtimegauges.txt",
	"/get_livedata_info", "/livedata.htm",
}

// devicePorts sono le porte web provate e quelle segnalate nel rapporto.
var devicePorts = []int{80, 8080, 8998, 8000, 5000, 443}

var otherPorts = map[int]string{22: "SSH", 1883: "MQTT (WeeWX/Home Assistant)", 3000: "Grafana", 8086: "InfluxDB", 22222: "Davis WeatherLink IP", 8123: "Home Assistant", 45000: "Ecowitt"}

// FoundSource è un indirizzo da cui si leggono dati meteo.
type FoundSource struct {
	URL    string         `json:"url"`
	Format string         `json:"format"`
	List   []StationValue `json:"list"`
}

// ProbeWeatherDevice cerca su un indirizzo IP le pagine con i dati meteo.
func ProbeWeatherDevice(ctx context.Context, host string) ([]FoundSource, string) {
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	line("Analisi di %s · %s", host, time.Now().Format("02/01/2006 15:04"))
	found := []FoundSource{}
	for p, name := range otherPorts {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(p)), time.Second); err == nil {
			c.Close()
			line("  porta %d aperta: %s", p, name)
		}
	}
	for _, port := range devicePorts {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 1500*time.Millisecond)
		if err != nil {
			continue
		}
		c.Close()
		scheme := "http"
		if port == 443 {
			scheme = "https"
		}
		base := scheme + "://" + host
		if port != 80 && port != 443 {
			base += ":" + strconv.Itoa(port)
		}
		title := ""
		if body, err := getBody(ctx, base+"/", ""); err == nil {
			if m := devTitleRe.FindSubmatch(body); m != nil {
				title = strings.TrimSpace(string(m[1]))
			}
		}
		line("  porta %d aperta: pagina web %q", port, title)
		for _, p := range stationPaths {
			vals, format, err := ReadStationURL(ctx, base+p)
			if err != nil {
				continue
			}
			completeStation(vals)
			found = append(found, FoundSource{URL: base + p, Format: format, List: stationValueList(vals)})
			line("    DATI: %s (%s)", base+p, format)
		}
	}
	if len(found) == 0 {
		line("Nessun file di dati meteo trovato negli indirizzi noti.")
	}
	return found, b.String()
}
