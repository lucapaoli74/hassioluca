package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

var historyHTML = func() []byte {
	b, err := webFS.ReadFile("web/history.html")
	if err != nil {
		panic(err)
	}
	return b
}()

// mask sostituisce le password nelle risposte al browser; se torna indietro
// invariato significa "lascia la password attuale".
const mask = "********"

type Server struct {
	app *App
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	a := s.app

	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServer(http.FS(static)))

	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/export", s.export)
	mux.HandleFunc("POST /api/import", s.importZip)

	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.updater.Status())
	})
	mux.HandleFunc("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		_ = a.updater.Check(r.Context())
		writeJSON(w, a.updater.Status())
	})
	mux.HandleFunc("POST /api/update/apply", func(w http.ResponseWriter, r *http.Request) {
		if err := a.updater.Apply(r.Context()); err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "Aggiornamento installato, il programma si sta riavviando"})
	})
	mux.HandleFunc("POST /api/update/upload", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, 200<<20))
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := InstallExecutable(data); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("nuovo eseguibile caricato dal pannello: riavvio")
		go func() { time.Sleep(time.Second); a.RequestRestart() }()
		writeJSON(w, map[string]string{"ok": "Eseguibile sostituito, il programma si sta riavviando"})
	})
	mux.HandleFunc("GET /api/backups", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.store.Backups())
	})
	mux.HandleFunc("POST /api/backups/{name}/restore", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := a.store.LoadBackup(r.PathValue("name"))
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		cur := a.store.Get()
		cfg.Listen, cfg.AdminUser, cfg.AdminPassword = cur.Listen, cur.AdminUser, cur.AdminPassword
		if err := a.store.Replace(cfg); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		a.Reload()
		writeJSON(w, map[string]string{"ok": "Configurazione ripristinata"})
	})
	mux.HandleFunc("GET /api/ffmpeg", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, GetFFmpegStatus())
	})
	mux.HandleFunc("POST /api/ffmpeg/install", func(w http.ResponseWriter, r *http.Request) {
		if err := StartFFmpegInstall(); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, GetFFmpegStatus())
	})
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, recentLog.Lines())
	})
	mux.HandleFunc("GET /api/diagnostics", s.diagnostics)

	mux.HandleFunc("POST /api/cameras/{id}/capture", func(w http.ResponseWriter, r *http.Request) {
		if !a.sched.Trigger(r.PathValue("id")) {
			httpError(w, http.StatusNotFound, "webcam non trovata o disattivata")
			return
		}
		writeJSON(w, map[string]string{"ok": "1"})
	})
	mux.HandleFunc("POST /api/test-camera", s.testCamera)
	mux.HandleFunc("POST /api/overlay-preview", s.overlayPreview)
	mux.HandleFunc("POST /api/test-site", s.testSite)
	mux.HandleFunc("POST /api/sites/{id}/resync-history", func(w http.ResponseWriter, r *http.Request) {
		a.sync.ResetSite(r.PathValue("id"))
		a.sched.Restart()
		writeJSON(w, map[string]string{"ok": "1"})
	})
	mux.HandleFunc("POST /api/test-datasource", s.testDataSource)
	// espande i link brevi di Google Maps (maps.app.goo.gl) per leggerne le coordinate
	mux.HandleFunc("POST /api/resolve-link", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ URL string }
		if !readJSON(w, r, &req) {
			return
		}
		u, err := url.Parse(req.URL)
		if err != nil || (u.Host != "maps.app.goo.gl" && u.Host != "goo.gl" && !strings.HasSuffix(u.Host, ".google.com") && u.Host != "google.com") {
			httpError(w, http.StatusBadRequest, "non è un link di Google Maps")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		req2.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := httpClient.Do(req2)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		resp.Body.Close()
		writeJSON(w, map[string]string{"url": resp.Request.URL.String()})
	})
	wuKey := func(k string) string {
		if k == "" || k == mask {
			return a.store.Get().WUApiKey
		}
		return k
	}
	mux.HandleFunc("POST /api/wu/search", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key       string  `json:"key"`
			Query     string  `json:"query"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		key := wuKey(req.Key)
		place := ""
		if req.Query != "" {
			lat, lon, name, err := WUGeocode(r.Context(), key, req.Query)
			if err != nil {
				httpError(w, http.StatusBadGateway, err.Error())
				return
			}
			req.Latitude, req.Longitude, place = lat, lon, name
		}
		st, err := WUNearby(r.Context(), key, req.Latitude, req.Longitude)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, map[string]any{"place": place, "latitude": req.Latitude, "longitude": req.Longitude, "stations": st})
	})
	mux.HandleFunc("POST /api/wu/observation", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key     string `json:"key"`
			Station string `json:"station"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		vals, when, err := WUObservation(r.Context(), wuKey(req.Key), req.Station)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, map[string]any{"values": vals, "time": when})
	})
	mux.HandleFunc("POST /api/test-email", func(w http.ResponseWriter, r *http.Request) {
		var st AlertSettings
		if !readJSON(w, r, &st) {
			return
		}
		if st.Password == mask {
			st.Password = a.store.Get().Alerts.Password
		}
		if err := SendMail(st, "[Webcam Manager] Email di prova da "+hostname(), "Le notifiche email di Webcam Manager su "+hostname()+" funzionano."); err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "Email inviata"})
	})
	mux.HandleFunc("POST /api/sites/{id}/onedrive-connect", func(w http.ResponseWriter, r *http.Request) {
		var site *Site
		cfg := a.store.Get()
		for i := range cfg.Sites {
			if cfg.Sites[i].ID == r.PathValue("id") {
				site = &cfg.Sites[i]
			}
		}
		if site == nil {
			httpError(w, http.StatusNotFound, "salva prima il sito")
			return
		}
		id, flow, err := StartDeviceFlow(a.store, site.ID, site.OAuthClientID, site.OAuthTenant)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, map[string]any{"flow": id, "user_code": flow.UserCode, "verification_uri": flow.VerificationURI, "message": flow.Message})
	})
	mux.HandleFunc("GET /api/onedrive-flow/{id}", func(w http.ResponseWriter, r *http.Request) {
		f, ok := GetDeviceFlow(r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, f)
	})

	mux.HandleFunc("POST /api/discover", func(w http.ResponseWriter, r *http.Request) {
		found, err := Discover(r.Context())
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg := a.store.Get()
		for i := range found {
			for _, c := range cfg.Cameras {
				if strings.Contains(c.URL, "//"+found[i].IP+"/") || strings.Contains(c.URL, "//"+found[i].IP+":") || strings.Contains(c.URL, "@"+found[i].IP) {
					found[i].Configured = c.ID
				}
			}
		}
		writeJSON(w, found)
	})
	mux.HandleFunc("POST /api/onvif", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ XAddr, Username, Password string }
		if !readJSON(w, r, &req) {
			return
		}
		info, err := QueryOnvif(r.Context(), req.XAddr, req.Username, req.Password)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, info)
	})
	mux.HandleFunc("POST /api/live-session", func(w http.ResponseWriter, r *http.Request) {
		var cam Camera
		if !readJSON(w, r, &cam) {
			return
		}
		s.fillCameraPassword(&cam)
		writeJSON(w, map[string]string{"token": a.live.NewSession(cam)})
	})

	mux.HandleFunc("GET /live/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cam, ok := a.sched.camera(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.mjpeg(w, r, "cam:"+id, cam)
	})
	mux.HandleFunc("GET /live/session/{token}", func(w http.ResponseWriter, r *http.Request) {
		tok := r.PathValue("token")
		cam, ok := a.live.Session(tok)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.mjpeg(w, r, "tmp:"+tok, cam)
	})

	mux.HandleFunc("GET /snapshot/{file}", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(r.PathValue("file"), ".jpg")
		s.serveLatest(w, id)
	})

	mux.HandleFunc("GET /api/logos", s.listLogos)
	mux.HandleFunc("POST /api/logos", s.uploadLogo)
	mux.HandleFunc("GET /api/logos/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !filenamePattern.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(a.logoDir(), name))
	})
	mux.HandleFunc("DELETE /api/logos/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !filenamePattern.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		_ = os.Remove(filepath.Join(a.logoDir(), name))
		writeJSON(w, map[string]string{"ok": "1"})
	})

	// storico locale: stessa struttura del mini-sito pubblicato
	mux.HandleFunc("GET /history/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(historyHTML)
	})
	mux.HandleFunc("GET /history/cameras.json", func(w http.ResponseWriter, r *http.Request) {
		cfg := a.store.Get()
		idx := historyIndex{Title: cfg.Location.Name, Cameras: []HistoryCamera{}}
		if idx.Title == "" {
			idx.Title = "Storico webcam"
		}
		for _, c := range cfg.Cameras {
			if c.ArchiveMinutes > 0 || c.TimelapseMinutes > 0 || a.archive.Count(c.ID) > 0 {
				idx.Cameras = append(idx.Cameras, HistoryCamera{ID: c.ID, Name: c.Name, Current: "/snapshot/" + c.ID + ".jpg", ArchiveMinutes: c.ArchiveMinutes, Timelapse: c.TimelapseMinutes > 0})
			}
		}
		noCache(w)
		writeJSON(w, idx)
	})
	mux.HandleFunc("GET /history/{cam}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		cam, rest := r.PathValue("cam"), r.PathValue("rest")
		if tlFile, ok := strings.CutPrefix(rest, "timelapse/"); ok {
			if tlFile == "timelapse.json" {
				noCache(w)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(a.tl.JSON(cam))
				return
			}
			p, ok := a.tl.File(cam, tlFile)
			if !ok {
				http.NotFound(w, r)
				return
			}
			noCache(w)
			http.ServeFile(w, r, p)
			return
		}
		if rest == "days.json" {
			noCache(w)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(a.archive.DaysJSON(cam))
			return
		}
		key, thumb := strings.TrimSuffix(rest, ".jpg"), false
		if k, ok := strings.CutSuffix(key, "_t"); ok {
			key, thumb = k, true
		}
		p, err := a.archive.File(cam, key, thumb)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, p)
	})

	return s.withAuth(mux)
}

