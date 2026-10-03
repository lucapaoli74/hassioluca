package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxFrameBytes = 20 << 20

var httpClient = &http.Client{Timeout: 20 * time.Second}

// streamClient non ha timeout globale: gli stream live durano finché il
// browser resta collegato e sono chiusi tramite il context.
var streamClient = &http.Client{}

// effectiveSource corregge il tipo di sorgente quando l'indirizzo non lascia
// dubbi (es. un URL rtsp:// impostato per errore come "snapshot").
func effectiveSource(cam Camera) string {
	u := strings.ToLower(strings.TrimSpace(cam.URL))
	if strings.HasPrefix(u, "rtsp://") || strings.HasPrefix(u, "rtsps://") {
		return "rtsp"
	}
	if cam.Source == "rtsp" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
		return "snapshot"
	}
	return cam.Source
}

// CaptureFrame restituisce un singolo JPEG dalla telecamera.
func CaptureFrame(ctx context.Context, cam Camera) ([]byte, error) {
	cam.Source = effectiveSource(cam)
	switch cam.Source {
	case "snapshot":
		return fetchSnapshot(ctx, cam)
	case "mjpeg":
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		resp, err := doCameraRequest(ctx, streamClient, cam.URL, cam.Username, cam.Password)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return nextJPEG(bufio.NewReader(resp.Body))
	case "rtsp":
		return rtspFrame(ctx, cam)
	}
	return nil, fmt.Errorf("sorgente %q sconosciuta", cam.Source)
}

func fetchSnapshot(ctx context.Context, cam Camera) ([]byte, error) {
	resp, err := doCameraRequest(ctx, httpClient, cam.URL, cam.Username, cam.Password)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFrameBytes))
	if err != nil {
		return nil, err
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errors.New("la risposta non è un'immagine JPEG (controlla l'URL)")
	}
	return data, nil
}

// doCameraRequest esegue una GET gestendo autenticazione Basic e Digest
// (molte telecamere Hikvision/Dahua accettano solo Digest).
func doCameraRequest(ctx context.Context, client *http.Client, rawURL, user, pass string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && user != "" {
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if !strings.HasPrefix(strings.ToLower(challenge), "digest ") {
			return nil, errors.New("credenziali rifiutate dalla telecamera")
		}
		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		auth, err := digestAuthorization(challenge, http.MethodGet, req2.URL.RequestURI(), user, pass)
		if err != nil {
			return nil, err
		}
		req2.Header.Set("Authorization", auth)
		resp, err = client.Do(req2)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("credenziali rifiutate dalla telecamera")
		}
		return nil, fmt.Errorf("la telecamera ha risposto %s", resp.Status)
	}
	return resp, nil
}

// parseAuthParams legge i parametri key="value" di un header WWW-Authenticate.
func parseAuthParams(s string) map[string]string {
	out := map[string]string{}
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ,")
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(s[:eq]))
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				val, s = s[1:], ""
			} else {
				val, s = s[1:end+1], s[end+2:]
			}
		} else {
			end := strings.IndexByte(s, ',')
			if end < 0 {
				val, s = s, ""
			} else {
				val, s = s[:end], s[end:]
			}
		}
		out[key] = strings.TrimSpace(val)
	}
	return out
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func digestAuthorization(challenge, method, uri, user, pass string) (string, error) {
	p := parseAuthParams(challenge[len("digest "):])
	if alg := strings.ToUpper(p["algorithm"]); alg != "" && alg != "MD5" {
		return "", fmt.Errorf("algoritmo digest %s non supportato", alg)
	}
	ha1 := md5hex(user + ":" + p["realm"] + ":" + pass)
	ha2 := md5hex(method + ":" + uri)
	var b strings.Builder
	fmt.Fprintf(&b, `Digest username="%s", realm="%s", nonce="%s", uri="%s"`, user, p["realm"], p["nonce"], uri)
	if strings.Contains(p["qop"], "auth") {
		cb := make([]byte, 8)
		_, _ = rand.Read(cb)
		cnonce := hex.EncodeToString(cb)
		nc := "00000001"
		resp := md5hex(ha1 + ":" + p["nonce"] + ":" + nc + ":" + cnonce + ":auth:" + ha2)
		fmt.Fprintf(&b, `, qop=auth, nc=%s, cnonce="%s", response="%s"`, nc, cnonce, resp)
	} else {
		fmt.Fprintf(&b, `, response="%s"`, md5hex(ha1+":"+p["nonce"]+":"+ha2))
	}
	if p["opaque"] != "" {
		fmt.Fprintf(&b, `, opaque="%s"`, p["opaque"])
	}
	if p["algorithm"] != "" {
		fmt.Fprintf(&b, `, algorithm=%s`, p["algorithm"])
	}
	return b.String(), nil
}

