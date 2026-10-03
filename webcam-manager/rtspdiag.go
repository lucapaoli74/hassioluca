package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// DiagnoseCamera esegue una serie di controlli sulla telecamera e restituisce
// un rapporto leggibile (senza password) da copiare nelle segnalazioni.
func DiagnoseCamera(ctx context.Context, cam Camera) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	cam.Source = effectiveSource(cam)
	line("Webcam Manager %s · %s · %s", version, platformKey(), time.Now().Format("02/01/2006 15:04:05"))
	line("Sorgente: %s  URL: %s", cam.Source, hideCreds(cam.URL))
	if cam.Source != "rtsp" {
		_, err := CaptureFrame(ctx, cam)
		if err != nil {
			line("Cattura: ERRORE %v", err)
		} else {
			line("Cattura: OK")
		}
		return b.String()
	}

	u, err := url.Parse(rtspURL(cam))
	if err != nil {
		line("URL non valido: %v", err)
		return b.String()
	}
	user, pass := cam.Username, cam.Password
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "554")
	}
	clean := *u
	clean.User = nil
	reqURL := clean.String()
	line("Credenziali: utente %q, password %s", user, map[bool]string{true: "presente", false: "ASSENTE"}[pass != ""])

	// 1. porta
	start := time.Now()
	conn, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		line("\n[1] Porta %s: NON RAGGIUNGIBILE (%v)", host, err)
		line("    → la telecamera non è raggiungibile da questo PC o RTSP è spento")
		return b.String()
	}
	line("\n[1] Porta %s: aperta (%d ms)", host, time.Since(start).Milliseconds())

	// 2. dialogo RTSP
	line("\n[2] Dialogo RTSP")
	rd := bufio.NewReader(conn)
	cseq := 0
	send := func(method, extra string) (int, map[string]string, string, error) {
		cseq++
		_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
		req := fmt.Sprintf("%s %s RTSP/1.0\r\nCSeq: %d\r\nUser-Agent: WebcamManager/%s\r\n%s\r\n", method, reqURL, cseq, version, extra)
		if _, err := conn.Write([]byte(req)); err != nil {
			return 0, nil, "", err
		}
		status, err := rd.ReadString('\n')
		if err != nil {
			return 0, nil, "", fmt.Errorf("nessuna risposta (%v)", err)
		}
		code := 0
		fmt.Sscanf(strings.TrimSpace(status), "RTSP/1.0 %d", &code)
		hdr := map[string]string{}
		length := 0
		for {
			l, err := rd.ReadString('\n')
			if err != nil {
				return code, hdr, "", err
			}
			l = strings.TrimSpace(l)
			if l == "" {
				break
			}
			if k, v, ok := strings.Cut(l, ":"); ok {
				hdr[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
				if strings.EqualFold(strings.TrimSpace(k), "content-length") {
					fmt.Sscanf(strings.TrimSpace(v), "%d", &length)
				}
			}
		}
		body := make([]byte, min(length, 64<<10))
		if _, err := ioReadFull(rd, body); err != nil {
			return code, hdr, "", err
		}
		return code, hdr, string(body), nil
	}
	code, hdr, _, err := send("OPTIONS", "")
	if err != nil {
		line("    OPTIONS: %v", err)
		line("    → la porta è aperta ma la telecamera non parla RTSP (o è occupata da un altro programma)")
		conn.Close()
		return b.String() + ffmpegDebug(ctx, cam)
	}
	line("    OPTIONS: %d  server=%q  metodi=%q", code, hdr["server"], hdr["public"])
	code, hdr, body, err := send("DESCRIBE", "Accept: application/sdp\r\n")
	if err == nil && code == 401 {
		chal := hdr["www-authenticate"]
		line("    DESCRIBE senza credenziali: 401, richiede %s", strings.SplitN(chal, " ", 2)[0])
		auth := ""
		if strings.HasPrefix(strings.ToLower(chal), "digest") {
			auth, _ = digestAuthorization(chal, "DESCRIBE", reqURL, user, pass)
		} else {
			auth = "Basic " + basicAuth(user, pass)
		}
		code, hdr, body, err = send("DESCRIBE", "Accept: application/sdp\r\nAuthorization: "+auth+"\r\n")
	}
	conn.Close()
	switch {
	case err != nil:
		line("    DESCRIBE: %v", err)
	case code == 200:
		line("    DESCRIBE: 200 OK — credenziali e percorso CORRETTI")
		for _, l := range strings.Split(body, "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "m=") || strings.HasPrefix(l, "a=rtpmap") || strings.HasPrefix(l, "a=control") {
				line("      %s", l)
			}
		}
	case code == 401:
		line("    DESCRIBE: 401 — CREDENZIALI RIFIUTATE (per EZVIZ: utente admin, password = codice di verifica)")
	case code == 404:
		line("    DESCRIBE: 404 — PERCORSO SBAGLIATO (prova /H.264 o /h264/ch1/main/av_stream)")
	default:
		line("    DESCRIBE: %d", code)
	}
	return b.String() + ffmpegDebug(ctx, cam)
}

// ffmpegDebug lancia ffmpeg per 15 secondi in modalità dettagliata.
func ffmpegDebug(ctx context.Context, cam Camera) string {
	var b strings.Builder
	bin, err := ffmpegPath()
	if err != nil {
		return fmt.Sprintf("\n[3] ffmpeg: %v\n", err)
	}
	fmt.Fprintf(&b, "\n[3] ffmpeg (%s), 15 secondi, TCP\n", bin)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "verbose", "-rtsp_transport", "tcp",
		"-i", rtspURL(cam), "-frames:v", "1", "-f", "image2", "-c:v", "mjpeg", "pipe:1")
	hideWindow(cmd)
	tail := &tailBuffer{}
	cmd.Stderr = tail
	out, err := cmd.Output()
	switch {
	case err == nil && len(out) > 2:
		fmt.Fprintf(&b, "    RISULTATO: immagine ricevuta (%d KB)\n", len(out)/1024)
	case ctx.Err() != nil:
		b.WriteString("    RISULTATO: nessuna immagine in 15 secondi\n")
	default:
		fmt.Fprintf(&b, "    RISULTATO: errore %v\n", err)
	}
	lines := strings.Split(strings.TrimSpace(hideCreds(tail.String())), "\n")
	if len(lines) > 30 {
		lines = lines[len(lines)-30:]
	}
	for _, l := range lines {
		fmt.Fprintf(&b, "    | %s\n", strings.TrimRight(l, "\r"))
	}
	return b.String()
}

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

func ioReadFull(r *bufio.Reader, b []byte) (int, error) { return io.ReadFull(r, b) }