// withAuth protegge tutto con Basic Auth tranne le immagini pubbliche.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/public/") {
			s.public(w, r)
			return
		}
		if r.URL.Path == "/healthz" {
			s.health(w)
			return
		}
		cfg := s.app.store.Get()
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(cfg.AdminUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(cfg.AdminPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Webcam Manager", charset="UTF-8"`)
			http.Error(w, "accesso richiesto", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// public serve /public/<id>.jpg per le webcam contrassegnate come pubbliche,
// da incorporare nei siti con <img src="http://indirizzo:8080/public/id.jpg">.
func (s *Server) public(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/public/"), ".jpg")
	cam, ok := s.app.sched.camera(id)
	if !ok || !cam.Public {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	s.serveLatest(w, id)
}

// health risponde senza password per il monitoraggio esterno (Uptime Kuma,
// PRTG, Zabbix…): 200 se tutto funziona, 503 se ci sono errori in corso.
func (s *Server) health(w http.ResponseWriter) {
	cfg := s.app.store.Get()
	cams, pubs, _ := s.app.sched.Status()
	failing := 0
	for _, c := range cfg.Cameras {
		if st, ok := cams[c.ID]; c.Enabled && ok && st.LastError != "" && !st.OutOfHours {
			failing++
		}
	}
	for _, p := range pubs {
		if p.LastError != "" {
			failing++
		}
	}
	s.app.sched.mu.Lock()
	low := s.app.sched.diskLow
	s.app.sched.mu.Unlock()
	status := "ok"
	if failing > 0 || low {
		status = "errori"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, map[string]any{"status": status, "errors": failing, "disk_low": low, "version": version,
		"uptime_seconds": int(time.Since(startTime).Seconds()), "location": cfg.Location.Name})
}

func (s *Server) serveLatest(w http.ResponseWriter, id string) {
	img := s.app.sched.Latest(id)
	if img == nil {
		httpError(w, http.StatusNotFound, "nessuna immagine ancora disponibile")
		return
	}
	noCache(w)
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(img)
}

func noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(v); err != nil {
		httpError(w, http.StatusBadRequest, "richiesta non valida: "+err.Error())
		return false
	}
	return true
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	_, ffErr := ffmpegPath()
	writeJSON(w, map[string]any{
		"version":  version,
		"credits":  credits,
		"data_dir": s.app.dataDir,
		"ffmpeg":   ffErr == nil,
		"hostname": hostname(),
	})
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func maskConfig(cfg Config) Config {
	if cfg.AdminPassword != "" {
		cfg.AdminPassword = mask
	}
	for i := range cfg.Cameras {
		if cfg.Cameras[i].Password != "" {
			cfg.Cameras[i].Password = mask
		}
	}
	for i := range cfg.Sites {
		if cfg.Sites[i].Password != "" {
			cfg.Sites[i].Password = mask
		}
		if cfg.Sites[i].OAuthRefresh != "" {
			cfg.Sites[i].OAuthRefresh = mask
		}
	}
	for i := range cfg.DataSources {
		if cfg.DataSources[i].Token != "" {
			cfg.DataSources[i].Token = mask
		}
	}
	if cfg.Update.Token != "" {
		cfg.Update.Token = mask
	}
	if cfg.Alerts.Password != "" {
		cfg.Alerts.Password = mask
	}
	if cfg.WUApiKey != "" {
		cfg.WUApiKey = mask
	}
	return cfg
}

// unmask rimette le password attuali dove il browser ha rimandato la maschera.
func unmask(cfg *Config, old Config) {
	if cfg.AdminPassword == mask {
		cfg.AdminPassword = old.AdminPassword
	}
	if cfg.Update.Token == mask {
		cfg.Update.Token = old.Update.Token
	}
	if cfg.Alerts.Password == mask {
		cfg.Alerts.Password = old.Alerts.Password
	}
	if cfg.WUApiKey == mask {
		cfg.WUApiKey = old.WUApiKey
	}
	for i := range cfg.Cameras {
		if cfg.Cameras[i].Password == mask {
			cfg.Cameras[i].Password = ""
			for _, o := range old.Cameras {
				if o.ID == cfg.Cameras[i].ID {
					cfg.Cameras[i].Password = o.Password
				}
			}
		}
	}
	for i := range cfg.Sites {
		for _, o := range old.Sites {
			if o.ID != cfg.Sites[i].ID {
				continue
			}
			if cfg.Sites[i].Password == mask {
				cfg.Sites[i].Password = o.Password
			}
			if cfg.Sites[i].OAuthRefresh == mask {
				cfg.Sites[i].OAuthRefresh = o.OAuthRefresh
			}
		}
		if cfg.Sites[i].Password == mask {
			cfg.Sites[i].Password = ""
		}
		if cfg.Sites[i].OAuthRefresh == mask {
			cfg.Sites[i].OAuthRefresh = ""
		}
	}
	for i := range cfg.DataSources {
		if cfg.DataSources[i].Token == mask {
			cfg.DataSources[i].Token = ""
			for _, o := range old.DataSources {
				if o.ID == cfg.DataSources[i].ID {
					cfg.DataSources[i].Token = o.Token
				}
			}
		}
	}
}

func (s *Server) fillCameraPassword(cam *Camera) {
	if cam.Password != mask {
		return
	}
	cam.Password = ""
	for _, o := range s.app.store.Get().Cameras {
		if o.ID == cam.ID {
			cam.Password = o.Password
		}
	}
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, maskConfig(s.app.store.Get()))
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if !readJSON(w, r, &cfg) {
		return
	}
	old := s.app.store.Get()
	unmask(&cfg, old)
	// la porta d'ascolto cambia solo al riavvio
	if err := s.app.store.Replace(cfg); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.app.Reload()
	writeJSON(w, maskConfig(s.app.store.Get()))
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	cams, pubs, sites := s.app.sched.Status()
	sort.Slice(pubs, func(i, j int) bool {
		return pubs[i].SiteID+pubs[i].Filename < pubs[j].SiteID+pubs[j].Filename
	})
	writeJSON(w, map[string]any{
		"cameras": cams,
		"pubs":    pubs,
		"sites":   sites,
		"data":    s.app.data.Snapshot(),
		"time":    time.Now(),
	})
}

func (s *Server) testCamera(w http.ResponseWriter, r *http.Request) {
	var cam Camera
	if !readJSON(w, r, &cam) {
		return
	}
	s.fillCameraPassword(&cam)
	img, err := CaptureFrame(r.Context(), cam)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(img)
}

// overlayPreview applica i livelli della webcam (anche non ancora salvati)
// all'ultima immagine catturata, o a un'immagine di prova.
func (s *Server) overlayPreview(w http.ResponseWriter, r *http.Request) {
	var cam Camera
	if !readJSON(w, r, &cam) {
		return
	}
	tmp := Config{Cameras: []Camera{cam}}
	tmp.applyDefaults()
	cam = tmp.Cameras[0]
	raw := s.app.sched.Raw(cam.ID)
	if raw == nil {
		raw = TestPattern(1280, 720)
	}
	img, boxes, size, err := s.app.renderer.Render(raw, cam, time.Now(), PrimarySite(s.app.store.Get(), cam.ID))
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	type box struct{ X, Y, W, H int }
	out := make([]box, len(boxes))
	for i, b := range boxes {
		out[i] = box{b.Min.X, b.Min.Y, b.Dx(), b.Dy()}
	}
	noCache(w)
	writeJSON(w, map[string]any{
		"image":  "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(img),
		"width":  size.X,
		"height": size.Y,
		"boxes":  out,
	})
}

func (s *Server) testSite(w http.ResponseWriter, r *http.Request) {
	var site Site
	if !readJSON(w, r, &site) {
		return
	}
	for _, o := range s.app.store.Get().Sites {
		if o.ID == site.ID {
			if site.Password == mask {
				site.Password = o.Password
			}
			site.HostKey = o.HostKey
			site.OAuthRefresh = o.OAuthRefresh
		}
	}
	if site.Password == mask {
		site.Password = ""
	}
	tmp := Config{Sites: []Site{site}}
	tmp.applyDefaults()
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	steps, err := TestSite(ctx, s.app.store, tmp.Sites[0])
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "steps": steps})
		return
	}
	writeJSON(w, map[string]any{"ok": "Pubblicazione possibile", "steps": steps})
}

