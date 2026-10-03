package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Config è l'intero stato persistente dell'applicazione: un solo file JSON
// che, copiato insieme all'eseguibile, replica l'installazione su un altro PC.
type Config struct {
	SchemaVersion int            `json:"schema_version"` // formato del file, per le migrazioni automatiche
	Location      Location       `json:"location"`       // nome e altitudine della località
	WUApiKey      string         `json:"wu_api_key"`     // chiave API Weather Underground (ricerca stazioni)
	Update        UpdateSettings `json:"update"`         // aggiornamenti automatici del programma
	Alerts        AlertSettings  `json:"alerts"`         // email in caso di errori
	Listen        string         `json:"listen"`         // indirizzo del pannello, es. ":8080"
	AdminUser     string         `json:"admin_user"`     // utente del pannello
	AdminPassword string         `json:"admin_password"` // password del pannello
	Cameras       []Camera       `json:"cameras"`
	Sites         []Site         `json:"sites"`
	DataSources   []DataSource   `json:"data_sources"` // dati live per le sovrimpressioni
}

// Location identifica l'installazione: compare nelle scritte, nel titolo del
// mini-sito e nelle email.
type Location struct {
	Name      string  `json:"name"`      // es. "Camping Coggiolo Sant'Anna Pelago (MO)"
	Altitude  int     `json:"altitude"`  // metri sul livello del mare
	Latitude  float64 `json:"latitude"`  // facoltative: default per meteo e previsioni
	Longitude float64 `json:"longitude"` //
}

// Camera descrive una sorgente di immagini.
type Camera struct {
	ID               string    `json:"id"`                // identificativo breve (lettere, numeri, - e _)
	Name             string    `json:"name"`              // nome leggibile
	Enabled          bool      `json:"enabled"`           //
	Source           string    `json:"source"`            // "snapshot" | "mjpeg" | "rtsp"
	URL              string    `json:"url"`               // URL dello snapshot/stream
	Username         string    `json:"username"`          // credenziali della telecamera (opzionali)
	Password         string    `json:"password"`          //
	IntervalSeconds  int       `json:"interval_seconds"`  // ogni quanto catturare
	MaxWidth         int       `json:"max_width"`         // ridimensiona se più larga (0 = originale)
	Quality          int       `json:"quality"`           // qualità JPEG 1-100
	Public           bool      `json:"public"`            // esposta su /public/<id>.jpg senza password
	ActiveFrom       string    `json:"active_from"`       // fascia oraria di attività "HH:MM" ("" = sempre)
	ActiveTo         string    `json:"active_to"`         //
	ArchiveMinutes   int       `json:"archive_minutes"`   // frequenza dello storico (0 = storico disattivato)
	RetentionDays    int       `json:"retention_days"`    // conserva lo storico locale per N giorni (0 = per sempre)
	RetentionMB      int       `json:"retention_mb"`      // oppure: spazio massimo dello storico locale in MB (0 = nessun limite)
	TimelapseMinutes int       `json:"timelapse_minutes"` // timelapse: un fotogramma ogni N minuti (0 = disattivato)
	TimelapseHours   int       `json:"timelapse_hours"`   // timelapse: durata in ore (default 6)
	Overlays         []Overlay `json:"overlays"`          // livelli in sovrimpressione, disegnati in ordine
}

