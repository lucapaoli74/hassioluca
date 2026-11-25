package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Su Windows ffmpeg (necessario per le telecamere RTSP) viene scaricato
// automaticamente la prima volta che serve, così non va copiato a mano su ogni PC.

const ffmpegZipURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-lgpl.zip"

// FFmpegStatus è lo stato del download mostrato nel pannello.
type FFmpegStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

var ffmpegInst struct {
	sync.Mutex
	running  bool
	err      string
	lastTry  time.Time
	extraDir string // cartella dati dell'applicazione (impostata all'avvio)
}

// ffmpegTargetDirs: dove salvare ffmpeg.exe, in ordine di preferenza.
func ffmpegTargetDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	ffmpegInst.Lock()
	if ffmpegInst.extraDir != "" {
		dirs = append(dirs, ffmpegInst.extraDir)
	}
	ffmpegInst.Unlock()
	return dirs
}

// GetFFmpegStatus riporta se ffmpeg è disponibile e lo stato del download.
func GetFFmpegStatus() FFmpegStatus {
	p, err := ffmpegPath()
	ffmpegInst.Lock()
	defer ffmpegInst.Unlock()
	return FFmpegStatus{Found: err == nil, Path: p, Running: ffmpegInst.running, Error: ffmpegInst.err}
}

// autoInstallFFmpeg avvia in background il download (al massimo un tentativo
// all'ora in caso di errore). Restituisce true se un download è in corso.
func autoInstallFFmpeg() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	ffmpegInst.Lock()
	defer ffmpegInst.Unlock()
	if ffmpegInst.running {
		return true
	}
	if !ffmpegInst.lastTry.IsZero() && time.Since(ffmpegInst.lastTry) < time.Hour && ffmpegInst.err != "" {
		return false
	}
	startFFmpegInstallLocked()
	return true
}

// StartFFmpegInstall avvia il download su richiesta dal pannello.
func StartFFmpegInstall() error {
	if runtime.GOOS != "windows" {
		return errors.New("su Linux installa ffmpeg con: sudo apt install ffmpeg")
	}
	ffmpegInst.Lock()
	defer ffmpegInst.Unlock()
	if !ffmpegInst.running {
		startFFmpegInstallLocked()
	}
	return nil
}

func startFFmpegInstallLocked() {
	ffmpegInst.running, ffmpegInst.err, ffmpegInst.lastTry = true, "", time.Now()
	go func() {
		err := downloadFFmpeg()
		ffmpegInst.Lock()
		ffmpegInst.running = false
		if err != nil {
			ffmpegInst.err = err.Error()
			log.Printf("download di ffmpeg non riuscito: %v", err)
		} else {
			log.Printf("ffmpeg installato")
		}
		ffmpegInst.Unlock()
	}()
}

func downloadFFmpeg() error {
	log.Printf("ffmpeg mancante: download in corso da %s", ffmpegZipURL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ffmpegZipURL, nil)
	req.Header.Set("User-Agent", "webcam-manager/"+version)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download ffmpeg: %s", resp.Status)
	}
	tmp, err := os.CreateTemp("", "ffmpeg-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, 500<<20))
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(tmp, n)
	if err != nil {
		return fmt.Errorf("archivio ffmpeg non valido: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, "/bin/ffmpeg.exe") {
			continue
		}
		var lastErr error
		for _, dir := range ffmpegTargetDirs() {
			if lastErr = extractTo(f, filepath.Join(dir, "ffmpeg.exe")); lastErr == nil {
				return nil
			}
		}
		return fmt.Errorf("impossibile salvare ffmpeg.exe: %w", lastErr)
	}
	return errors.New("ffmpeg.exe non trovato nell'archivio")
}

func extractTo(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
