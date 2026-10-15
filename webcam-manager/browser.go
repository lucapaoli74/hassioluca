package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Alcune pagine (es. le pagine meteo dei parchi) caricano i valori con
// JavaScript: nel codice HTML non ci sono. Per leggerle come le vede una
// persona si usa il browser del computer (Microsoft Edge su Windows, oppure
// Chrome/Chromium) in modalità invisibile: apre la pagina, lascia girare il
// JavaScript e restituisce la pagina finale.

var (
	browserOnce sync.Once
	browserBin  string
	browserMu   sync.Mutex // una pagina alla volta: il browser è pesante
)

func findBrowser() string {
	browserOnce.Do(func() {
		if p := os.Getenv("WEBCAM_MANAGER_BROWSER"); p != "" {
			browserBin = p
			return
		}
		var cands []string
		if runtime.GOOS == "windows" {
			for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LocalAppData")} {
				if base == "" {
					continue
				}
				cands = append(cands,
					filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
					filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
			}
		} else {
			for _, n := range []string{"chromium", "chromium-browser", "google-chrome", "microsoft-edge"} {
				if p, err := exec.LookPath(n); err == nil {
					cands = append(cands, p)
				}
			}
			cands = append(cands, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
		}
		for _, c := range cands {
			if _, err := os.Stat(c); err == nil {
				browserBin = c
				return
			}
		}
	})
	return browserBin
}

// renderPage apre la pagina nel browser invisibile e restituisce l'HTML dopo
// l'esecuzione del JavaScript (al massimo circa 15 secondi).
func renderPage(ctx context.Context, url string) ([]byte, error) {
	bin := findBrowser()
	if bin == "" {
		return nil, errors.New("serve Microsoft Edge o Google Chrome installato sul computer")
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	profile, err := os.MkdirTemp("", "wm-browser-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	args := []string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--mute-audio", "--hide-scrollbars", "--user-data-dir=" + profile,
		"--virtual-time-budget=15000", "--dump-dom", url}
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		if ctx.Err() != nil {
			return nil, errors.New("il browser non ha caricato la pagina in tempo")
		}
		return nil, fmt.Errorf("browser: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
