package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpdateSettings dice dove cercare le nuove versioni del programma.
//
// Ogni release contiene un manifest "latest.json":
//
//	{"version":"1.2.0","notes":"...","assets":{"windows-amd64":{"file":"webcam-manager-windows-amd64.exe","sha256":"..."}}}
//
// Con source "github" il manifest e i file sono gli allegati dell'ultima
// release del repository; con source "url" il manifest è a un indirizzo
// qualsiasi (es. sul proprio sito FTP) e i file sono relativi a esso.
type UpdateSettings struct {
	Source     string `json:"source"`      // "github" | "url" | "" (disattivato)
	Repo       string `json:"repo"`        // github: "proprietario/repository"
	Token      string `json:"token"`       // github: token in sola lettura (solo per repository privati)
	URL        string `json:"url"`         // url: indirizzo di latest.json
	Auto       bool   `json:"auto"`        // installa da solo le nuove versioni
	CheckHours int    `json:"check_hours"` // ogni quante ore controllare
}

type manifestAsset struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	Version string                   `json:"version"`
	Notes   string                   `json:"notes"`
	Assets  map[string]manifestAsset `json:"assets"`
}

// UpdateStatus è lo stato mostrato in Impostazioni → Aggiornamenti.
type UpdateStatus struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	Notes     string    `json:"notes"`
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checked_at"`
	Error     string    `json:"error"`
	Busy      bool      `json:"busy"`
	Platform  string    `json:"platform"`
}

type Updater struct {
	store   *Store
	restart func() // chiamata dopo aver sostituito l'eseguibile

	mu     sync.Mutex
	status UpdateStatus
	ghRel  *ghRelease // ultima release letta (modalità github)
	man    *manifest
}

func platformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

func NewUpdater(store *Store, restart func()) *Updater {
	cleanupOldExecutable()
	return &Updater{store: store, restart: restart, status: UpdateStatus{Current: version, Platform: platformKey()}}
}

