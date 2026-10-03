package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fakeCamera simula una telecamera con autenticazione Digest.
func fakeCamera(t *testing.T, frame []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="cam", qop="auth", nonce="abc123", opaque="xyz"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p := parseAuthParams(auth[len("Digest "):])
		ha1 := md5hex("admin:cam:secret")
		ha2 := md5hex("GET:" + p["uri"])
		h := md5.Sum([]byte(ha1 + ":" + p["nonce"] + ":" + p["nc"] + ":" + p["cnonce"] + ":auth:" + ha2))
		if p["response"] != hex.EncodeToString(h[:]) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/mjpeg" {
			w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=f")
			for i := 0; i < 3; i++ {
				fmt.Fprintf(w, "--f\r\nContent-Type: image/jpeg\r\n\r\n")
				w.Write(frame)
				fmt.Fprintf(w, "\r\n")
			}
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(frame)
	}))
}

func TestCaptureDigestSnapshotAndMJPEG(t *testing.T) {
	frame := testJPEG(t, 64, 36, color.RGBA{200, 0, 0, 255})
	srv := fakeCamera(t, frame)
	defer srv.Close()
	for _, src := range []string{"snapshot", "mjpeg"} {
		path := "/snap.jpg"
		if src == "mjpeg" {
			path = "/mjpeg"
		}
		got, err := CaptureFrame(context.Background(), Camera{Source: src, URL: srv.URL + path, Username: "admin", Password: "secret"})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if !bytes.Equal(got, frame) {
			t.Fatalf("%s: fotogramma diverso", src)
		}
	}
	if _, err := CaptureFrame(context.Background(), Camera{Source: "snapshot", URL: srv.URL + "/x", Username: "admin", Password: "wrong"}); err == nil {
		t.Fatal("credenziali errate accettate")
	}
}

func TestNextJPEG(t *testing.T) {
	data := []byte{0x00, 0x11, 0xFF, 0xD8, 0x01, 0x02, 0xFF, 0xD9, 0x33}
	got, err := nextJPEG(bufio.NewReader(bytes.NewReader(data)))
	if err != nil || !bytes.Equal(got, []byte{0xFF, 0xD8, 0x01, 0x02, 0xFF, 0xD9}) {
		t.Fatalf("got %x %v", got, err)
	}
}

func TestRenderOverlaysAndBoxes(t *testing.T) {
	dir := t.TempDir()
	r := NewRenderer(dir, nil)
	src := testJPEG(t, 1280, 720, color.RGBA{30, 60, 90, 255})
	cam := Camera{ID: "c", Name: "Porto", Quality: 85, MaxWidth: 640, Overlays: []Overlay{
		{Type: "text", Enabled: true, Text: "{name} {date}", Anchor: "bottom-right", Size: 5, Color: "#fff", Background: "#000", BgOpacity: .5, Opacity: 1, OffsetX: 2, OffsetY: 3},
		{Type: "text", Enabled: false, Text: "spento", Anchor: "center", Size: 5, Opacity: 1},
	}}
	out, boxes, size, err := r.Render(src, cam, time.Date(2026, 10, 3, 14, 5, 0, 0, time.Local), nil)
	if err != nil {
		t.Fatal(err)
	}
	if size.X != 640 || size.Y != 360 {
		t.Fatalf("dimensione %v", size)
	}
	b := boxes[0]
	if b.Empty() || b.Max.X > 640 || b.Max.Y > 360 {
		t.Fatalf("riquadro %v", b)
	}
	// ancorato in basso a destra con margine 2%/3%
	if d := 640 - b.Max.X - 12; d < -1 || d > 1 {
		t.Fatalf("margine orizzontale errato: %v", b)
	}
	if !boxes[1].Empty() {
		t.Fatal("livello disattivato disegnato")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatal(err)
	}
}

