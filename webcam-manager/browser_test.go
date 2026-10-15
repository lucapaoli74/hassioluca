package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Pagina come quelle dei parchi: l'HTML ha solo l'etichetta, il valore arriva
// da JavaScript dopo il caricamento. Serve un browser (in CI si salta).
func TestPageValuesWithBrowser(t *testing.T) {
	if findBrowser() == "" {
		t.Skip("nessun browser disponibile")
	}
	temp := "3.8"
	mux := http.NewServeMux()
	mux.HandleFunc("/meteo/05/lago-santo/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Lago Santo</h1><div class="box"><span>Temperatura</span> <b id="t">--</b></div>
			<div>1° incontro Workshop</div>
			<script>setTimeout(()=>fetch('/x/'+'dati').then(r=>r.text()).then(v=>document.getElementById('t').innerText=v+' °C'),300)</script></body></html>`))
	})
	mux.HandleFunc("/x/dati", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(temp)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	vals, _, err := FindPageValues(context.Background(), srv.URL+"/meteo/05/lago-santo/")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vals {
		t.Logf("%s %s ← %q [%s]", v.Value, v.Unit, v.Context, v.Kind)
	}
	if len(vals) != 1 || vals[0].Kind != "browser" || vals[0].Value != "3.8" || !strings.Contains(vals[0].Context, "Temperatura") {
		t.Fatalf("atteso solo la temperatura letta col browser: %+v", vals)
	}
	temp = "-0.6"
	got, err := FetchDataSource(context.Background(), DataSource{Type: "html", Variable: "browser", URL: srv.URL + "/meteo/05/lago-santo/", Path: vals[0].Pattern, Decimals: 1, Unit: "°C"})
	if err != nil || got != "-0,6°C" {
		t.Fatalf("rilettura: %q %v", got, err)
	}
	_ = os.Getenv
}
