package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
// l'esecuzione del JavaScript e l'elenco degli indirizzi che la pagina ha
// scaricato (tra questi c'è il file con i dati della stazione meteo).
// Prova più combinazioni: alcune versioni di Edge, avviate dal servizio di
// Windows, non partono in modalità "headless=new" o con la sandbox.
func renderPage(ctx context.Context, url string) ([]byte, []string, error) {
	var bins []string
	if b := findBrowser(); b != "" {
		bins = append(bins, b)
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if base == "" {
				continue
			}
			c := filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe")
			if _, err := os.Stat(c); err == nil && c != findBrowser() {
				bins = append(bins, c)
			}
		}
	}
	if len(bins) == 0 {
		return nil, nil, errors.New("serve Microsoft Edge o Google Chrome installato sul computer")
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	var lastErr error
	for _, bin := range bins {
		for _, mode := range []string{"--headless=new", "--headless"} {
			dom, urls, err := renderOnce(ctx, bin, mode, url)
			if err == nil {
				return dom, urls, nil
			}
			lastErr = err
			if ctx.Err() != nil {
				return nil, nil, err
			}
		}
	}
	return nil, nil, lastErr
}

var reNetURL = regexp.MustCompile(`"url":"(https?://[^"]+)"`)

func renderOnce(ctx context.Context, bin, mode, url string) ([]byte, []string, error) {
	profile, err := os.MkdirTemp("", "wm-browser-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(profile)
	netlog := filepath.Join(profile, "net.json")
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	args := []string{mode, "--no-sandbox", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--disable-sync", "--mute-audio", "--hide-scrollbars", "--user-data-dir=" + profile,
		"--log-net-log=" + netlog, "--virtual-time-budget=15000", "--dump-dom", url}
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		if ctx.Err() != nil {
			return nil, nil, errors.New("il browser non ha caricato la pagina in tempo")
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 200 {
			msg = msg[len(msg)-200:]
		}
		return nil, nil, fmt.Errorf("%s (%s): %v %s", filepath.Base(bin), mode, err, msg)
	}
	var urls []string
	if b, err := os.ReadFile(netlog); err == nil {
		seen := map[string]bool{}
		for _, m := range reNetURL.FindAllStringSubmatch(string(b), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				urls = append(urls, m[1])
			}
		}
	}
	return out, urls, nil
}
