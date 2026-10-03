package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Anchor -> frazione (x, y) della posizione all'interno dell'immagine.
var anchors = map[string][2]float64{
	"top-left": {0, 0}, "top-center": {0.5, 0}, "top-right": {1, 0},
	"middle-left": {0, 0.5}, "center": {0.5, 0.5}, "middle-right": {1, 0.5},
	"bottom-left": {0, 1}, "bottom-center": {0.5, 1}, "bottom-right": {1, 1},
}

// Renderer applica ridimensionamento e livelli in sovrimpressione.
type Renderer struct {
	logoDir  string
	store    *Store
	data     *DataManager
	forecast *ForecastCache

	mu    sync.Mutex
	faces map[string]font.Face
	logos map[string]cachedLogo
}

type cachedLogo struct {
	img     image.Image
	modTime time.Time
}

var (
	fontRegular, _ = opentype.Parse(goregular.TTF)
	fontBold, _    = opentype.Parse(gobold.TTF)
)

func NewRenderer(logoDir string, data *DataManager) *Renderer {
	return &Renderer{logoDir: logoDir, data: data, forecast: NewForecastCache(), faces: map[string]font.Face{}, logos: map[string]cachedLogo{}}
}

// Process ridimensiona il JPEG e disegna i livelli attivi.
// Senza modifiche da fare restituisce il JPEG originale.
// site (può essere nil) fornisce i segnaposto {site_url} e {history_url}.
func (r *Renderer) Process(src []byte, cam Camera, now time.Time, site *Site) ([]byte, error) {
	out, _, _, err := r.Render(src, cam, now, site)
	return out, err
}

// Render è come Process ma restituisce anche, per ogni livello, il riquadro
// occupato nell'immagine finale (usato dall'editor per trascinare i livelli)
// e le dimensioni dell'immagine.
func (r *Renderer) Render(src []byte, cam Camera, now time.Time, site *Site) ([]byte, []image.Rectangle, image.Point, error) {
	boxes := make([]image.Rectangle, len(cam.Overlays))
	active := 0
	for _, o := range cam.Overlays {
		if o.Enabled {
			active++
		}
	}
	img, err := jpeg.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, nil, image.Point{}, fmt.Errorf("immagine non leggibile: %w", err)
	}
	if cam.MaxWidth <= 0 && active == 0 {
		return src, boxes, img.Bounds().Size(), nil
	}
	dst := resizeToRGBA(img, cam.MaxWidth)
	for i, o := range cam.Overlays {
		if !o.Enabled {
			continue
		}
		switch o.Type {
		case "text":
			boxes[i] = r.drawText(dst, o, r.expand(o.Text, cam, now, site))
		case "image":
			boxes[i] = r.drawLogo(dst, o)
		case "forecast":
			boxes[i] = r.drawForecast(dst, o, now)
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: cam.Quality}); err != nil {
		return nil, nil, image.Point{}, err
	}
	return out.Bytes(), boxes, dst.Bounds().Size(), nil
}

func resizeToRGBA(img image.Image, maxWidth int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxWidth > 0 && w > maxWidth {
		h = h * maxWidth / w
		w = maxWidth
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w == b.Dx() {
		xdraw.Draw(dst, dst.Bounds(), img, b.Min, xdraw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	}
	return dst
}

var (
	giorni = []string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"}
	mesi   = []string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}

	placeholder = regexp.MustCompile(`\{([a-z_]+)(?::([^}]*))?\}`)
)

// expand sostituisce i segnaposto nel testo:
//
//	{name} nome webcam   {date} 03/10/2026   {time} 14:05   {seconds} 14:05:09
//	{weekday} sabato     {month} ottobre     {year} 2026    {longdate} sabato 3 ottobre 2026
//	{datetime:FORMATO} formato Go, es. {datetime:2006-01-02 15:04}
//	{data:ID} valore della fonte di dati live ID (es. temperatura)
//	{site_url} indirizzo del sito   {history_url} indirizzo del mini-sito storico
func (r *Renderer) expand(tpl string, cam Camera, now time.Time, site *Site) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		sub := placeholder.FindStringSubmatch(m)
		switch sub[1] {
		case "name":
			return cam.Name
		case "date":
			return now.Format("02/01/2006")
		case "time":
			return now.Format("15:04")
		case "seconds":
			return now.Format("15:04:05")
		case "weekday":
			return giorni[now.Weekday()]
		case "month":
			return mesi[now.Month()-1]
		case "year":
			return strconv.Itoa(now.Year())
		case "longdate":
			return fmt.Sprintf("%s %d %s %d", giorni[now.Weekday()], now.Day(), mesi[now.Month()-1], now.Year())
		case "datetime":
			return now.Format(sub[2])
		case "location":
			return r.location().Name
		case "altitude":
			if alt := r.location().Altitude; alt > 0 {
				return formatThousands(alt) + " m s.l.m."
			}
			return ""
		case "site_url":
			if site == nil {
				return ""
			}
			return displayURL(site.PublicURL)
		case "history_url":
			if site == nil || site.PublicURL == "" {
				return ""
			}
			return displayURL(strings.TrimRight(site.PublicURL, "/") + "/" + site.HistoryDir + "/")
		case "data":
			if r.data == nil {
				return "—"
			}
			if v := r.data.Value(sub[2]); v != "" {
				return v
			}
			return "—"
		}
		return m
	})
}

