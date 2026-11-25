package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Timelapse conserva, per ogni webcam, i fotogrammi delle ultime ore (es. uno
// ogni 5 minuti per 6 ore) usati dal player timelapse del mini-sito.
// I file hanno nomi a rotazione (tl_000.jpg … tl_071.jpg): il fotogramma nuovo
// sovrascrive il più vecchio, così né il disco né il sito crescono nel tempo.
type Timelapse struct {
	dir    string
	mu     sync.Mutex
	frames map[string]*tlIndex
}

type tlFrame struct {
	T int64  `json:"t"` // unix time
	F string `json:"f"` // nome file
}

type tlIndex struct {
	Step   int       `json:"step"`  // minuti fra un fotogramma e l'altro
	Hours  int       `json:"hours"` //
	Frames []tlFrame `json:"frames"`
}

const timelapseWidth = 960

func NewTimelapse(dir string) *Timelapse {
	t := &Timelapse{dir: dir, frames: map[string]*tlIndex{}}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "timelapse.json"))
		if err != nil {
			continue
		}
		var idx tlIndex
		if json.Unmarshal(data, &idx) == nil {
			t.frames[e.Name()] = &idx
		}
	}
	return t
}

// Add aggiunge il fotogramma se è passato il tempo previsto dall'ultimo.
// Restituisce true se è stato aggiunto.
func (t *Timelapse) Add(cam string, now time.Time, img []byte, stepMin, hours int) (bool, error) {
	if stepMin <= 0 {
		return false, nil
	}
	if hours <= 0 {
		hours = 6
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := t.frames[cam]
	if idx == nil || idx.Step != stepMin || idx.Hours != hours {
		idx = &tlIndex{Step: stepMin, Hours: hours} // impostazioni cambiate: si riparte
		t.frames[cam] = idx
	}
	step := int64(stepMin * 60)
	ts := now.Unix()
	if n := len(idx.Frames); n > 0 && ts/step == idx.Frames[n-1].T/step {
		return false, nil
	}
	slots := int64(hours * 60 / stepMin)
	name := fmt.Sprintf("tl_%03d.jpg", (ts/step)%slots)

	decoded, err := jpeg.Decode(bytes.NewReader(img))
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resizeToRGBA(decoded, timelapseWidth), &jpeg.Options{Quality: 75}); err != nil {
		return false, err
	}
	dir := filepath.Join(t.dir, cam)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := writeAtomic(filepath.Join(dir, name), buf.Bytes()); err != nil {
		return false, err
	}
	cutoff := ts - int64(hours*3600)
	kept := idx.Frames[:0]
	for _, f := range idx.Frames {
		if f.T > cutoff && f.F != name {
			kept = append(kept, f)
		}
	}
	idx.Frames = append(kept, tlFrame{T: ts, F: name})
	data, _ := json.Marshal(idx)
	_ = writeAtomic(filepath.Join(dir, "timelapse.json"), data)
	return true, nil
}

// JSON restituisce l'indice dei fotogrammi per il player.
func (t *Timelapse) JSON(cam string) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := t.frames[cam]
	if idx == nil {
		idx = &tlIndex{Frames: []tlFrame{}}
	}
	data, _ := json.Marshal(idx)
	return data
}

// FramesAfter restituisce i fotogrammi più recenti di ts.
func (t *Timelapse) FramesAfter(cam string, ts int64) []tlFrame {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := t.frames[cam]
	if idx == nil {
		return nil
	}
	var out []tlFrame
	for _, f := range idx.Frames {
		if f.T > ts {
			out = append(out, f)
		}
	}
	return out
}

// File restituisce il percorso locale di un fotogramma.
func (t *Timelapse) File(cam, name string) (string, bool) {
	if !idPattern.MatchString(cam) || !tlName(name) {
		return "", false
	}
	return filepath.Join(t.dir, cam, name), true
}

func tlName(n string) bool {
	var i int
	_, err := fmt.Sscanf(n, "tl_%03d.jpg", &i)
	return err == nil && len(n) == len("tl_000.jpg")
}
