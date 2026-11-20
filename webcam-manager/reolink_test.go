package main

import (
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReolinkSnapshot(t *testing.T) {
	frame := testJPEG(t, 640, 360, color.RGBA{10, 90, 30, 255})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/cgi-bin/api.cgi" || q.Get("cmd") != "Snap" || q.Get("rs") == "" {
			http.NotFound(w, r)
			return
		}
		if q.Get("user") != "admin" || q.Get("password") != "p@ss&word" {
			w.Write([]byte(`[{"cmd":"Snap","code":1,"error":{"detail":"login failed","rspCode":-6}}]`))
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(frame)
	}))
	defer srv.Close()
	cam := Camera{Source: "snapshot", URL: srv.URL + "/cgi-bin/api.cgi?cmd=Snap&channel=0", Username: "admin", Password: "p@ss&word"}
	got, err := CaptureFrame(context.Background(), cam)
	if err != nil || len(got) != len(frame) {
		t.Fatalf("snapshot Reolink in HTTPS: %v", err)
	}
	cam.Password = "sbagliata"
	_, err = CaptureFrame(context.Background(), cam)
	if err == nil || !strings.Contains(err.Error(), "utente o password") || strings.Contains(err.Error(), "sbagliata") {
		t.Fatalf("errore credenziali: %v", err)
	}
}

func TestDiagnoseFindsReolinkSnapshot(t *testing.T) {
	frame := testJPEG(t, 320, 180, color.RGBA{1, 2, 3, 255})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/api.cgi" && r.URL.Query().Get("user") == "admin" {
			w.Write(frame)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	rep := DiagnoseCamera(context.Background(), Camera{Source: "mjpeg", URL: srv.URL + "/ISAPI/Streaming/channels/101/picture", Username: "admin", Password: "x"})
	t.Log(rep)
	if !strings.Contains(rep, "FUNZIONA: http://"+host+"/cgi-bin/api.cgi?cmd=Snap&channel=0") {
		t.Fatal("indirizzo Reolink non proposto")
	}
}
