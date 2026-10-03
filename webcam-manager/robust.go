package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// protect esegue fn intercettando eventuali panic: un errore imprevisto in una
// webcam o in un upload viene registrato senza fermare tutto il programma.
func protect(what string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERRORE INTERNO in %s: %v\n%s", what, r, debug.Stack())
		}
	}()
	fn()
}

// minFreeBytes: sotto questa soglia di spazio libero lo storico locale smette
// di crescere (la pubblicazione continua) e viene inviato un avviso.
const minFreeBytes = 1 << 30

// diskFree restituisce i byte liberi sul disco che contiene path.
func diskFree(path string) (uint64, error) { return freeBytes(path) }

// lowDisk dice se lo spazio libero è sotto la soglia.
func lowDisk(dir string) (bool, uint64) {
	free, err := diskFree(dir)
	if err != nil {
		return false, 0
	}
	return free < minFreeBytes, free
}

// rotatingFile è un file di log che, superata la dimensione massima, viene
// rinominato in .1 e ricominciato: il log non cresce mai oltre ~2×max.
type rotatingFile struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

func openRotating(path string, max int64) (*rotatingFile, error) {
	r := &rotatingFile{path: path, max: max}
	return r, r.open()
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	r.f, r.size = f, st.Size()
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > r.max {
		r.f.Close()
		_ = os.Rename(r.path, r.path+".1")
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	default:
		return fmt.Sprintf("%d MB", n>>20)
	}
}

var startTime = time.Now()
