package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CamStatus è lo stato di una webcam mostrato nel pannello.
type CamStatus struct {
	LastCapture  time.Time `json:"last_capture"`
	LastError    string    `json:"last_error"`
	LastErrorAt  time.Time `json:"last_error_at"`
	FailingSince time.Time `json:"failing_since"` // primo errore dopo l'ultima cattura riuscita
	Captures     int       `json:"captures"`
	OutOfHours   bool      `json:"out_of_hours"`
	ArchiveCount int       `json:"archive_count"`
	ArchiveBytes int64     `json:"archive_bytes"`
}

// PubStatus è lo stato di una pubblicazione (webcam -> file su un sito).
type PubStatus struct {
	SiteID       string    `json:"site_id"`
	CameraID     string    `json:"camera_id"`
	Filename     string    `json:"filename"`
	LastUpload   time.Time `json:"last_upload"`
	LastError    string    `json:"last_error"`
	LastTry      time.Time `json:"last_try"`
	FailingSince time.Time `json:"failing_since"`
}

// SiteStatus riassume lo stato del mini-sito storico di un sito.
type SiteStatus struct {
	LastHistorySync time.Time `json:"last_history_sync"`
	HistoryError    string    `json:"history_error"`
	HistoryPending  int       `json:"history_pending"`
	FailingSince    time.Time `json:"failing_since"`
}

// historyBatch limita quante immagini dello storico caricare per ciclo, così
// un sito nuovo (o tornato raggiungibile) recupera l'arretrato gradualmente
// senza bloccare la pubblicazione dell'immagine corrente.
const historyBatch = 60

type Scheduler struct {
	store    *Store
	renderer *Renderer
	archive  *Archive
	tl       *Timelapse
	sync     *SyncState
	dataDir  string

	mu         sync.Mutex
	latest     map[string][]byte // ultima immagine elaborata, per webcam
	raw        map[string][]byte // ultima immagine grezza (anteprima sovrimpressioni)
	camStatus  map[string]*CamStatus
	pubStatus  map[string]*PubStatus
	siteStatus map[string]*SiteStatus
	lastPub    map[string]time.Time
	siteLocks  map[string]*sync.Mutex
	busy       map[string]bool  // sito|webcam con un invio in coda o in corso
	tlSent     map[string]int64 // sito|webcam -> ultimo fotogramma timelapse inviato
	assetsDone map[string]bool  // mini-site: index/cameras già caricati dopo l'ultimo riavvio
	triggers   map[string]chan struct{}
	diskLow    bool   // spazio su disco sotto la soglia: storico sospeso
	diskFree   uint64 //

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewScheduler(store *Store, r *Renderer, a *Archive, tl *Timelapse, s *SyncState, dataDir string) *Scheduler {
	sc := &Scheduler{
		store: store, renderer: r, archive: a, tl: tl, sync: s, dataDir: dataDir, tlSent: map[string]int64{},
		latest: map[string][]byte{}, raw: map[string][]byte{},
		camStatus: map[string]*CamStatus{}, pubStatus: map[string]*PubStatus{},
		siteStatus: map[string]*SiteStatus{}, lastPub: map[string]time.Time{},
		siteLocks: map[string]*sync.Mutex{}, busy: map[string]bool{},
	}
	// ricarica le ultime immagini salvate, così pannello e URL pubblici
	// mostrano subito qualcosa dopo un riavvio
	for _, cam := range store.Get().Cameras {
		if data, err := os.ReadFile(sc.latestPath(cam.ID)); err == nil {
			sc.latest[cam.ID] = data
		}
	}
	return sc
}

func (s *Scheduler) latestPath(id string) string {
	return filepath.Join(s.dataDir, "latest", id+".jpg")
}

// Restart ferma tutti i lavori e li riavvia con la configurazione attuale.
func (s *Scheduler) Restart() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
	cfg := s.store.Get()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.mu.Lock()
	s.triggers = map[string]chan struct{}{}
	s.assetsDone = map[string]bool{}
	for _, cam := range cfg.Cameras {
		if cam.Enabled {
			s.triggers[cam.ID] = make(chan struct{}, 1)
		}
	}
	s.mu.Unlock()

	for _, cam := range cfg.Cameras {
		if !cam.Enabled {
			continue
		}
		s.wg.Add(1)
		go s.worker(ctx, cam.ID, s.triggers[cam.ID])
	}
}

// Trigger chiede una cattura immediata della webcam.
func (s *Scheduler) Trigger(id string) bool {
	s.mu.Lock()
	ch, ok := s.triggers[id]
	s.mu.Unlock()
	if ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return ok
}