func (r *Renderer) face(bold bool, px float64) font.Face {
	px = math.Round(px)
	if px < 6 {
		px = 6
	}
	key := fmt.Sprintf("%v-%v", bold, px)
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.faces[key]; ok {
		return f
	}
	src := fontRegular
	if bold {
		src = fontBold
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // font integrati: non può fallire
	}
	r.faces[key] = f
	return f
}

// parseColor accetta #rgb, #rrggbb e #rrggbbaa.
func parseColor(s string, opacity float64) color.NRGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	c := color.NRGBA{255, 255, 255, 255}
	if v, err := strconv.ParseUint(s, 16, 32); err == nil {
		switch len(s) {
		case 6:
			c = color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
		case 8:
			c = color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
		}
	}
	c.A = uint8(float64(c.A) * clamp01(opacity))
	return c
}

func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// place calcola l'angolo in alto a sinistra di un elemento w×h secondo
// ancoraggio e margini.
func place(o Overlay, W, H, w, h int) image.Point {
	a := anchors[o.Anchor]
	mx := int(o.OffsetX / 100 * float64(W))
	my := int(o.OffsetY / 100 * float64(H))
	x := int(a[0]*float64(W-w)) + mx - int(a[0]*2*float64(mx))
	y := int(a[1]*float64(H-h)) + my - int(a[1]*2*float64(my))
	// centrato: il margine sposta semplicemente l'elemento
	if a[0] == 0.5 {
		x = (W-w)/2 + mx
	}
	if a[1] == 0.5 {
		y = (H-h)/2 + my
	}
	return image.Pt(x, y)
}

func (r *Renderer) drawText(dst *image.RGBA, o Overlay, text string) image.Rectangle {
	if strings.TrimSpace(text) == "" {
		return image.Rectangle{}
	}
	W, H := dst.Bounds().Dx(), dst.Bounds().Dy()
	px := o.Size / 100 * float64(H)
	face := r.face(o.Bold, px)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	m := face.Metrics()
	lineH := (m.Ascent + m.Descent).Ceil()
	lineGap := int(px * 0.15)
	textW := 0
	for _, l := range lines {
		if w := font.MeasureString(face, l).Ceil(); w > textW {
			textW = w
		}
	}
	textH := len(lines)*lineH + (len(lines)-1)*lineGap
	pad := 0
	if o.Background != "" {
		pad = int(px * 0.35)
	}
	boxW, boxH := textW+2*pad, textH+2*pad
	pos := place(o, W, H, boxW, boxH)
	if o.FullWidth {
		pos.X = 0
		boxW = W
	}

	if o.Background != "" {
		bg := parseColor(o.Background, o.BgOpacity)
		box := image.Rect(pos.X, pos.Y, pos.X+boxW, pos.Y+boxH)
		xdraw.Draw(dst, box, image.NewUniform(bg), image.Point{}, xdraw.Over)
	}

	textX := pos.X + pad
	if o.FullWidth {
		// nella fascia a tutta larghezza il testo segue comunque l'ancoraggio
		a := anchors[o.Anchor]
		mx := int(o.OffsetX/100*float64(W)) + pad
		textX = int(a[0]*float64(W-textW)) + mx - int(a[0]*2*float64(mx))
		if a[0] == 0.5 {
			textX = (W-textW)/2 + mx - pad
		}
	}
	fg := parseColor(o.Color, o.Opacity)
	shadow := color.NRGBA{0, 0, 0, uint8(170 * clamp01(o.Opacity))}
	shOff := int(math.Max(1, px/14))
	for i, l := range lines {
		lw := font.MeasureString(face, l).Ceil()
		lx := textX
		switch anchors[o.Anchor][0] { // allinea le righe come l'ancoraggio
		case 0.5:
			lx += (textW - lw) / 2
		case 1:
			lx += textW - lw
		}
		baseline := pos.Y + pad + i*(lineH+lineGap) + m.Ascent.Ceil()
		if o.Shadow {
			d := font.Drawer{Dst: dst, Src: image.NewUniform(shadow), Face: face, Dot: fixed.P(lx+shOff, baseline+shOff)}
			d.DrawString(l)
		}
		d := font.Drawer{Dst: dst, Src: image.NewUniform(fg), Face: face, Dot: fixed.P(lx, baseline)}
		d.DrawString(l)
	}
	return image.Rect(pos.X, pos.Y, pos.X+boxW, pos.Y+boxH)
}