// nextJPEG estrae il prossimo fotogramma JPEG completo da uno stream MJPEG
// cercando i marcatori SOI (FFD8) ed EOI (FFD9).
func nextJPEG(r *bufio.Reader) ([]byte, error) {
	var prev byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if prev == 0xFF && b == 0xD8 {
			break
		}
		prev = b
	}
	buf := bytes.NewBuffer([]byte{0xFF, 0xD8})
	prev = 0
	for buf.Len() < maxFrameBytes {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(b)
		if prev == 0xFF && b == 0xD9 {
			return buf.Bytes(), nil
		}
		prev = b
	}
	return nil, errors.New("fotogramma MJPEG troppo grande")
}

// ffmpegPath cerca ffmpeg accanto all'eseguibile, poi nel PATH.
func ffmpegPath() (string, error) {
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	if exe, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(local); err == nil {
			allowFFmpegFirewall(local)
			return local, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		allowFFmpegFirewall(p)
		return p, nil
	}
	// posizioni comuni (il servizio di Windows non vede il PATH dell'utente)
	var extra []string
	if runtime.GOOS == "windows" {
		extra = []string{`C:\ffmpeg\bin\ffmpeg.exe`, `C:\ffmpeg\ffmpeg.exe`,
			filepath.Join(os.Getenv("ProgramFiles"), "ffmpeg", "bin", "ffmpeg.exe"),
			filepath.Join(systemDataDir(), "ffmpeg.exe")}
	} else {
		extra = []string{"/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg", "/opt/homebrew/bin/ffmpeg"}
	}
	ffmpegInst.Lock()
	if ffmpegInst.extraDir != "" {
		extra = append(extra, filepath.Join(ffmpegInst.extraDir, name))
	}
	ffmpegInst.Unlock()
	for _, p := range extra {
		if _, err := os.Stat(p); err == nil {
			allowFFmpegFirewall(p)
			return p, nil
		}
	}
	if autoInstallFFmpeg() {
		return "", errors.New("ffmpeg (necessario per le telecamere RTSP) non c'è ancora: lo sto scaricando in automatico, riprova tra qualche minuto")
	}
	return "", errors.New("per le telecamere RTSP serve ffmpeg: copia ffmpeg.exe nella cartella del programma (accanto a webcam-manager.exe)")
}

var credsInURL = regexp.MustCompile(`(rtsps?://)[^/@\s]+@`)

// hideCreds toglie utente e password dagli URL contenuti nei messaggi.
func hideCreds(s string) string { return credsInURL.ReplaceAllString(s, "${1}***@") }

// rtspTransports ricorda, per ogni URL, il trasporto che ha funzionato.
var (
	rtspMu         sync.Mutex
	rtspTransports = map[string]string{}
)

// transportsFor restituisce l'ordine in cui provare TCP e UDP: prima quello
// che ha già funzionato. Alcune telecamere (es. EZVIZ più vecchie) vanno solo in UDP.
func transportsFor(u string) []string {
	rtspMu.Lock()
	defer rtspMu.Unlock()
	if rtspTransports[u] == "udp" {
		return []string{"udp", "tcp"}
	}
	return []string{"tcp", "udp"}
}

func rememberTransport(u, t string) {
	rtspMu.Lock()
	rtspTransports[u] = t
	rtspMu.Unlock()
}

// ffmpegError riassume l'errore di ffmpeg in modo comprensibile.
func ffmpegError(stderr string, err error) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			msg = "nessuna risposta entro il tempo limite"
		}
	}
	if lines := strings.Split(msg, "\n"); len(lines) > 3 {
		msg = strings.Join(lines[len(lines)-3:], " / ")
	}
	low := strings.ToLower(msg)
	hint := ""
	switch {
	case strings.Contains(low, "401") || strings.Contains(low, "unauthorized"):
		hint = " — credenziali rifiutate (per EZVIZ: utente admin, password = codice di verifica)"
	case strings.Contains(low, "404") || strings.Contains(low, "not found"):
		hint = " — percorso dello stream sbagliato"
	case strings.Contains(low, "connection refused"):
		hint = " — la porta RTSP è chiusa (attiva RTSP nell'app della telecamera)"
	case strings.Contains(low, "timed out") || strings.Contains(low, "timeout") || errors.Is(err, context.DeadlineExceeded):
		hint = " — la telecamera non invia video (è già collegata a un altro programma, es. iSpy? molte EZVIZ accettano un solo collegamento)"
	}
	if len(msg) > 300 {
		msg = msg[len(msg)-300:]
	}
	return fmt.Errorf("ffmpeg: %s%s", hideCreds(msg), hint)
}

