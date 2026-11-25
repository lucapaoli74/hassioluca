package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"net/url"
	"strconv"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// DayForecast è la previsione di un giorno.
type DayForecast struct {
	Date    time.Time
	Code    int     // codice meteo WMO
	Max     float64 // °C
	Min     float64 // °C
	RainPct float64 // probabilità di precipitazione %
}

type forecastEntry struct {
	days    []DayForecast
	fetched time.Time
}

// ForecastCache conserva le previsioni per coordinate, aggiornate ogni ora.
type ForecastCache struct {
	mu      sync.Mutex
	entries map[string]*forecastEntry
	fetchMu sync.Mutex
	lastLog time.Time
}

// logOnce registra l'errore al massimo una volta ogni ora.
func (f *ForecastCache) logOnce(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Since(f.lastLog) > time.Hour {
		f.lastLog = time.Now()
		log.Printf("previsioni meteo non disponibili: %v", err)
	}
}

func NewForecastCache() *ForecastCache {
	return &ForecastCache{entries: map[string]*forecastEntry{}}
}

// Get restituisce le previsioni (dalla cache se recenti; in caso di errore di
// rete restituisce comunque l'ultimo dato valido).
func (f *ForecastCache) Get(lat, lon float64) ([]DayForecast, error) {
	key := fmt.Sprintf("%.3f,%.3f", lat, lon)
	f.mu.Lock()
	e := f.entries[key]
	f.mu.Unlock()
	if e != nil && time.Since(e.fetched) < time.Hour {
		return e.days, nil
	}
	f.fetchMu.Lock()
	defer f.fetchMu.Unlock()
	f.mu.Lock()
	e = f.entries[key]
	f.mu.Unlock()
	if e != nil && time.Since(e.fetched) < time.Hour {
		return e.days, nil
	}
	days, err := fetchForecast(lat, lon)
	if err != nil {
		if e != nil {
			return e.days, nil
		}
		return nil, err
	}
	f.mu.Lock()
	f.entries[key] = &forecastEntry{days: days, fetched: time.Now()}
	f.mu.Unlock()
	return days, nil
}

func fetchForecast(lat, lon float64) ([]DayForecast, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', 4, 64))
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max")
	q.Set("forecast_days", "7")
	q.Set("timezone", "auto")
	doc, err := getJSON(ctx, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), "")
	if err != nil {
		return nil, err
	}
	daily, _ := doc.(map[string]any)["daily"].(map[string]any)
	if daily == nil {
		return nil, fmt.Errorf("risposta previsioni senza dati")
	}
	arr := func(k string) []any { a, _ := daily[k].([]any); return a }
	num := func(a []any, i int) float64 {
		if i < len(a) {
			if f, ok := a[i].(float64); ok {
				return f
			}
		}
		return math.NaN()
	}
	times := arr("time")
	var out []DayForecast
	for i, t := range times {
		ts, _ := t.(string)
		d, err := time.ParseInLocation("2006-01-02", ts, time.Local)
		if err != nil {
			continue
		}
		code := num(arr("weather_code"), i)
		if math.IsNaN(code) {
			code = -1
		}
		out = append(out, DayForecast{Date: d, Code: int(code), Max: num(arr("temperature_2m_max"), i),
			Min: num(arr("temperature_2m_min"), i), RainPct: num(arr("precipitation_probability_max"), i)})
	}
	return out, nil
}

var giorniBrevi = []string{"Dom", "Lun", "Mar", "Mer", "Gio", "Ven", "Sab"}

