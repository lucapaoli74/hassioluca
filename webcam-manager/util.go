package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const logFileName = "webcam-manager.log"

// impostate da "install -admin-password ... -location ... -altitude ..."
var (
	adminPasswordFlag string
	locationFlag      string
	altitudeFlag      = -1
)

// prepareDataDir crea la configurazione prima di installare il servizio e
// applica porta e password scelte durante l'installazione.
func prepareDataDir(dataDir, listen string) error {
	store, _, err := LoadStore(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return err
	}
	cfg := store.Get()
	changed := false
	if listen != "" && cfg.Listen != listen {
		cfg.Listen, changed = listen, true
	}
	if adminPasswordFlag != "" {
		cfg.AdminPassword, changed = adminPasswordFlag, true
	}
	if locationFlag != "" {
		cfg.Location.Name, changed = locationFlag, true
	}
	if altitudeFlag >= 0 {
		cfg.Location.Altitude, changed = altitudeFlag, true
	}
	if changed {
		return store.Replace(cfg)
	}
	return nil
}

func printAccess(dataDir string) {
	store, _, err := LoadStore(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return
	}
	cfg := store.Get()
	port := cfg.Listen
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	fmt.Printf("\nPannello:  http://localhost:%s\nUtente:    %s\nPassword:  %s\nDati:      %s\n\n", port, cfg.AdminUser, cfg.AdminPassword, dataDir)
}

// ringLog conserva le ultime righe di log per la pagina Diagnostica.
type ringLog struct {
	mu    sync.Mutex
	lines []string
	part  string
}

var recentLog = &ringLog{}

func (r *ringLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.part + string(p)
	parts := strings.Split(s, "\n")
	r.part = parts[len(parts)-1]
	r.lines = append(r.lines, parts[:len(parts)-1]...)
	if len(r.lines) > 500 {
		r.lines = r.lines[len(r.lines)-500:]
	}
	return len(p), nil
}

func (r *ringLog) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func init() {
	log.SetOutput(io.MultiWriter(os.Stderr, recentLog))
}

// setupFileLog scrive il log anche su file nella cartella dati (usato dal
// servizio, che non ha una console). Oltre 5 MB il file viene ruotato.
func setupFileLog(dataDir string) {
	_ = os.MkdirAll(dataDir, 0o755)
	f, err := openRotating(filepath.Join(dataDir, logFileName), 5<<20)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, recentLog))
}