func TestExpandPlaceholders(t *testing.T) {
	dm := NewDataManager()
	dm.values["temp"] = DataValue{Value: "12,3°C"}
	r := NewRenderer("", dm)
	got := r.expand("{name} {weekday} {longdate} {time} {data:temp} {data:none} {history_url}", Camera{Name: "Lago"},
		time.Date(2026, 10, 3, 9, 7, 0, 0, time.Local), &Site{PublicURL: "https://www.esempio.it/webcam/", HistoryDir: "storico"})
	want := "Lago sabato sabato 3 ottobre 2026 09:07 12,3°C — www.esempio.it/webcam/storico"
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func TestFormatValueAndPath(t *testing.T) {
	var doc any
	_ = json.Unmarshal([]byte(`{"observations":[{"humidity":81,"metric":{"temp":12.34}}]}`), &doc)
	v, err := lookupPath(doc, "observations.0.metric.temp")
	if err != nil || formatValue(v, 1, "°C") != "12,3°C" {
		t.Fatalf("%v %v", v, err)
	}
	if formatThousands(1250) != "1.250" {
		t.Fatal(formatThousands(1250))
	}
}

func TestConfigValidation(t *testing.T) {
	c := Config{Cameras: []Camera{{ID: "a b", Source: "snapshot", URL: "x"}}}
	c.applyDefaults()
	if c.Validate() == nil {
		t.Fatal("ID con spazio accettato")
	}
	c = Config{Cameras: []Camera{{ID: "a", Source: "snapshot", URL: "x", ArchiveMinutes: 7}}}
	c.applyDefaults()
	if c.Validate() == nil {
		t.Fatal("frequenza storico non divisore accettata")
	}
	c = Config{Cameras: []Camera{{ID: "a", Source: "snapshot", URL: "x"}},
		Sites: []Site{{ID: "s", Protocol: "ftp", Host: "h", Publications: []Publication{{CameraID: "zzz", Filename: "a.jpg"}}}}}
	c.applyDefaults()
	if c.Validate() == nil {
		t.Fatal("pubblicazione di webcam inesistente accettata")
	}
}

func TestActiveHours(t *testing.T) {
	c := Camera{ActiveFrom: "20:00", ActiveTo: "06:00"}
	at := func(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.Local) }
	if !c.IsActive(at(23)) || !c.IsActive(at(3)) || c.IsActive(at(12)) {
		t.Fatal("fascia notturna errata")
	}
}

func TestArchivePrune(t *testing.T) {
	a, err := NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	img := testJPEG(t, 400, 300, color.RGBA{1, 2, 3, 255})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for d := 0; d < 10; d++ {
		for h := 0; h < 24; h += 6 {
			if err := a.Save("cam", start.AddDate(0, 0, d).Add(time.Duration(h)*time.Hour), img); err != nil {
				t.Fatal(err)
			}
		}
	}
	if a.Count("cam") != 40 {
		t.Fatalf("count %d", a.Count("cam"))
	}
	// conservazione 5 giorni rispetto all'ultimo giorno
	a.Prune("cam", 5, 0, start.AddDate(0, 0, 9))
	if a.Count("cam") != 24 { // giorni 4..9
		t.Fatalf("dopo prune per giorni: %d", a.Count("cam"))
	}
	per := a.Size("cam") / int64(a.Count("cam"))
	a.Prune("cam", 0, per*10, time.Now())
	if a.Count("cam") != 10 {
		t.Fatalf("dopo prune per spazio: %d", a.Count("cam"))
	}
	// ricaricando da disco lo stato coincide
	b, _ := NewArchive(a.dir)
	if b.Count("cam") != 10 || b.Size("cam") != a.Size("cam") {
		t.Fatalf("riletto: %d %d / %d", b.Count("cam"), b.Size("cam"), a.Size("cam"))
	}
	keys := b.KeysAfter("cam", "", 100)
	if len(keys) != 10 || keys[0] >= keys[9] {
		t.Fatalf("chiavi %v", keys)
	}
}

func TestTimelapseRolling(t *testing.T) {
	tl := NewTimelapse(t.TempDir())
	img := testJPEG(t, 1280, 720, color.RGBA{9, 9, 9, 255})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 200; i++ { // ogni 2 minuti per oltre 6 ore
		if _, err := tl.Add("cam", start.Add(time.Duration(i*2)*time.Minute), img, 5, 6); err != nil {
			t.Fatal(err)
		}
	}
	var idx tlIndex
	_ = json.Unmarshal(tl.JSON("cam"), &idx)
	if len(idx.Frames) != 72 {
		t.Fatalf("fotogrammi %d, attesi 72", len(idx.Frames))
	}
	files, _ := filepath.Glob(filepath.Join(tl.dir, "cam", "tl_*.jpg"))
	if len(files) != 72 {
		t.Fatalf("file su disco %d", len(files))
	}
}

func TestParseVersion(t *testing.T) {
	if compareVersions("v1.10.0", "1.9.3") != 1 || compareVersions("dev", "0.0.1") != -1 || compareVersions("1.2.0", "v1.2.0") != 0 {
		t.Fatal("confronto versioni errato")
	}
}