// drawForecast disegna il riquadro delle previsioni: una colonna per giorno
// con nome del giorno, icona, temperature e probabilità di pioggia.
func (r *Renderer) drawForecast(dst *image.RGBA, o Overlay, now time.Time) image.Rectangle {
	if r.forecast == nil || (o.Latitude == 0 && o.Longitude == 0) {
		return image.Rectangle{}
	}
	all, err := r.forecast.Get(o.Latitude, o.Longitude)
	if err != nil {
		r.forecast.logOnce(err)
	}
	if err != nil || len(all) == 0 {
		return image.Rectangle{}
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	start := today
	if o.StartTomorrow {
		start = today.AddDate(0, 0, 1)
	}
	n := o.Days
	if n <= 0 {
		n = 3
	}
	var days []DayForecast
	for _, d := range all {
		if !d.Date.Before(start) && len(days) < n {
			days = append(days, d)
		}
	}
	if len(days) == 0 {
		return image.Rectangle{}
	}

	W, H := dst.Bounds().Dx(), dst.Bounds().Dy()
	panelW := int(o.Size / 100 * float64(W))
	colW := panelW / len(days)
	if colW < 30 {
		return image.Rectangle{}
	}
	textPx := float64(colW) * 0.16
	face := r.face(o.Bold, textPx)
	small := r.face(false, textPx*0.8)
	pad := int(textPx * 0.5)
	iconSize := int(float64(colW) * 0.5)
	lineH := (face.Metrics().Ascent + face.Metrics().Descent).Ceil()
	smallH := (small.Metrics().Ascent + small.Metrics().Descent).Ceil()
	rows := pad + lineH + pad/2 + iconSize + pad/2 + lineH
	if o.ShowRain {
		rows += smallH + pad/3
	}
	panelH := rows + pad
	pos := place(o, W, H, panelW, panelH)

	if o.Background != "" {
		xdraw.Draw(dst, image.Rect(pos.X, pos.Y, pos.X+panelW, pos.Y+panelH), image.NewUniform(parseColor(o.Background, o.BgOpacity)), image.Point{}, xdraw.Over)
	}
	fg := parseColor(o.Color, o.Opacity)
	alpha := clamp01(o.Opacity)
	center := func(f font.Face, s string, cx, baseline int, c color.NRGBA) {
		w := font.MeasureString(f, s).Ceil()
		if o.Shadow {
			sh := font.Drawer{Dst: dst, Src: image.NewUniform(color.NRGBA{0, 0, 0, uint8(160 * alpha)}), Face: f, Dot: fixed.P(cx-w/2+1, baseline+1)}
			sh.DrawString(s)
		}
		d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(cx-w/2, baseline)}
		d.DrawString(s)
	}
	for i, d := range days {
		cx := pos.X + i*colW + colW/2
		y := pos.Y + pad
		label := giorniBrevi[d.Date.Weekday()] + " " + strconv.Itoa(d.Date.Day())
		switch {
		case d.Date.Equal(today):
			label = "Oggi"
		case d.Date.Equal(today.AddDate(0, 0, 1)):
			label = "Domani"
		}
		center(face, label, cx, y+face.Metrics().Ascent.Ceil(), fg)
		y += lineH + pad/2
		drawWeatherIcon(dst, d.Code, cx, y+iconSize/2, iconSize, alpha)
		y += iconSize + pad/2
		temps := fmt.Sprintf("%s° / %s°", roundTemp(d.Max), roundTemp(d.Min))
		center(face, temps, cx, y+face.Metrics().Ascent.Ceil(), fg)
		y += lineH
		if o.ShowRain && !math.IsNaN(d.RainPct) {
			y += pad / 3
			rainCol := color.NRGBA{150, 200, 255, uint8(255 * alpha)}
			txt := fmt.Sprintf("%d%%", int(math.Round(d.RainPct)))
			tw := font.MeasureString(small, txt).Ceil()
			asc := small.Metrics().Ascent.Ceil()
			dropR := max(2, asc/4)
			gap := dropR
			startX := cx - (tw+2*dropR+gap)/2
			// goccia: cerchio con punta triangolare (il font non ha il simbolo ☂)
			dy := y + asc - dropR - 1
			fillCircle(dst, startX+dropR, dy, dropR, rainCol)
			fillPolygon(dst, []image.Point{{startX, dy}, {startX + 2*dropR, dy}, {startX + dropR, dy - 2*dropR - dropR/2}}, rainCol)
			d := font.Drawer{Dst: dst, Src: image.NewUniform(rainCol), Face: small, Dot: fixed.P(startX+2*dropR+gap, y+asc)}
			d.DrawString(txt)
		}
	}
	return image.Rect(pos.X, pos.Y, pos.X+panelW, pos.Y+panelH)
}

func roundTemp(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	return strconv.Itoa(int(math.Round(v)))
}

// ---- icone meteo disegnate con forme semplici (nessun file esterno) ----

func fillCircle(dst *image.RGBA, cx, cy, r int, c color.NRGBA) {
	for y := -r; y <= r; y++ {
		dx := int(math.Sqrt(float64(r*r - y*y)))
		xdraw.Draw(dst, image.Rect(cx-dx, cy+y, cx+dx+1, cy+y+1), image.NewUniform(c), image.Point{}, xdraw.Over)
	}
}