func (s *Server) testDataSource(w http.ResponseWriter, r *http.Request) {
	var ds DataSource
	if !readJSON(w, r, &ds) {
		return
	}
	if ds.Token == mask {
		ds.Token = ""
		for _, o := range s.app.store.Get().DataSources {
			if o.ID == ds.ID {
				ds.Token = o.Token
			}
		}
		if ds.Token == "" && ds.Type == "wunderground" {
			ds.Token = s.app.store.Get().WUApiKey
		}
	}
	v, err := FetchDataSource(r.Context(), ds)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, map[string]string{"value": v})
}

// mjpeg invia il flusso live come multipart/x-mixed-replace: il browser lo
// mostra in un semplice <img>, senza plugin.
func (s *Server) mjpeg(w http.ResponseWriter, r *http.Request, key string, cam Camera) {
	frames, done := s.app.live.Subscribe(key, cam)
	defer done()
	const boundary = "webcamframe"
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	noCache(w)
	flusher, _ := w.(http.Flusher)
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-timeout.C:
			// nessun fotogramma: chiudiamo così il browser mostra l'errore
			log.Printf("live %s: nessun fotogramma (%s)", key, s.app.live.Error(key))
			return
		case f := <-frames:
			timeout.Reset(30 * time.Second)
			if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", boundary, len(f)); err != nil {
				return
			}
			if _, err := w.Write(f); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\r\n"); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// ---- loghi ----