// Prova completa: telecamera finta -> cattura -> sovrimpressioni -> storico,
// timelapse e pubblicazione su una cartella con mini-sito.
func TestEndToEndPublishToFolder(t *testing.T) {
	frame := testJPEG(t, 800, 450, color.RGBA{0, 120, 0, 255})
	srv := fakeCamera(t, frame)
	defer srv.Close()
	data := t.TempDir()
	out := t.TempDir()
	cfgJSON := fmt.Sprintf(`{"cameras":[{"id":"porto","name":"Porto","enabled":true,"source":"snapshot","url":%q,"username":"admin","password":"secret",
		"interval_seconds":60,"archive_minutes":60,"timelapse_minutes":5,"overlays":[{"type":"text","enabled":true,"text":"{name}","anchor":"top-left"}]}],
		"sites":[{"id":"locale","name":"Locale","enabled":true,"protocol":"folder","remote_dir":%q,"history":true,
		"publications":[{"camera_id":"porto","filename":"webcam.jpg","interval_seconds":120}]}]}`, srv.URL+"/snap.jpg", out)
	if err := os.WriteFile(filepath.Join(data, "config.json"), []byte(cfgJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(data, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	app.Reload()
	defer app.sched.cancel()
	want := []string{"webcam.jpg", "storico/index.html", "storico/cameras.json", "storico/porto/days.json", "storico/porto/timelapse/timelapse.json"}
	deadline := time.Now().Add(15 * time.Second)
	for {
		missing := ""
		for _, w := range want {
			if _, err := os.Stat(filepath.Join(out, w)); err != nil {
				missing = w
			}
		}
		if missing == "" {
			break
		}
		if time.Now().After(deadline) {
			cams, pubs, sites := app.sched.Status()
			t.Fatalf("manca %s; stato %+v %+v %+v", missing, cams, pubs, sites)
		}
		time.Sleep(200 * time.Millisecond)
	}
	var idx historyIndex
	b, _ := os.ReadFile(filepath.Join(out, "storico/cameras.json"))
	if err := json.Unmarshal(b, &idx); err != nil || len(idx.Cameras) != 1 || idx.Cameras[0].Current != "../webcam.jpg" || !idx.Cameras[0].Timelapse {
		t.Fatalf("cameras.json: %s", b)
	}
	matches, _ := filepath.Glob(filepath.Join(out, "storico/porto/*/*/*/*.jpg"))
	if len(matches) != 2 { // immagine + miniatura
		t.Fatalf("storico pubblicato: %v", matches)
	}
	steps, err := TestSite(context.Background(), app.store, app.store.Get().Sites[0])
	if err != nil || len(steps) < 3 {
		t.Fatalf("test sito: %v %v", steps, err)
	}
	if _, err := os.Stat(filepath.Join(out, "webcam-manager-test.txt")); err == nil {
		t.Fatal("file di prova non cancellato")
	}
}

func TestForecastRender(t *testing.T) {
	r := NewRenderer("", nil)
	today := time.Now()
	base := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	r.forecast.entries["44.197,10.573"] = &forecastEntry{fetched: time.Now(), days: []DayForecast{
		{Date: base, Code: 2, Max: 18.4, Min: 7.1, RainPct: 10},
		{Date: base.AddDate(0, 0, 1), Code: 63, Max: 14, Min: 8, RainPct: 80},
		{Date: base.AddDate(0, 0, 2), Code: 95, Max: 12, Min: 5, RainPct: 65},
		{Date: base.AddDate(0, 0, 3), Code: 73, Max: 2, Min: -4, RainPct: 40},
	}}
	src := testJPEG(t, 1280, 720, color.RGBA{60, 110, 160, 255})
	cam := Camera{Quality: 90, Overlays: []Overlay{
		{Type: "forecast", Enabled: true, Anchor: "top-right", Size: 40, Days: 4, ShowRain: true, Bold: true, Color: "#ffffff", Background: "#000000", BgOpacity: .45, Opacity: 1, OffsetX: 1.5, OffsetY: 2.5, Latitude: 44.197, Longitude: 10.573},
		{Type: "text", Enabled: true, Text: "Prova fascia", Anchor: "bottom-left", Size: 4, Color: "#fff", Background: "#000", BgOpacity: .5, Opacity: 1, FullWidth: true},
	}}
	out, boxes, _, err := r.Render(src, cam, time.Now(), nil)
	if err != nil || boxes[0].Empty() {
		t.Fatalf("%v %v", boxes, err)
	}
	if p := os.Getenv("FORECAST_PNG"); p != "" {
		_ = os.WriteFile(p, out, 0o644)
	}
}

func TestRTSPSourceAndErrors(t *testing.T) {
	cam := Camera{Source: "snapshot", URL: "rtsp://admin:SEGRETO@127.0.0.1:1/h264_stream"}
	if effectiveSource(cam) != "rtsp" {
		t.Fatal("URL rtsp:// non riconosciuto")
	}
	_, err := CaptureFrame(context.Background(), cam)
	if err == nil {
		t.Fatal("atteso errore")
	}
	msg := err.Error()
	if strings.Contains(msg, "SEGRETO") {
		t.Fatalf("password nel messaggio: %s", msg)
	}
	t.Log(msg)
}