// Overlay è un livello disegnato sopra l'immagine: una scritta (con
// segnaposto per data/ora e dati live) oppure un logo.
// Dimensioni e margini sono in percentuale, così il risultato è identico a
// qualsiasi risoluzione della telecamera.
type Overlay struct {
	Type       string  `json:"type"`       // "text" | "image" | "forecast"
	Enabled    bool    `json:"enabled"`    //
	Text       string  `json:"text"`       // testo; segnaposto: {name} {date} {time} {data:ID} ...
	Image      string  `json:"image"`      // nome del logo caricato (cartella data/logos)
	Anchor     string  `json:"anchor"`     // top-left, top-center, top-right, middle-left, center, middle-right, bottom-left, bottom-center, bottom-right
	OffsetX    float64 `json:"offset_x"`   // margine orizzontale, % della larghezza
	OffsetY    float64 `json:"offset_y"`   // margine verticale, % dell'altezza
	Size       float64 `json:"size"`       // testo: altezza carattere in % dell'altezza; logo: larghezza in % della larghezza
	Color      string  `json:"color"`      // colore testo, es. "#ffffff"
	Bold       bool    `json:"bold"`       //
	Shadow     bool    `json:"shadow"`     // ombra dietro il testo
	Background string  `json:"background"` // colore riquadro di sfondo ("" = nessuno)
	BgOpacity  float64 `json:"bg_opacity"` // 0-1
	Opacity    float64 `json:"opacity"`    // 0-1, opacità del testo o del logo
	FullWidth  bool    `json:"full_width"` // lo sfondo occupa tutta la larghezza (fascia)

	// solo per "forecast" (previsioni del tempo, dati Open-Meteo)
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Days          int     `json:"days"`           // giorni da mostrare (1-5)
	StartTomorrow bool    `json:"start_tomorrow"` // parti da domani invece che da oggi
	ShowRain      bool    `json:"show_rain"`      // mostra la probabilità di pioggia
}