func (s *Server) listLogos(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(s.app.logoDir())
	names := []string{}
	for _, e := range entries {
		if !e.IsDir() && filenamePattern.MatchString(e.Name()) && !strings.HasSuffix(e.Name(), ".tmp") {
			names = append(names, e.Name())
		}
	}
	writeJSON(w, names)
}

// diagnostics prepara un pacchetto da allegare alle segnalazioni: versione,
// configurazione senza password, stato e log.
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) {
		fw, _ := zw.Create(name)
		_, _ = fw.Write(data)
	}
	cams, pubs, sites := s.app.sched.Status()
	_, ffErr := ffmpegPath()
	info, _ := json.MarshalIndent(map[string]any{
		"version": version, "platform": platformKey(), "hostname": hostname(),
		"ffmpeg": ffErr == nil, "time": time.Now(), "update": s.app.updater.Status(),
	}, "", "  ")
	add("info.json", info)
	cfg, _ := json.MarshalIndent(maskConfig(s.app.store.Get()), "", "  ")
	add("config-senza-password.json", cfg)
	st, _ := json.MarshalIndent(map[string]any{"cameras": cams, "pubs": pubs, "sites": sites, "data": s.app.data.Snapshot()}, "", "  ")
	add("stato.json", st)
	add("log-recente.txt", []byte(strings.Join(recentLog.Lines(), "\n")))
	for _, n := range []string{logFileName, logFileName + ".1"} {
		if data, err := os.ReadFile(filepath.Join(s.app.dataDir, n)); err == nil {
			add(n, data)
		}
	}
	_ = zw.Close()
	name := fmt.Sprintf("diagnostica-%s-%s.zip", hostname(), time.Now().Format("20060102-1504"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+unsafeChars.ReplaceAllString(name, "-")+`"`)
	_, _ = w.Write(buf.Bytes())
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *Server) uploadLogo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httpError(w, http.StatusBadRequest, "file troppo grande (max 10 MB)")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "nessun file")
		return
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		httpError(w, http.StatusBadRequest, "usa un PNG (consigliato, supporta la trasparenza) o un JPEG")
		return
	}
	name := unsafeChars.ReplaceAllString(strings.TrimSuffix(filepath.Base(hdr.Filename), filepath.Ext(hdr.Filename)), "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "logo"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	name += ext
	_ = os.MkdirAll(s.app.logoDir(), 0o755)
	if err := writeAtomic(filepath.Join(s.app.logoDir(), name), data); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.app.renderer.LoadLogo(name); err != nil {
		_ = os.Remove(filepath.Join(s.app.logoDir(), name))
		httpError(w, http.StatusBadRequest, "immagine non leggibile")
		return
	}
	writeJSON(w, map[string]string{"name": name})
}