// LoadLogo legge (con cache) un logo PNG/JPEG dalla cartella dei loghi.
func (r *Renderer) LoadLogo(name string) (image.Image, error) {
	if !filenamePattern.MatchString(name) {
		return nil, fmt.Errorf("nome logo non valido")
	}
	p := filepath.Join(r.logoDir, name)
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	c, ok := r.logos[name]
	r.mu.Unlock()
	if ok && c.modTime.Equal(st.ModTime()) {
		return c.img, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.logos[name] = cachedLogo{img: img, modTime: st.ModTime()}
	r.mu.Unlock()
	return img, nil
}

func (r *Renderer) drawLogo(dst *image.RGBA, o Overlay) image.Rectangle {
	if o.Image == "" {
		return image.Rectangle{}
	}
	logo, err := r.LoadLogo(o.Image)
	if err != nil {
		return image.Rectangle{}
	}
	W, H := dst.Bounds().Dx(), dst.Bounds().Dy()
	lb := logo.Bounds()
	w := int(o.Size / 100 * float64(W))
	if w < 1 {
		return image.Rectangle{}
	}
	h := lb.Dy() * w / lb.Dx()
	scaled := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), logo, lb, xdraw.Src, nil)
	pos := place(o, W, H, w, h)
	if o.Background != "" {
		pad := int(float64(w) * 0.06)
		box := image.Rect(pos.X-pad, pos.Y-pad, pos.X+w+pad, pos.Y+h+pad)
		xdraw.Draw(dst, box, image.NewUniform(parseColor(o.Background, o.BgOpacity)), image.Point{}, xdraw.Over)
	}
	mask := image.NewUniform(color.Alpha{uint8(255 * clamp01(o.Opacity))})
	xdraw.DrawMask(dst, image.Rect(pos.X, pos.Y, pos.X+w, pos.Y+h), scaled, image.Point{}, mask, image.Point{}, xdraw.Over)
	return image.Rect(pos.X, pos.Y, pos.X+w, pos.Y+h)
}

// TestPattern genera un'immagine di prova per l'anteprima delle
// sovrimpressioni quando la telecamera non ha ancora fornito fotogrammi.
func TestPattern(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sky := float64(y) / float64(h)
			img.Set(x, y, color.RGBA{uint8(70 + 60*sky), uint8(120 + 50*sky), uint8(190 - 40*sky), 255})
		}
	}
	var out bytes.Buffer
	_ = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	return out.Bytes()
}

func (r *Renderer) location() Location {
	if r.store == nil {
		return Location{}
	}
	return r.store.Get().Location
}

// formatThousands scrive 1250 come "1.250".
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

// displayURL toglie "https://" per una scritta più corta e leggibile.
func displayURL(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return strings.TrimSuffix(u, "/")
}

// UsesSitePlaceholders dice se le scritte cambiano da sito a sito.
func UsesSitePlaceholders(cam Camera) bool {
	for _, o := range cam.Overlays {
		if o.Enabled && o.Type == "text" && (strings.Contains(o.Text, "{site_url}") || strings.Contains(o.Text, "{history_url}")) {
			return true
		}
	}
	return false
}