func (s *Scheduler) worker(ctx context.Context, camID string, trigger chan struct{}) {
	defer s.wg.Done()
	for {
		cam, ok := s.camera(camID)
		if !ok {
			return
		}
		protect("webcam "+camID, func() { s.tick(ctx, cam) })
		select {
		case <-ctx.Done():
			return
		case <-trigger:
		case <-time.After(time.Duration(cam.IntervalSeconds) * time.Second):
		}
	}
}

func (s *Scheduler) camera(id string) (Camera, bool) {
	for _, c := range s.store.Get().Cameras {
		if c.ID == id {
			return c, true
		}
	}
	return Camera{}, false
}

func (s *Scheduler) status(id string) *CamStatus {
	st := s.camStatus[id]
	if st == nil {
		st = &CamStatus{}
		s.camStatus[id] = st
	}
	return st
}

func (s *Scheduler) tick(ctx context.Context, cam Camera) {
	now := time.Now()
	if !cam.IsActive(now) {
		s.mu.Lock()
		s.status(cam.ID).OutOfHours = true
		s.mu.Unlock()
		return
	}
	raw, err := CaptureFrame(ctx, cam)
	if err == nil {
		var img []byte
		img, err = s.renderer.Process(raw, cam, now, PrimarySite(s.store.Get(), cam.ID))
		if err == nil {
			s.store_(cam, raw, img, now)
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	log.Printf("webcam %s: %v", cam.ID, err)
	s.mu.Lock()
	st := s.status(cam.ID)
	st.OutOfHours = false
	st.LastError = err.Error()
	st.LastErrorAt = now
	if st.FailingSince.IsZero() {
		st.FailingSince = now
	}
	s.mu.Unlock()
}

// store_ salva l'immagine, la archivia se è iniziata una nuova fascia e la
// pubblica sui siti per cui è arrivato il momento.
func (s *Scheduler) store_(cam Camera, raw, img []byte, now time.Time) {
	_ = os.MkdirAll(filepath.Dir(s.latestPath(cam.ID)), 0o755)
	if err := writeAtomic(s.latestPath(cam.ID), img); err != nil {
		log.Printf("webcam %s: salvataggio: %v", cam.ID, err)
	}
	if low, free := lowDisk(s.dataDir); low {
		s.mu.Lock()
		s.diskLow, s.diskFree = true, free
		s.mu.Unlock()
	} else if cam.ArchiveMinutes > 0 {
		s.mu.Lock()
		s.diskLow = false
		s.mu.Unlock()
		slot := SlotFor(now, cam.ArchiveMinutes)
		if !s.archive.Has(cam.ID, slot) {
			if err := s.archive.Save(cam.ID, slot, img); err != nil {
				log.Printf("webcam %s: storico: %v", cam.ID, err)
			}
			if n := s.archive.Prune(cam.ID, cam.RetentionDays, int64(cam.RetentionMB)<<20, now); n > 0 {
				log.Printf("webcam %s: eliminate %d immagini vecchie dallo storico locale", cam.ID, n)
			}
		}
	}
	tlAdded := false
	if cam.TimelapseMinutes > 0 {
		var err error
		if tlAdded, err = s.tl.Add(cam.ID, now, img, cam.TimelapseMinutes, cam.TimelapseHours); err != nil {
			log.Printf("webcam %s: timelapse: %v", cam.ID, err)
		}
	}
	s.mu.Lock()
	s.latest[cam.ID] = img
	s.raw[cam.ID] = raw
	st := s.status(cam.ID)
	st.LastCapture = now
	st.LastError = ""
	st.FailingSince = time.Time{}
	st.OutOfHours = false
	st.Captures++
	st.ArchiveCount = s.archive.Count(cam.ID)
	st.ArchiveBytes = s.archive.Size(cam.ID)
	s.mu.Unlock()

	for _, site := range s.store.Get().Sites {
		if !site.Enabled {
			continue
		}
		var due []Publication
		onSite := false
		for _, p := range site.Publications {
			if p.CameraID != cam.ID {
				continue
			}
			onSite = true
			if s.isDue(site.ID, p, cam, now) {
				due = append(due, p)
			}
		}
		history := site.History && onSite && (cam.ArchiveMinutes > 0 &&
			len(s.archive.KeysAfter(cam.ID, s.sync.Get(site.ID, cam.ID), 1)) > 0 || tlAdded || s.tlPending(site.ID, cam.ID))
		if len(due) == 0 && !history {
			continue
		}
		siteImg := img
		if len(due) > 0 && UsesSitePlaceholders(cam) {
			if primary := PrimarySite(s.store.Get(), cam.ID); primary == nil || primary.ID != site.ID {
				if alt, err := s.renderer.Process(raw, cam, now, &site); err == nil {
					siteImg = alt
				}
			}
		}
		go protect("pubblicazione su "+site.ID, func() { s.publish(site, cam, siteImg, due, history) })
	}
}

// PrimarySite è il primo sito attivo su cui è pubblicata la webcam: fornisce
// l'indirizzo per le scritte dell'immagine locale e dello storico.
func PrimarySite(cfg Config, camID string) *Site {
	for i, site := range cfg.Sites {
		if !site.Enabled {
			continue
		}
		for _, p := range site.Publications {
			if p.CameraID == camID {
				return &cfg.Sites[i]
			}
		}
	}
	return nil
}

// isDue applica la periodicità della pubblicazione. Una tolleranza di metà
// intervallo di cattura evita di saltare un giro per pochi secondi.
func (s *Scheduler) isDue(siteID string, p Publication, cam Camera, now time.Time) bool {
	if p.IntervalSeconds <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	last := s.lastPub[siteID+"|"+p.CameraID+"|"+p.Filename]
	if last.IsZero() {
		return true
	}
	tolerance := time.Duration(cam.IntervalSeconds) * time.Second / 2
	return now.Sub(last)+tolerance >= time.Duration(p.IntervalSeconds)*time.Second
}

func (s *Scheduler) siteLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.siteLocks[id]
	if l == nil {
		l = &sync.Mutex{}
		s.siteLocks[id] = l
	}
	return l
}

func (s *Scheduler) publish(site Site, cam Camera, img []byte, due []Publication, history bool) {
	// Se un invio di questa webcam verso questo sito è ancora in coda o in
	// corso, questo giro si salta: l'immagine arriverà al prossimo e lo
	// storico recupera da sé. Così una rete lenta non accumula arretrato.
	busyKey := site.ID + "|" + cam.ID
	s.mu.Lock()
	if s.busy[busyKey] {
		s.mu.Unlock()
		return
	}
	s.busy[busyKey] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.busy, busyKey)
		s.mu.Unlock()
	}()
	// Una connessione alla volta per sito: gli hosting limitano le connessioni FTP.
	lock := s.siteLock(site.ID)
	lock.Lock()
	defer lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	now := time.Now()
	conn, err := OpenSite(ctx, s.store, site)
	if err == nil {
		// se il server smette di rispondere, chiudere la connessione sblocca l'upload
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-done:
			}
		}()
		defer conn.Close()
	}
	for _, p := range due {
		perr := err
		if perr == nil {
			perr = conn.Put(p.Filename, img)
		}
		s.setPub(site.ID, p, now, perr)
	}
	if history {
		herr := err
		if herr == nil {
			herr = s.syncHistory(conn, site, cam)
		}
		s.mu.Lock()
		ss := s.siteStatus[site.ID]
		if ss == nil {
			ss = &SiteStatus{}
			s.siteStatus[site.ID] = ss
		}
		if herr != nil {
			ss.HistoryError = herr.Error()
			if ss.FailingSince.IsZero() {
				ss.FailingSince = now
			}
		} else {
			ss.HistoryError = ""
			ss.LastHistorySync = time.Now()
			ss.FailingSince = time.Time{}
		}
		ss.HistoryPending = len(s.archive.KeysAfter(cam.ID, s.sync.Get(site.ID, cam.ID), 100000))
		s.mu.Unlock()
	}
}