// DataSource è un valore live (es. temperatura) aggiornato periodicamente e
// usabile nelle scritte con il segnaposto {data:ID}.
type DataSource struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`            // "open_meteo" | "wunderground" | "home_assistant" | "http_json" | "http_text"
	URL            string  `json:"url"`             // HA: indirizzo di Home Assistant; http_*: URL da leggere
	Token          string  `json:"token"`           // HA: token di lunga durata; http_*: bearer token opzionale
	Entity         string  `json:"entity"`          // HA: entity_id, es. sensor.temperatura_esterna
	Path           string  `json:"path"`            // http_json / HA: campo da leggere, es. "main.temp" o "attributes.humidity"
	Latitude       float64 `json:"latitude"`        // open_meteo
	Longitude      float64 `json:"longitude"`       // open_meteo
	Variable       string  `json:"variable"`        // open_meteo: temperature_2m, relative_humidity_2m, wind_speed_10m, ...
	Station        string  `json:"station"`         // wunderground: ID stazione, es. ILERICI12
	Decimals       int     `json:"decimals"`        // cifre decimali per i numeri (-1 = come arriva)
	Unit           string  `json:"unit"`            // suffisso, es. "°C"
	RefreshSeconds int     `json:"refresh_seconds"` // ogni quanto aggiornare
}

// Site è una destinazione dove pubblicare le immagini.
type Site struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Enabled       bool          `json:"enabled"`
	Protocol      string        `json:"protocol"` // "ftp" | "ftps" | "sftp" | "folder" | "onedrive"
	Host          string        `json:"host"`
	Port          int           `json:"port"`
	Username      string        `json:"username"`
	Password      string        `json:"password"`
	RemoteDir     string        `json:"remote_dir"` // cartella remota (o locale per "folder")
	HostKey       string        `json:"host_key"`   // impronta SSH memorizzata al primo collegamento (sftp)
	Publications  []Publication `json:"publications"`
	PublicURL     string        `json:"public_url"`      // indirizzo web della cartella, es. https://www.miosito.it/webcam/
	OAuthClientID string        `json:"oauth_client_id"` // onedrive: ID applicazione registrata su Azure
	OAuthTenant   string        `json:"oauth_tenant"`    // onedrive: "consumers" (account personale) o "organizations"
	OAuthRefresh  string        `json:"oauth_refresh"`   // onedrive: credenziale ottenuta collegando l'account
	History       bool          `json:"history"`         // pubblica anche il mini-sito con lo storico
	HistoryDir    string        `json:"history_dir"`     // sottocartella del mini-sito (default "storico")
	HistoryTitle  string        `json:"history_title"`
	HistoryIndex  string        `json:"history_index"` // nome della pagina del mini-sito (default index.html)
}

// Publication collega una webcam a un file sul sito, con la sua periodicità.
type Publication struct {
	CameraID        string `json:"camera_id"`
	Filename        string `json:"filename"`         // es. "webcam-porto.jpg"
	IntervalSeconds int    `json:"interval_seconds"` // ogni quanto caricare (0 = a ogni cattura)
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var filenamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Store protegge la configurazione e la salva su disco.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func randomPassword() string {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// LoadStore legge il file di configurazione; se non esiste ne crea uno nuovo
// con una password amministratore casuale. Il secondo valore è true se il file
// è stato appena creato.
func LoadStore(path string) (*Store, bool, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	created := false
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.cfg = Config{Listen: ":8080", AdminUser: "admin", AdminPassword: randomPassword()}
		created = true
	case err != nil:
		return nil, false, err
	default:
		if err := json.Unmarshal(data, &s.cfg); err != nil {
			return nil, false, fmt.Errorf("%s non è un JSON valido: %w", path, err)
		}
	}
	migrated := s.cfg.migrate()
	s.cfg.applyDefaults()
	if err := s.cfg.Validate(); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	if migrated && !created {
		s.backup()
	}
	if created || migrated {
		if err := s.save(); err != nil {
			return nil, false, err
		}
	}
	return s, created, nil
}

// currentSchema è la versione attuale del formato del file di configurazione.
// Quando un aggiornamento cambia il formato si incrementa e si aggiunge qui
// sotto il passo di migrazione: le installazioni esistenti vengono convertite
// da sole al primo avvio (dopo una copia di sicurezza).
const currentSchema = 1

func (c *Config) migrate() bool {
	if c.SchemaVersion >= currentSchema {
		return false
	}
	// 0 -> 1: introdotto il numero di versione; nessuna conversione necessaria.
	c.SchemaVersion = currentSchema
	return true
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.AdminUser == "" {
		c.AdminUser = "admin"
	}
	if c.AdminPassword == "" {
		c.AdminPassword = randomPassword()
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = currentSchema
	}
	if c.Alerts.AfterMinutes <= 0 {
		c.Alerts.AfterMinutes = 10
	}
	if c.Alerts.Security == "" {
		c.Alerts.Security = "starttls"
	}
	if c.Update.CheckHours <= 0 {
		c.Update.CheckHours = 6
	}
	if c.Cameras == nil {
		c.Cameras = []Camera{}
	}
	if c.Sites == nil {
		c.Sites = []Site{}
	}
	if c.DataSources == nil {
		c.DataSources = []DataSource{}
	}
	for i := range c.DataSources {
		if c.DataSources[i].RefreshSeconds <= 0 {
			c.DataSources[i].RefreshSeconds = 300
		}
	}
	for i := range c.Cameras {
		cam := &c.Cameras[i]
		if cam.Source == "" {
			cam.Source = "snapshot"
		}
		if cam.IntervalSeconds <= 0 {
			cam.IntervalSeconds = 60
		}
		if cam.Quality <= 0 || cam.Quality > 100 {
			cam.Quality = 85
		}
		if cam.Overlays == nil {
			cam.Overlays = []Overlay{}
		}
		for j := range cam.Overlays {
			o := &cam.Overlays[j]
			if o.Anchor == "" {
				o.Anchor = "bottom-left"
			}
			if o.Size <= 0 {
				if o.Type == "image" {
					o.Size = 15
				} else if o.Type == "forecast" {
					o.Size = 30
				} else {
					o.Size = 4
				}
			}
			if o.Color == "" {
				o.Color = "#ffffff"
			}
			if o.Opacity <= 0 || o.Opacity > 1 {
				o.Opacity = 1
			}
			if o.BgOpacity < 0 || o.BgOpacity > 1 {
				o.BgOpacity = 0.5
			}
		}
	}
	for i := range c.Sites {
		site := &c.Sites[i]
		if site.Publications == nil {
			site.Publications = []Publication{}
		}
		if site.HistoryDir == "" {
			site.HistoryDir = "storico"
		}
		if site.HistoryIndex == "" {
			site.HistoryIndex = "index.html"
		}
		if site.Port == 0 {
			switch site.Protocol {
			case "ftp", "ftps":
				site.Port = 21
			case "sftp":
				site.Port = 22
			}
		}
	}
}

// Validate controlla la coerenza della configurazione prima di accettarla.
func (c *Config) Validate() error {
	cams := map[string]bool{}
	for _, cam := range c.Cameras {
		if !idPattern.MatchString(cam.ID) {
			return fmt.Errorf("webcam %q: ID non valido (usa lettere, numeri, - e _)", cam.ID)
		}
		if cams[cam.ID] {
			return fmt.Errorf("webcam %q: ID duplicato", cam.ID)
		}
		cams[cam.ID] = true
		switch cam.Source {
		case "snapshot", "mjpeg", "rtsp":
		default:
			return fmt.Errorf("webcam %q: sorgente %q sconosciuta", cam.ID, cam.Source)
		}
		if cam.URL == "" {
			return fmt.Errorf("webcam %q: URL mancante", cam.ID)
		}
		if cam.IntervalSeconds < 5 {
			return fmt.Errorf("webcam %q: intervallo minimo 5 secondi", cam.ID)
		}
		if cam.TimelapseMinutes < 0 || cam.TimelapseHours < 0 || cam.TimelapseHours > 72 ||
			(cam.TimelapseMinutes > 0 && cam.TimelapseMinutes*500 < max(cam.TimelapseHours, 6)*60) {
			return fmt.Errorf("webcam %q: timelapse fino a 72 ore e al massimo 500 fotogrammi", cam.ID)
		}
		if cam.ArchiveMinutes < 0 || (cam.ArchiveMinutes > 0 && 1440%cam.ArchiveMinutes != 0) {
			return fmt.Errorf("webcam %q: la frequenza dello storico deve dividere le 24 ore (es. 10, 15, 30, 60, 120)", cam.ID)
		}
		for _, t := range []string{cam.ActiveFrom, cam.ActiveTo} {
			if _, ok := parseClock(t); t != "" && !ok {
				return fmt.Errorf("webcam %q: orario %q non valido (usa HH:MM)", cam.ID, t)
			}
		}
	}
	dsIDs := map[string]bool{}
	for _, ds := range c.DataSources {
		if dsIDs[ds.ID] {
			return fmt.Errorf("dato live %q: ID duplicato", ds.ID)
		}
		dsIDs[ds.ID] = true
		if !idPattern.MatchString(ds.ID) {
			return fmt.Errorf("dato live %q: ID non valido (usa lettere, numeri, - e _)", ds.ID)
		}
		switch ds.Type {
		case "open_meteo":
			if ds.Variable == "" {
				return fmt.Errorf("dato live %q: scegli la grandezza meteo", ds.ID)
			}
		case "wunderground":
			if ds.Station == "" || ds.Variable == "" || (ds.Token == "" && c.WUApiKey == "") {
				return fmt.Errorf("dato live %q: servono ID stazione, chiave API e grandezza", ds.ID)
			}
		case "home_assistant":
			if ds.URL == "" || ds.Entity == "" || ds.Token == "" {
				return fmt.Errorf("dato live %q: servono indirizzo, token ed entità di Home Assistant", ds.ID)
			}
		case "http_json", "http_text":
			if ds.URL == "" {
				return fmt.Errorf("dato live %q: URL mancante", ds.ID)
			}
		default:
			return fmt.Errorf("dato live %q: tipo %q sconosciuto", ds.ID, ds.Type)
		}
	}
	for _, cam := range c.Cameras {
		for i, o := range cam.Overlays {
			switch o.Type {
			case "text", "forecast":
			case "image":
				if o.Image != "" && !filenamePattern.MatchString(o.Image) {
					return fmt.Errorf("webcam %q, livello %d: nome logo non valido", cam.ID, i+1)
				}
			default:
				return fmt.Errorf("webcam %q, livello %d: tipo %q sconosciuto", cam.ID, i+1, o.Type)
			}
			if _, ok := anchors[o.Anchor]; !ok {
				return fmt.Errorf("webcam %q, livello %d: posizione %q sconosciuta", cam.ID, i+1, o.Anchor)
			}
		}
	}
	sites := map[string]bool{}
	for _, s := range c.Sites {
		if !idPattern.MatchString(s.ID) {
			return fmt.Errorf("sito %q: ID non valido (usa lettere, numeri, - e _)", s.ID)
		}
		if sites[s.ID] {
			return fmt.Errorf("sito %q: ID duplicato", s.ID)
		}
		sites[s.ID] = true
		switch s.Protocol {
		case "ftp", "ftps", "sftp":
			if s.Host == "" {
				return fmt.Errorf("sito %q: host mancante", s.ID)
			}
		case "folder":
			if s.RemoteDir == "" {
				return fmt.Errorf("sito %q: cartella mancante", s.ID)
			}
		case "onedrive":
			if s.OAuthClientID == "" {
				return fmt.Errorf("sito %q: serve l'ID applicazione Microsoft", s.ID)
			}
		default:
			return fmt.Errorf("sito %q: protocollo %q sconosciuto", s.ID, s.Protocol)
		}
		for _, p := range s.Publications {
			if !cams[p.CameraID] {
				return fmt.Errorf("sito %q: la webcam %q non esiste", s.ID, p.CameraID)
			}
			if !filenamePattern.MatchString(p.Filename) {
				return fmt.Errorf("sito %q: nome file %q non valido", s.ID, p.Filename)
			}
		}
		if !filenamePattern.MatchString(s.HistoryIndex) {
			return fmt.Errorf("sito %q: nome pagina storico %q non valido", s.ID, s.HistoryIndex)
		}
		if !filenamePattern.MatchString(s.HistoryDir) {
			return fmt.Errorf("sito %q: cartella storico %q non valida", s.ID, s.HistoryDir)
		}
	}
	return nil
}

// Get restituisce una copia profonda della configurazione.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.clone()
}

func (c Config) clone() Config {
	data, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(data, &out)
	return out
}

// Replace valida, salva e attiva una nuova configurazione.
func (s *Store) Replace(cfg Config) error {
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backup()
	old := s.cfg
	s.cfg = cfg
	if err := s.save(); err != nil {
		s.cfg = old
		return err
	}
	return nil
}

// SetOAuthRefresh salva la credenziale OneDrive di un sito.
func (s *Store) SetOAuthRefresh(siteID, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Sites {
		if s.cfg.Sites[i].ID == siteID {
			s.cfg.Sites[i].OAuthRefresh = token
			_ = s.save()
		}
	}
}

// SetHostKey memorizza l'impronta SSH di un sito (trust on first use).
func (s *Store) SetHostKey(siteID, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Sites {
		if s.cfg.Sites[i].ID == siteID && s.cfg.Sites[i].HostKey == "" {
			s.cfg.Sites[i].HostKey = key
			_ = s.save()
		}
	}
}

// save scrive il file in modo atomico (file temporaneo + rename).
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// parseClock converte "HH:MM" in minuti dalla mezzanotte.
func parseClock(s string) (int, bool) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// IsActive dice se la webcam deve lavorare all'orario indicato.
// La fascia può scavalcare la mezzanotte (es. 20:00-06:00).
func (c Camera) IsActive(t time.Time) bool {
	from, ok1 := parseClock(c.ActiveFrom)
	to, ok2 := parseClock(c.ActiveTo)
	if !ok1 || !ok2 || from == to {
		return true
	}
	now := t.Hour()*60 + t.Minute()
	if from < to {
		return now >= from && now < to
	}
	return now >= from || now < to
}

const maxBackups = 50

func (s *Store) backupDir() string { return filepath.Join(filepath.Dir(s.path), "backups") }

// backup copia il file attuale in backups/ prima di ogni modifica, così
// qualsiasi cambiamento fatto durante le prove si può annullare dal pannello.
func (s *Store) backup() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	dir := s.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := "config-" + time.Now().Format("20060102-150405.000") + ".json"
	_ = os.WriteFile(filepath.Join(dir, name), data, 0o600)
	list := s.backupsLocked()
	for i := maxBackups; i < len(list); i++ {
		_ = os.Remove(filepath.Join(dir, list[i]))
	}
}

func (s *Store) backupsLocked() []string {
	entries, _ := os.ReadDir(s.backupDir())
	var out []string
	for _, e := range entries {
		if backupName.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out))) // più recenti prima
	return out
}

var backupName = regexp.MustCompile(`^config-\d{8}-\d{6}\.\d{3}\.json$`)

// Backups elenca le copie di sicurezza, dalla più recente.
func (s *Store) Backups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backupsLocked()
}

// LoadBackup legge una copia di sicurezza.
func (s *Store) LoadBackup(name string) (Config, error) {
	var cfg Config
	if !backupName.MatchString(name) {
		return cfg, fmt.Errorf("nome non valido")
	}
	data, err := os.ReadFile(filepath.Join(s.backupDir(), name))
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.migrate()
	return cfg, nil
}
