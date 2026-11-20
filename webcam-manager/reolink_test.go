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