func (u *Updater) Status() UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// Loop controlla periodicamente e, se richiesto, installa da solo.
func (u *Updater) Loop(ctx context.Context) {
	select { // lascia partire tutto il resto prima del primo controllo
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	for {
		st := u.store.Get().Update
		if st.Source != "" {
			if err := u.Check(ctx); err == nil && st.Auto && u.Status().Available {
				log.Printf("aggiornamento automatico alla versione %s", u.Status().Latest)
				if err := u.Apply(ctx); err != nil {
					log.Printf("aggiornamento non riuscito: %v", err)
				}
			}
		}
		hours := st.CheckHours
		if hours <= 0 {
			hours = 6
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(hours) * time.Hour):
		}
	}
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"url"` // API URL: con Accept octet-stream scarica il file
	} `json:"assets"`
}

func (u *Updater) get(ctx context.Context, rawURL string, st UpdateSettings, binary bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "webcam-manager/"+version)
	if st.Source == "github" {
		if st.Token != "" {
			req.Header.Set("Authorization", "Bearer "+st.Token)
		}
		if binary {
			req.Header.Set("Accept", "application/octet-stream")
		} else {
			req.Header.Set("Accept", "application/vnd.github+json")
		}
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound && st.Source == "github" {
			return nil, fmt.Errorf("release non trovata (repository privato? serve un token)")
		}
		return nil, fmt.Errorf("%s risponde %s", req.URL.Host, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

// Check legge l'ultima versione disponibile.
func (u *Updater) Check(ctx context.Context) error {
	st := u.store.Get().Update
	man, rel, err := u.fetchManifest(ctx, st)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status.CheckedAt = time.Now()
	if err != nil {
		u.status.Error = err.Error()
		return err
	}
	u.status.Error = ""
	u.status.Latest = man.Version
	u.status.Notes = man.Notes
	_, hasAsset := man.Assets[platformKey()]
	u.status.Available = hasAsset && compareVersions(man.Version, version) > 0
	u.man, u.ghRel = man, rel
	return nil
}

func (u *Updater) fetchManifest(ctx context.Context, st UpdateSettings) (*manifest, *ghRelease, error) {
	var data []byte
	var rel *ghRelease
	switch st.Source {
	case "github":
		if !strings.Contains(st.Repo, "/") {
			return nil, nil, errors.New("indica il repository come proprietario/nome")
		}
		body, err := u.get(ctx, "https://api.github.com/repos/"+st.Repo+"/releases/latest", st, false)
		if err != nil {
			return nil, nil, err
		}
		rel = &ghRelease{}
		if err := json.Unmarshal(body, rel); err != nil {
			return nil, nil, err
		}
		assetURL := ""
		for _, a := range rel.Assets {
			if a.Name == "latest.json" {
				assetURL = a.URL
			}
		}
		if assetURL == "" {
			return nil, nil, fmt.Errorf("la release %s non contiene latest.json", rel.TagName)
		}
		if data, err = u.get(ctx, assetURL, st, true); err != nil {
			return nil, nil, err
		}
	case "url":
		var err error
		if data, err = u.get(ctx, st.URL, st, false); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, errors.New("aggiornamenti disattivati")
	}
	var man manifest
	if err := json.Unmarshal(data, &man); err != nil {
		return nil, nil, fmt.Errorf("latest.json non valido: %w", err)
	}
	if man.Version == "" {
		return nil, nil, errors.New("latest.json senza versione")
	}
	return &man, rel, nil
}

// Apply scarica, verifica e installa la versione indicata dall'ultimo Check.
func (u *Updater) Apply(ctx context.Context) error {
	u.mu.Lock()
	if u.status.Busy {
		u.mu.Unlock()
		return errors.New("aggiornamento già in corso")
	}
	u.status.Busy = true
	man, rel := u.man, u.ghRel
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.status.Busy = false
		u.mu.Unlock()
	}()
	if man == nil {
		return errors.New("controlla prima la disponibilità di aggiornamenti")
	}
	asset, ok := man.Assets[platformKey()]
	if !ok {
		return fmt.Errorf("nessun eseguibile per %s nella versione %s", platformKey(), man.Version)
	}
	st := u.store.Get().Update
	var fileURL string
	switch st.Source {
	case "github":
		for _, a := range rel.Assets {
			if a.Name == asset.File {
				fileURL = a.URL
			}
		}
		if fileURL == "" {
			return fmt.Errorf("file %s non trovato nella release", asset.File)
		}
	case "url":
		base, err := url.Parse(st.URL)
		if err != nil {
			return err
		}
		ref, err := url.Parse(asset.File)
		if err != nil {
			return err
		}
		fileURL = base.ResolveReference(ref).String()
	}
	data, err := u.get(ctx, fileURL, st, true)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), asset.SHA256) {
		return errors.New("impronta SHA-256 non corrispondente: file corrotto, aggiornamento annullato")
	}
	if err := InstallExecutable(data); err != nil {
		return err
	}
	log.Printf("installata la versione %s: riavvio", man.Version)
	go func() { time.Sleep(time.Second); u.restart() }()
	return nil
}

// InstallExecutable sostituisce l'eseguibile in uso con quello nuovo.
// Windows permette di rinominare un eseguibile in esecuzione: quello vecchio
// diventa ".old" e viene cancellato al prossimo avvio.
func InstallExecutable(data []byte) error {
	if !looksExecutable(data) {
		return fmt.Errorf("il file non è un eseguibile per %s", platformKey())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	newPath, oldPath := exe+".new", exe+".old"
	if err := os.WriteFile(newPath, data, 0o755); err != nil {
		return fmt.Errorf("scrittura non permessa nella cartella del programma: %w", err)
	}
	_ = os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(newPath)
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(oldPath, exe) // ripristina
		return err
	}
	return nil
}

func looksExecutable(data []byte) bool {
	switch runtime.GOOS {
	case "windows":
		return bytes.HasPrefix(data, []byte("MZ"))
	case "linux":
		return bytes.HasPrefix(data, []byte("\x7fELF"))
	case "darwin":
		return len(data) > 4 && (bytes.HasPrefix(data, []byte{0xcf, 0xfa, 0xed, 0xfe}) || bytes.HasPrefix(data, []byte{0xca, 0xfe, 0xba, 0xbe}))
	}
	return len(data) > 0
}

func cleanupOldExecutable() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

// compareVersions confronta versioni tipo "v1.2.3"; "dev" è la più vecchia.
func compareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}
		}
		out[i] = n
	}
	return out
}
