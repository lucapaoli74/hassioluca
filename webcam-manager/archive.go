package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Archive conserva lo storico delle immagini su disco, per sempre:
//
//	<dir>/<webcam>/AAAA/MM/GG/HHMM.jpg     immagine
//	<dir>/<webcam>/AAAA/MM/GG/HHMM_t.jpg   miniatura
//
// La stessa struttura viene replicata sui siti dentro la cartella del
// mini-sito, così il visualizzatore funziona identico in locale e online.
type Archive struct {
	dir string
	mu  sync.RWMutex
	// webcam -> "AAAA-MM-GG" -> orari "HHMM" ordinati
	index map[string]map[string][]string
	sizes map[string]map[string]int64 // webcam -> chiave -> byte (immagine + miniatura)
	total map[string]int64            // webcam -> byte occupati
}

const thumbWidth = 320

var archiveFile = regexp.MustCompile(`^(\d{4})/(\d{2})/(\d{2})/(\d{4})\.jpg$`)

func NewArchive(dir string) (*Archive, error) {
	a := &Archive{dir: dir, index: map[string]map[string][]string{}, sizes: map[string]map[string]int64{}, total: map[string]int64{}}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cams, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, c := range cams {
		if !c.IsDir() || !idPattern.MatchString(c.Name()) {
			continue
		}
		root := filepath.Join(dir, c.Name())
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			var size int64
			if info, err := d.Info(); err == nil {
				size = info.Size()
			}
			if m := archiveFile.FindStringSubmatch(rel); m != nil {
				a.add(c.Name(), m[1]+"-"+m[2]+"-"+m[3], m[4])
				a.addSize(c.Name(), strings.TrimSuffix(rel, ".jpg"), size)
			} else if m := archiveFile.FindStringSubmatch(strings.Replace(rel, "_t.jpg", ".jpg", 1)); m != nil {
				a.addSize(c.Name(), strings.TrimSuffix(rel, "_t.jpg"), size)
			}
			return nil
		})
	}
	return a, nil
}

func (a *Archive) addSize(cam, key string, n int64) {
	if a.sizes[cam] == nil {
		a.sizes[cam] = map[string]int64{}
	}
	a.sizes[cam][key] += n
	a.total[cam] += n
}

func (a *Archive) add(cam, day, hhmm string) {
	days := a.index[cam]
	if days == nil {
		days = map[string][]string{}
		a.index[cam] = days
	}
	slots := days[day]
	i := sort.SearchStrings(slots, hhmm)
	if i < len(slots) && slots[i] == hhmm {
		return
	}
	slots = append(slots, "")
	copy(slots[i+1:], slots[i:])
	slots[i] = hhmm
	days[day] = slots
}

// SlotFor arrotonda l'orario all'inizio della fascia di archiviazione.
func SlotFor(t time.Time, minutes int) time.Time {
	m := t.Hour()*60 + t.Minute()
	m -= m % minutes
	return time.Date(t.Year(), t.Month(), t.Day(), m/60, m%60, 0, 0, t.Location())
}

// Key è il percorso relativo di uno slot, es. "2026/10/03/1400".
func Key(slot time.Time) string { return slot.Format("2006/01/02/1504") }

// Has dice se lo slot è già archiviato.
func (a *Archive) Has(cam string, slot time.Time) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	slots := a.index[cam][slot.Format("2006-01-02")]
	hhmm := slot.Format("1504")
	i := sort.SearchStrings(slots, hhmm)
	return i < len(slots) && slots[i] == hhmm
}

// Save archivia l'immagine (e la sua miniatura) nello slot indicato.
func (a *Archive) Save(cam string, slot time.Time, data []byte) error {
	key := Key(slot)
	full := filepath.Join(a.dir, cam, filepath.FromSlash(key)+".jpg")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	thumb, err := makeThumb(data)
	if err != nil {
		return err
	}
	if err := writeAtomic(full, data); err != nil {
		return err
	}
	if err := writeAtomic(strings.TrimSuffix(full, ".jpg")+"_t.jpg", thumb); err != nil {
		return err
	}
	a.mu.Lock()
	if old := a.sizes[cam][key]; old > 0 { // slot riscritto
		a.total[cam] -= old
		delete(a.sizes[cam], key)
	}
	a.add(cam, slot.Format("2006-01-02"), slot.Format("1504"))
	a.addSize(cam, key, int64(len(data)+len(thumb)))
	a.mu.Unlock()
	return nil
}

// Size restituisce lo spazio occupato dallo storico di una webcam.
func (a *Archive) Size(cam string) int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.total[cam]
}