// ---- esporta / importa (per replicare l'installazione su un altro PC) ----

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	cfgData, _ := json.MarshalIndent(s.app.store.Get(), "", "  ")
	fw, _ := zw.Create("config.json")
	_, _ = fw.Write(cfgData)
	entries, _ := os.ReadDir(s.app.logoDir())
	for _, e := range entries {
		if e.IsDir() || !filenamePattern.MatchString(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.app.logoDir(), e.Name()))
		if err != nil {
			continue
		}
		fw, _ := zw.Create("logos/" + e.Name())
		_, _ = fw.Write(data)
	}
	if err := zw.Close(); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	name := fmt.Sprintf("webcam-manager-%s-%s.zip", hostname(), time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+unsafeChars.ReplaceAllString(name, "-")+`"`)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) importZip(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 100<<20))
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		httpError(w, http.StatusBadRequest, "il file non è un export valido (.zip)")
		return
	}
	var cfg *Config
	logos := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, _ := io.ReadAll(io.LimitReader(rc, 20<<20))
		rc.Close()
		switch {
		case f.Name == "config.json":
			cfg = &Config{}
			if err := json.Unmarshal(content, cfg); err != nil {
				httpError(w, http.StatusBadRequest, "config.json non valido: "+err.Error())
				return
			}
		case strings.HasPrefix(f.Name, "logos/"):
			name := strings.TrimPrefix(f.Name, "logos/")
			if filenamePattern.MatchString(name) {
				logos[name] = content
			}
		}
	}
	if cfg == nil {
		httpError(w, http.StatusBadRequest, "config.json mancante nell'archivio")
		return
	}
	// si mantengono porta e password del pannello di questo computer
	cur := s.app.store.Get()
	cfg.Listen, cfg.AdminUser, cfg.AdminPassword = cur.Listen, cur.AdminUser, cur.AdminPassword
	if err := s.app.store.Replace(*cfg); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = os.MkdirAll(s.app.logoDir(), 0o755)
	for name, content := range logos {
		if err := writeAtomic(filepath.Join(s.app.logoDir(), name), content); err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.app.Reload()
	writeJSON(w, map[string]string{"ok": "Configurazione importata"})
}