func (s *Scheduler) setPub(siteID string, p Publication, now time.Time, err error) {
	key := siteID + "|" + p.CameraID + "|" + p.Filename
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.pubStatus[key]
	if ps == nil {
		ps = &PubStatus{SiteID: siteID, CameraID: p.CameraID, Filename: p.Filename}
		s.pubStatus[key] = ps
	}
	ps.LastTry = now
	if err != nil {
		ps.LastError = err.Error()
		if ps.FailingSince.IsZero() {
			ps.FailingSince = now
		}
		log.Printf("sito %s, %s: %v", siteID, p.Filename, err)
		return
	}
	ps.LastError = ""
	ps.FailingSince = time.Time{}
	ps.LastUpload = now
	s.lastPub[key] = now
}

// HistoryCamera è una voce di cameras.json, letto dal visualizzatore storico.
type HistoryCamera struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Current        string `json:"current"` // URL dell'immagine corrente
	ArchiveMinutes int    `json:"archive_minutes"`
	Timelapse      bool   `json:"timelapse"`
}

type historyIndex struct {
	Title   string          `json:"title"`
	Cameras []HistoryCamera `json:"cameras"`
}

// SiteHistoryIndex elenca le webcam con storico pubblicate su un sito.
func SiteHistoryIndex(cfg Config, site Site) []byte {
	title := site.HistoryTitle
	if title == "" {
		title = cfg.Location.Name
	}
	idx := historyIndex{Title: title, Cameras: []HistoryCamera{}}
	seen := map[string]bool{}
	for _, p := range site.Publications {
		if seen[p.CameraID] {
			continue
		}
		for _, c := range cfg.Cameras {
			if c.ID == p.CameraID && (c.ArchiveMinutes > 0 || c.TimelapseMinutes > 0) {
				seen[c.ID] = true
				idx.Cameras = append(idx.Cameras, HistoryCamera{ID: c.ID, Name: c.Name, Current: "../" + p.Filename, ArchiveMinutes: c.ArchiveMinutes, Timelapse: c.TimelapseMinutes > 0})
			}
		}
	}
	data, _ := json.Marshal(idx)
	return data
}