// rtspURL inserisce le credenziali nell'URL RTSP se non sono già presenti.
func rtspURL(cam Camera) string {
	u, err := url.Parse(cam.URL)
	if err != nil || cam.Username == "" || u.User != nil {
		return cam.URL
	}
	u.User = url.UserPassword(cam.Username, cam.Password)
	return u.String()
}

func rtspFrame(ctx context.Context, cam Camera) ([]byte, error) {
	bin, err := ffmpegPath()
	if err != nil {
		return nil, err
	}
	u := rtspURL(cam)
	var errs []string
	for _, transport := range transportsFor(u) {
		start := time.Now()
		out, err := rtspFrameWith(ctx, bin, u, transport)
		if err == nil {
			rememberTransport(u, transport)
			return out, nil
		}
		// l'errore riporta entrambi i tentativi, per capire dove si blocca
		errs = append(errs, fmt.Sprintf("%s (%.0f s): %s", strings.ToUpper(transport), time.Since(start).Seconds(),
			strings.TrimPrefix(err.Error(), "ffmpeg: ")))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("ffmpeg non riceve immagini · %s", strings.Join(errs, " · "))
}

func rtspFrameWith(ctx context.Context, bin, u, transport string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "warning",
		"-rtsp_transport", transport, "-i", u,
		"-frames:v", "1", "-f", "image2", "-c:v", "mjpeg", "-q:v", "2", "pipe:1")
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			err = context.DeadlineExceeded
		}
		return nil, ffmpegError(stderr.String(), err)
	}
	if len(out) < 2 || out[0] != 0xFF || out[1] != 0xD8 {
		return nil, errors.New("ffmpeg non ha prodotto un'immagine (lo stream è video?)")
	}
	return out, nil
}

// LiveStream invia fotogrammi JPEG a emit finché ctx non viene annullato o
// emit restituisce un errore (browser disconnesso).
func LiveStream(ctx context.Context, cam Camera, emit func([]byte) error) error {
	cam.Source = effectiveSource(cam)
	switch cam.Source {
	case "mjpeg":
		resp, err := doCameraRequest(ctx, streamClient, cam.URL, cam.Username, cam.Password)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		r := bufio.NewReaderSize(resp.Body, 64<<10)
		for {
			frame, err := nextJPEG(r)
			if err != nil {
				return err
			}
			if err := emit(frame); err != nil {
				return err
			}
		}
	case "rtsp":
		bin, err := ffmpegPath()
		if err != nil {
			return err
		}
		u := rtspURL(cam)
		var lastErr error
		for _, transport := range transportsFor(u) {
			got := false
			lastErr = rtspLive(ctx, bin, u, transport, func(f []byte) error {
				if !got {
					got = true
					rememberTransport(u, transport)
				}
				return emit(f)
			})
			if got || ctx.Err() != nil {
				return lastErr // il flusso era partito: lo riavvia chi ci chiama
			}
		}
		return lastErr
	default: // snapshot: interroga la telecamera circa una volta al secondo
		for {
			start := time.Now()
			frame, err := fetchSnapshot(ctx, cam)
			if err != nil {
				return err
			}
			if err := emit(frame); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second - time.Since(start)):
			}
		}
	}
}

// rtspLive converte lo stream RTSP in fotogrammi JPEG con ffmpeg.
func rtspLive(ctx context.Context, bin, u, transport string, emit func([]byte) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "warning",
		"-rtsp_transport", transport, "-i", u,
		"-an", "-vf", "fps=8,scale='min(1280,iw)':-2", "-f", "mjpeg", "-q:v", "6", "pipe:1")
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	// se in 20 secondi non arriva nulla, si prova l'altro trasporto
	first := time.AfterFunc(20*time.Second, cancel)
	r := bufio.NewReaderSize(stdout, 64<<10)
	for {
		frame, err := nextJPEG(r)
		if err != nil {
			first.Stop()
			_ = cmd.Wait()
			return ffmpegError(stderr.String(), err)
		}
		first.Stop()
		if err := emit(frame); err != nil {
			return err
		}
	}
}