// fillPolygon riempie un poligono con l'algoritmo scanline.
func fillPolygon(dst *image.RGBA, pts []image.Point, c color.NRGBA) {
	minY, maxY := pts[0].Y, pts[0].Y
	for _, p := range pts {
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	for y := minY; y <= maxY; y++ {
		var xs []int
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			if (a.Y <= y && b.Y > y) || (b.Y <= y && a.Y > y) {
				xs = append(xs, a.X+(y-a.Y)*(b.X-a.X)/(b.Y-a.Y))
			}
		}
		for i := 0; i+1 < len(xs); i += 2 {
			x0, x1 := min(xs[i], xs[i+1]), max(xs[i], xs[i+1])
			xdraw.Draw(dst, image.Rect(x0, y, x1+1, y+1), image.NewUniform(c), image.Point{}, xdraw.Over)
		}
	}
}

// cloud disegna una nuvola senza sovrapposizioni di trasparenza: prima su una
// maschera, poi in un colpo solo.
func cloud(dst *image.RGBA, cx, cy, s int, c color.NRGBA) {
	mask := image.NewRGBA(image.Rect(cx-s, cy-s, cx+s, cy+s))
	solid := color.NRGBA{255, 255, 255, 255}
	fillCircle(mask, cx-s/3, cy+s/10, s/4, solid)
	fillCircle(mask, cx+s/8, cy-s/8, s/3, solid)
	fillCircle(mask, cx+s/2-s/8, cy+s/8, s/4, solid)
	xdraw.Draw(mask, image.Rect(cx-s/3, cy+s/10, cx+s/2-s/8, cy+s/10+s/4), image.NewUniform(solid), image.Point{}, xdraw.Over)
	xdraw.DrawMask(dst, mask.Bounds(), image.NewUniform(c), image.Point{}, mask, mask.Bounds().Min, xdraw.Over)
}

func drawWeatherIcon(dst *image.RGBA, code, cx, cy, size int, alpha float64) {
	a := uint8(255 * alpha)
	sun := color.NRGBA{255, 196, 40, a}
	white := color.NRGBA{245, 247, 250, a}
	grey := color.NRGBA{170, 178, 190, a}
	dark := color.NRGBA{120, 128, 140, a}
	blue := color.NRGBA{90, 170, 255, a}
	s := size / 2
	drawSun := func(x, y, r int) {
		for i := 0; i < 8; i++ {
			ang := float64(i) * math.Pi / 4
			x1, y1 := x+int(float64(r)*1.35*math.Cos(ang)), y+int(float64(r)*1.35*math.Sin(ang))
			fillCircle(dst, x1, y1, max(1, r/5), sun)
		}
		fillCircle(dst, x, y, r, sun)
	}
	drops := func(c color.NRGBA, snow bool) {
		for i := -1; i <= 1; i++ {
			x := cx + i*s/3
			y := cy + s/2
			if snow {
				fillCircle(dst, x, y+s/6, max(1, s/9), c)
			} else {
				fillPolygon(dst, []image.Point{{x, y}, {x - s/10, y + s/3}, {x - s/20, y + s/3}, {x + s/20, y}}, c)
			}
		}
	}
	switch {
	case code == 0: // sereno
		drawSun(cx, cy, s/2)
	case code == 1 || code == 2: // poco nuvoloso
		drawSun(cx-s/4, cy-s/4, s/3)
		cloud(dst, cx+s/8, cy+s/8, s*3/4, white)
	case code == 3: // coperto
		cloud(dst, cx-s/6, cy-s/5, s*2/3, grey)
		cloud(dst, cx+s/8, cy+s/10, s*3/4, white)
	case code == 45 || code == 48: // nebbia
		for i := 0; i < 3; i++ {
			y := cy - s/3 + i*s/3
			xdraw.Draw(dst, image.Rect(cx-s*3/4, y, cx+s*3/4, y+max(2, s/8)), image.NewUniform(grey), image.Point{}, xdraw.Over)
		}
	case code >= 51 && code <= 67, code >= 80 && code <= 82: // pioggia / pioviggine
		cloud(dst, cx, cy-s/4, s*3/4, grey)
		drops(blue, false)
	case code >= 71 && code <= 77, code == 85 || code == 86: // neve
		cloud(dst, cx, cy-s/4, s*3/4, grey)
		drops(white, true)
	case code >= 95: // temporale
		cloud(dst, cx, cy-s/4, s*3/4, dark)
		fillPolygon(dst, []image.Point{{cx + s/10, cy}, {cx - s/5, cy + s/2}, {cx, cy + s/2}, {cx - s/8, cy + s*7/8}, {cx + s/4, cy + s/3}, {cx + s/20, cy + s/3}, {cx + s/4, cy}}, sun)
	default:
		cloud(dst, cx, cy, s*3/4, grey)
	}
}
