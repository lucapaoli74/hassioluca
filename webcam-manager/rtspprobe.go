package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Percorsi RTSP noti (EZVIZ, Hikvision, Dahua, Reolink, TP-Link, Xiongmai,
// generiche). Per le telecamere a più obiettivi il secondo canale è spesso
// ch2 / 201 / channel=2.
var knownRTSPPaths = []string{
	"/h264_stream", "/H.264",
	"/h264/ch1/main/av_stream", "/h264/ch1/sub/av_stream",
	"/h264/ch2/main/av_stream", "/h264/ch2/sub/av_stream",
	"/Streaming/Channels/101", "/Streaming/Channels/102",
	"/Streaming/Channels/201", "/Streaming/Channels/202",
	"/Streaming/Channels/301",
	"/ch1/main", "/ch1/sub", "/ch2/main", "/ch2/sub",
	"/cam/realmonitor?channel=1&subtype=0", "/cam/realmonitor?channel=2&subtype=0",
	"/stream1", "/stream2",
	"/h264Preview_01_main", "/h264Preview_02_main",
	"/live/ch00_0", "/live/ch01_0",
	"/onvif1", "/onvif2", "/11", "/12", "/21", "/22",
	"/live", "/media/video1", "/media/video2",
}

// FoundStream è un percorso RTSP che la telecamera accetta.
type FoundStream struct {
	Path  string `json:"path"`
	URL   string `json:"url"`   // senza credenziali
	Codec string `json:"codec"` // es. H264, H265
	Same  string `json:"same"`  // percorso con la stessa descrizione (probabilmente lo stesso flusso)
}

// ProbeRTSPPaths prova i percorsi noti sull'host indicato con il solo
// comando DESCRIBE (niente video, pochi millisecondi per percorso).
func ProbeRTSPPaths(ctx context.Context, host, user, pass string) ([]FoundStream, error) {
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "554")
	}
	c, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("la porta RTSP %s non risponde: %s", host, shortNetErr(err))
	}
	c.Close()

	type res struct {
		path, codec, sdp string
	}
	results := make([]*res, len(knownRTSPPaths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // poche richieste alla volta: le telecamere piccole si offendono
	for i, p := range knownRTSPPaths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			code, sdp, err := rtspDescribe(ctx, "rtsp://"+host+p, user, pass)
			if err == nil && code == 200 && strings.Contains(sdp, "m=video") {
				results[i] = &res{path: p, codec: sdpCodec(sdp), sdp: normalizeSDP(sdp)}
			}
		}(i, p)
	}
	wg.Wait()
	var out []FoundStream
	seen := map[string]string{}
	for _, r := range results {
		if r == nil {
			continue
		}
		fs := FoundStream{Path: r.path, URL: "rtsp://" + host + r.path, Codec: r.codec}
		if first, ok := seen[r.sdp]; ok {
			fs.Same = first
		} else {
			seen[r.sdp] = r.path
		}
		out = append(out, fs)
	}
	return out, nil
}

// rtspDescribe esegue DESCRIBE (con autenticazione Digest o Basic se richiesta).
func rtspDescribe(ctx context.Context, rawURL, user, pass string) (int, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, "", err
	}
	d := net.Dialer{Timeout: 4 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return 0, "", err
	}
	defer conn.Close()
	rd := bufio.NewReader(conn)
	cseq := 0
	send := func(auth string) (int, map[string]string, string, error) {
		cseq++
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: %d\r\nAccept: application/sdp\r\nUser-Agent: WebcamManager/%s\r\n", rawURL, cseq, version)
		if auth != "" {
			req += "Authorization: " + auth + "\r\n"
		}
		if _, err := conn.Write([]byte(req + "\r\n")); err != nil {
			return 0, nil, "", err
		}
		status, err := rd.ReadString('\n')
		if err != nil {
			return 0, nil, "", err
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
			if l = strings.TrimSpace(l); l == "" {
				break
			}
			if k, v, ok := strings.Cut(l, ":"); ok {
				k = strings.ToLower(strings.TrimSpace(k))
				hdr[k] = strings.TrimSpace(v)
				if k == "content-length" {
					fmt.Sscanf(hdr[k], "%d", &length)
				}
			}
		}
		body := make([]byte, min(length, 64<<10))
		if _, err := io.ReadFull(rd, body); err != nil {
			return code, hdr, "", err
		}
		return code, hdr, string(body), nil
	}
	code, hdr, body, err := send("")
	if err == nil && code == 401 {
		chal := hdr["www-authenticate"]
		auth := "Basic " + basicAuth(user, pass)
		if strings.HasPrefix(strings.ToLower(chal), "digest") {
			auth, _ = digestAuthorization(chal, "DESCRIBE", rawURL, user, pass)
		}
		code, _, body, err = send(auth)
	}
	return code, body, err
}

func sdpCodec(sdp string) string {
	for _, l := range strings.Split(sdp, "\n") {
		if strings.HasPrefix(l, "a=rtpmap:") {
			if f := strings.Fields(l); len(f) > 1 {
				codec := strings.SplitN(f[1], "/", 2)[0]
				if codec != "" && !strings.EqualFold(codec, "PCMA") && !strings.EqualFold(codec, "PCMU") && !strings.HasPrefix(strings.ToUpper(codec), "MPEG4-GENERIC") {
					return codec
				}
			}
		}
	}
	return "video"
}

// normalizeSDP toglie le righe che cambiano a ogni richiesta, per riconoscere
// percorsi diversi che portano allo stesso flusso.
func normalizeSDP(sdp string) string {
	var keep []string
	for _, l := range strings.Split(sdp, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "o=") || strings.HasPrefix(l, "a=control") || strings.HasPrefix(l, "a=range") || strings.HasPrefix(l, "s=") || strings.HasPrefix(l, "u=") {
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, "\n")
}
