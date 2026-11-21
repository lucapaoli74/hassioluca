package main

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"math"
	"strings"
	"time"
)

// Pagina web della stazione meteo. Si pubblica sui siti come una webcam:
// nella lista delle pubblicazioni del sito si sceglie "Pagina meteo"
// (StationPubID) con nome del file e periodicità. Accanto alla pagina si
// pubblica il file .json con gli stessi dati, che la pagina rilegge ogni
// minuto per aggiornarsi senza ricaricarsi.

// StationPubID è l'identificativo della pagina meteo nelle pubblicazioni.
const StationPubID = "_meteo"

// stationPageJSON prepara i dati della pagina.
func (h *StationHub) stationPageJSON(cfg Config, site *Site) map[string]any {
	s := cfg.Station
	h.mu.RLock()
	st := h.st
	vals := map[string]float64{}
	for k, v := range st.Reading.Values {
		vals[k] = v
	}
	hist := append([]stationSample(nil), st.Hist...)
	days := append([]stationDay(nil), st.Days...)
	h.mu.RUnlock()

	now := time.Now()
	title := s.Page.Title
	if title == "" {
		title = "Meteo " + cfg.Location.Name
		if cfg.Location.Name == "" {
			title = "Meteo in tempo reale"
		}
	}
	cur := map[string]any{}
	for _, v := range stationValueList(vals) {
		cur[v.Key] = map[string]any{"v": v.Value, "t": strings.Replace(v.Text, ".", ",", 1), "l": v.Label}
	}
	fields := s.Page.Fields
	if len(fields) == 0 {
		fields = defaultPageFields
	}
	lat, lon := cfg.Location.Latitude, cfg.Location.Longitude
	cond, night := stationCondition(vals, lat, lon, now)

	// serie: 24 ore a 5 minuti, 7 giorni a 30 minuti
	keys := []string{"temp", "humidity", "pressure", "wind", "gust", "solar"}
	series := func(from time.Time, bucket int64) map[string][]any {
		out := map[string][]any{"t": {}}
		for _, k := range keys {
			out[k] = []any{}
		}
		var lastB int64 = -1
		for _, smp := range hist {
			if smp.T < from.Unix() {
				continue
			}
			if b := smp.T / bucket; b == lastB {
				continue
			} else {
				lastB = b
			}
			out["t"] = append(out["t"], smp.T*1000)
			for _, k := range keys {
				if x, ok := smp.V[k]; ok {
					out[k] = append(out[k], x)
				} else {
					out[k] = append(out[k], nil)
				}
			}
		}
		return out
	}
	// pioggia per ora nelle ultime 24 ore, dalla pioggia del giorno
	rain24 := [][2]float64{}
	{
		type hb struct {
			t   int64
			max float64
		}
		var hours []hb
		prev, prevDay := -1.0, ""
		acc := map[int64]float64{}
		for _, smp := range hist {
			if smp.T < now.Add(-25*time.Hour).Unix() {
				continue
			}
			r, ok := smp.V["rain_day"]
			if !ok {
				continue
			}
			day := time.Unix(smp.T, 0).Format("2006-01-02")
			if prev >= 0 {
				d := r - prev
				if day != prevDay || d < 0 {
					d = r
				}
				hour := smp.T / 3600 * 3600
				acc[hour] += d
				if n := len(hours); n == 0 || hours[n-1].t != hour {
					hours = append(hours, hb{t: hour})
				}
			}
			prev, prevDay = r, day
		}
		for _, x := range hours {
			if x.t >= now.Add(-24*time.Hour).Unix() {
				rain24 = append(rain24, [2]float64{float64(x.t * 1000), math.Round(acc[x.t]*10) / 10})
			}
		}
	}
	if len(days) > 400 {
		days = days[len(days)-400:]
	}
	dl := []map[string]any{}
	for _, d := range days {
		m := map[string]any{"d": d.D}
		for k, v := range d.V {
			m[k] = v
		}
		dl = append(dl, m)
	}
	data := map[string]any{
		"title": title, "location": cfg.Location.Name, "altitude": cfg.Location.Altitude, "credits": credits,
		"updated": nil, "cond": cond, "night": night, "cur": cur, "fields": fields,
		"day": series(now.Add(-24*time.Hour), 300), "week": series(now.Add(-7*24*time.Hour), 1800),
		"rain24": rain24, "days": dl, "generated": now.UnixMilli(),
	}
	if !st.Reading.Time.IsZero() {
		data["updated"] = st.Reading.Time.UnixMilli()
		if !st.MinAt.IsZero() && st.Day == now.Format("2006-01-02") {
			data["minmax"] = map[string]any{"min": vals["temp_min"], "max": vals["temp_max"], "min_at": st.MinAt.Format("15:04"), "max_at": st.MaxAt.Format("15:04")}
		}
	}
	// webcam nella pagina: quella scelta, oppure la prima pubblicata sullo stesso sito
	if site != nil && s.Page.CameraID != "none" {
		for _, p := range site.Publications {
			if p.CameraID == StationPubID || (s.Page.CameraID != "" && p.CameraID != s.Page.CameraID) {
				continue
			}
			data["cam"] = p.Filename
			for _, c := range cfg.Cameras {
				if c.ID == p.CameraID {
					data["cam_name"] = c.Name
				}
			}
			break
		}
	}
	return data
}

