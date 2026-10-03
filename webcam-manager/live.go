package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// LiveHub condivide un unico flusso dalla telecamera fra tutti i browser che
// la stanno guardando: la telecamera (o ffmpeg, per le RTSP) lavora una volta
// sola e il flusso si ferma quando l'ultimo spettatore chiude la pagina.
type LiveHub struct {
	mu    sync.Mutex
	feeds map[string]*liveFeed

	// sessioni temporanee per vedere telecamere trovate dalla ricerca senza
	// doverle prima aggiungere alla configurazione
	sessions map[string]liveSession
}

type liveSession struct {
	cam     Camera
	expires time.Time
}

type liveFeed struct {
	subs   map[chan []byte]struct{}
	cancel context.CancelFunc
	err    string
}

func NewLiveHub() *LiveHub {
	return &LiveHub{feeds: map[string]*liveFeed{}, sessions: map[string]liveSession{}}
}

// Subscribe restituisce un canale di fotogrammi e la funzione per chiuderlo.
func (h *LiveHub) Subscribe(key string, cam Camera) (<-chan []byte, func()) {
	ch := make(chan []byte, 2)
	h.mu.Lock()
	f := h.feeds[key]
	if f == nil {
		ctx, cancel := context.WithCancel(context.Background())
		f = &liveFeed{subs: map[chan []byte]struct{}{}, cancel: cancel}
		h.feeds[key] = f
		go h.run(ctx, key, cam, f)
	}
	f.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(f.subs, ch)
		if len(f.subs) == 0 && h.feeds[key] == f {
			f.cancel()
			delete(h.feeds, key)
		}
	}
}

func (h *LiveHub) run(ctx context.Context, key string, cam Camera, f *liveFeed) {
	for ctx.Err() == nil {
		var err error
		protect("live "+key, func() {
			err = LiveStream(ctx, cam, func(frame []byte) error {
				h.mu.Lock()
				defer h.mu.Unlock()
				f.err = ""
				for ch := range f.subs {
					select {
					case ch <- frame:
					default: // spettatore lento: salta il fotogramma
					}
				}
				return ctx.Err()
			})
		})
		if ctx.Err() != nil {
			return
		}
		log.Printf("live %s: %v", key, err)
		h.mu.Lock()
		if err != nil {
			f.err = err.Error()
		}
		h.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// Error restituisce l'ultimo errore del flusso (vuoto se va tutto bene).
func (h *LiveHub) Error(key string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if f := h.feeds[key]; f != nil {
		return f.err
	}
	return ""
}

// NewSession registra una telecamera temporanea e ne restituisce il token.
func (h *LiveHub) NewSession(cam Camera) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for k, s := range h.sessions {
		if now.After(s.expires) {
			delete(h.sessions, k)
		}
	}
	h.sessions[token] = liveSession{cam: cam, expires: now.Add(2 * time.Hour)}
	return token
}

func (h *LiveHub) Session(token string) (Camera, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[token]
	if !ok || time.Now().After(s.expires) {
		return Camera{}, false
	}
	return s.cam, true
}