// Prune applica la politica di conservazione: elimina le immagini più
// vecchie di days giorni e/o, finché lo storico supera maxBytes, le più
// vecchie in assoluto. 0 = nessun limite. Restituisce quante ne ha eliminate.
func (a *Archive) Prune(cam string, days int, maxBytes int64, now time.Time) int {
	if days <= 0 && maxBytes <= 0 {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := ""
	if days > 0 {
		cutoff = now.AddDate(0, 0, -days).Format("2006-01-02")
	}
	dayList := make([]string, 0, len(a.index[cam]))
	for d := range a.index[cam] {
		dayList = append(dayList, d)
	}
	sort.Strings(dayList)
	removed := 0
	for _, d := range dayList {
		if !(cutoff != "" && d < cutoff) && !(maxBytes > 0 && a.total[cam] > maxBytes) {
			break
		}
		slots := a.index[cam][d]
		for len(slots) > 0 {
			if !(cutoff != "" && d < cutoff) && !(maxBytes > 0 && a.total[cam] > maxBytes) {
				break
			}
			key := strings.ReplaceAll(d, "-", "/") + "/" + slots[0]
			base := filepath.Join(a.dir, cam, filepath.FromSlash(key))
			_ = os.Remove(base + ".jpg")
			_ = os.Remove(base + "_t.jpg")
			a.total[cam] -= a.sizes[cam][key]
			delete(a.sizes[cam], key)
			slots = slots[1:]
			removed++
		}
		if len(slots) == 0 {
			delete(a.index[cam], d)
			// rimuove le cartelle rimaste vuote (giorno, mese, anno)
			dir := filepath.Join(a.dir, cam, filepath.FromSlash(strings.ReplaceAll(d, "-", "/")))
			for i := 0; i < 3; i++ {
				if os.Remove(dir) != nil {
					break
				}
				dir = filepath.Dir(dir)
			}
		} else {
			a.index[cam][d] = slots
		}
	}
	return removed
}

func makeThumb(data []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	small := resizeToRGBA(img, thumbWidth)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, small, &jpeg.Options{Quality: 70}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeAtomic(p string, data []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// File restituisce il percorso locale di un'immagine dello storico.
func (a *Archive) File(cam, key string, thumb bool) (string, error) {
	if !idPattern.MatchString(cam) || !archiveFile.MatchString(key+".jpg") {
		return "", fmt.Errorf("percorso non valido")
	}
	suffix := ".jpg"
	if thumb {
		suffix = "_t.jpg"
	}
	return filepath.Join(a.dir, cam, filepath.FromSlash(key)+suffix), nil
}

// DaysJSON produce l'indice usato dal visualizzatore: {"AAAA-MM-GG":["HHMM",...]}.
func (a *Archive) DaysJSON(cam string) []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()
	days := a.index[cam]
	if days == nil {
		days = map[string][]string{}
	}
	data, _ := json.Marshal(days)
	return data
}

// KeysAfter restituisce, in ordine cronologico, le chiavi successive ad
// after ("" = dall'inizio), al massimo limit.
func (a *Archive) KeysAfter(cam, after string, limit int) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	days := a.index[cam]
	dayList := make([]string, 0, len(days))
	for d := range days {
		dayList = append(dayList, d)
	}
	sort.Strings(dayList)
	var out []string
	for _, d := range dayList {
		prefix := strings.ReplaceAll(d, "-", "/") + "/"
		if after != "" && prefix+"9999" <= after {
			continue
		}
		for _, hhmm := range days[d] {
			k := prefix + hhmm
			if k > after {
				out = append(out, k)
				if len(out) >= limit {
					return out
				}
			}
		}
	}
	return out
}

// Count restituisce il numero di immagini archiviate per una webcam.
func (a *Archive) Count(cam string) int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	n := 0
	for _, s := range a.index[cam] {
		n += len(s)
	}
	return n
}

// SyncState ricorda, per ogni coppia sito/webcam, l'ultima immagine dello
// storico caricata. Lo storico cresce solo in avanti, quindi basta un
// "segnalibro": dopo un errore di rete si riparte da lì.
type SyncState struct {
	mu    sync.Mutex
	path  string
	marks map[string]string
}

func LoadSyncState(path string) *SyncState {
	s := &SyncState{path: path, marks: map[string]string{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s.marks)
	}
	return s
}

func (s *SyncState) Get(site, cam string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.marks[site+"|"+cam]
}

func (s *SyncState) Set(site, cam, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks[site+"|"+cam] = key
	data, _ := json.MarshalIndent(s.marks, "", "  ")
	_ = writeAtomic(s.path, data)
}

// ResetSite fa ripartire da zero il caricamento dello storico su un sito.
func (s *SyncState) ResetSite(site string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.marks {
		if strings.HasPrefix(k, site+"|") {
			delete(s.marks, k)
		}
	}
	data, _ := json.MarshalIndent(s.marks, "", "  ")
	_ = writeAtomic(s.path, data)
}