// StationPageFiles restituisce la pagina e il file .json da pubblicare.
func (h *StationHub) StationPageFiles(cfg Config, site *Site, filename string) map[string][]byte {
	data := h.stationPageJSON(cfg, site)
	base := strings.TrimSuffix(filename, ".html")
	data["json"] = base + ".json"
	jb, _ := json.Marshal(data)
	tpl, _ := webFS.ReadFile("web/meteo-page.html")
	page := bytes.Replace(tpl, []byte("__DATA__"), jb, 1)
	page = bytes.Replace(page, []byte("__TITLE__"), []byte(html.EscapeString(data["title"].(string))), 1)
	return map[string][]byte{filename: page, base + ".json": jb}
}

// stationCondition stima il tempo attuale dai dati: sole, poco nuvoloso,
// nuvoloso, pioggia, temporale, neve, nebbia, vento, notte.
func stationCondition(v map[string]float64, lat, lon float64, now time.Time) (string, bool) {
	if lat == 0 && lon == 0 {
		lat, lon = 44.2, 10.6
	}
	el := sunElevation(lat, lon, now)
	night := el < -2
	rr, t, hum := v["rain_rate"], v["temp"], v["humidity"]
	_, hasT := v["temp"]
	switch {
	case rr >= 0.2 && hasT && t <= 1:
		return "snow", night
	case rr >= 8:
		return "storm", night
	case rr >= 0.2:
		return "rain", night
	case v["gust"] >= 45 || v["wind"] >= 35:
		return "wind", night
	case hum >= 97 && v["wind"] < 5:
		return "fog", night
	case night:
		return "night", true
	}
	solar, ok := v["solar"]
	if !ok {
		return "partly", false
	}
	if el < 5 {
		if solar > 30 {
			return "sun", false
		}
		return "cloudy", false
	}
	clear := 1000 * math.Sin(el*math.Pi/180) * 0.85
	switch r := solar / clear; {
	case r > 0.65:
		return "sun", false
	case r > 0.3:
		return "partly", false
	}
	return "cloudy", false
}

// sunElevation: altezza del sole sull'orizzonte in gradi (approssimata).
func sunElevation(lat, lon float64, t time.Time) float64 {
	u := t.UTC()
	n := float64(u.YearDay())
	decl := 23.44 * math.Sin(2*math.Pi*(284+n)/365) * math.Pi / 180
	hours := float64(u.Hour()) + float64(u.Minute())/60 + lon/15
	ha := (hours - 12) * 15 * math.Pi / 180
	la := lat * math.Pi / 180
	return math.Asin(math.Sin(la)*math.Sin(decl)+math.Cos(la)*math.Cos(decl)*math.Cos(ha)) * 180 / math.Pi
}

// PublishDue pubblica la pagina sui siti dove è il momento (o su tutti se force).
func (h *StationHub) PublishDue(ctx context.Context, force bool) []error {
	cfg := h.cfg()
	var errs []error
	for i := range cfg.Sites {
		site := cfg.Sites[i]
		if !site.Enabled {
			continue
		}
		for _, p := range site.Publications {
			if p.CameraID != StationPubID {
				continue
			}
			if !force && h.isDue != nil && !h.isDue(site.ID, p) {
				continue
			}
			err := h.publishF(ctx, site, h.StationPageFiles(cfg, &site, p.Filename))
			if h.setPub != nil {
				h.setPub(site.ID, p, time.Now(), err)
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}