// syncHistory carica sul sito il mini-sito e le immagini dello storico non
// ancora inviate per questa webcam.
func (s *Scheduler) syncHistory(conn SiteConn, site Site, cam Camera) error {
	dir := site.HistoryDir
	s.mu.Lock()
	assets := !s.assetsDone[site.ID]
	s.mu.Unlock()
	if assets {
		if err := conn.Put(dir+"/"+site.HistoryIndex, historyHTML); err != nil {
			return err
		}
		if err := conn.Put(dir+"/cameras.json", SiteHistoryIndex(s.store.Get(), site)); err != nil {
			return err
		}
		s.mu.Lock()
		s.assetsDone[site.ID] = true
		s.mu.Unlock()
	}
	keys := s.archive.KeysAfter(cam.ID, s.sync.Get(site.ID, cam.ID), historyBatch)
	var firstErr error
	uploaded := 0
	for _, k := range keys {
		ok := true
		for _, thumb := range []bool{true, false} {
			p, _ := s.archive.File(cam.ID, k, thumb)
			data, err := os.ReadFile(p)
			if err != nil {
				continue // file rimosso a mano: lo saltiamo
			}
			name := k + ".jpg"
			if thumb {
				name = k + "_t.jpg"
			}
			if err := conn.Put(dir+"/"+cam.ID+"/"+name, data); err != nil {
				firstErr, ok = err, false
				break
			}
		}
		if !ok {
			break
		}
		s.sync.Set(site.ID, cam.ID, k)
		uploaded++
	}
	if uploaded > 0 || assets {
		if err := conn.Put(dir+"/"+cam.ID+"/days.json", s.archive.DaysJSON(cam.ID)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil && cam.TimelapseMinutes > 0 {
		firstErr = s.syncTimelapse(conn, site, cam)
	}
	return firstErr
}

func (s *Scheduler) tlPending(siteID, camID string) bool {
	s.mu.Lock()
	last := s.tlSent[siteID+"|"+camID]
	s.mu.Unlock()
	return len(s.tl.FramesAfter(camID, last)) > 0
}

// syncTimelapse carica i nuovi fotogrammi del timelapse e il relativo indice.
func (s *Scheduler) syncTimelapse(conn SiteConn, site Site, cam Camera) error {
	key := site.ID + "|" + cam.ID
	s.mu.Lock()
	last := s.tlSent[key]
	s.mu.Unlock()
	frames := s.tl.FramesAfter(cam.ID, last)
	if len(frames) == 0 && last != 0 {
		return nil
	}
	base := site.HistoryDir + "/" + cam.ID + "/timelapse/"
	for _, f := range frames {
		p, ok := s.tl.File(cam.ID, f.F)
		if !ok {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := conn.Put(base+f.F, data); err != nil {
			return err
		}
		s.mu.Lock()
		s.tlSent[key] = f.T
		s.mu.Unlock()
	}
	return conn.Put(base+"timelapse.json", s.tl.JSON(cam.ID))
}

// Latest restituisce l'ultima immagine elaborata della webcam.
func (s *Scheduler) Latest(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest[id]
}

// Raw restituisce l'ultima immagine grezza (senza sovrimpressioni).
func (s *Scheduler) Raw(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw[id]
}

// Status restituisce una copia dello stato per il pannello.
func (s *Scheduler) Status() (map[string]CamStatus, []PubStatus, map[string]SiteStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cams := map[string]CamStatus{}
	for k, v := range s.camStatus {
		cams[k] = *v
	}
	pubs := []PubStatus{}
	for _, v := range s.pubStatus {
		pubs = append(pubs, *v)
	}
	sites := map[string]SiteStatus{}
	for k, v := range s.siteStatus {
		sites[k] = *v
	}
	return cams, pubs, sites
}
