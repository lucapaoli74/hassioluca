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
	scan, err := FindPageValues(context.Background(), srv.URL+"/meteo/05/lago-santo/", "")
	vals := scan.Values
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

// valore senza "°C" accanto (unità disegnata come immagine/icona)
func TestPageValueWithoutUnit(t *testing.T) {
	if findBrowser() == "" {
		t.Skip("nessun browser disponibile")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/p", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><div>Temperatura</div><div><span id="t">--</span><img src="gradi.png"></div>
			<script>setTimeout(()=>document.getElementById('t').innerText='5.2',200)</script></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	scan, err := FindPageValues(context.Background(), srv.URL+"/p", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Values) == 0 || scan.Values[0].Value != "5.2" || !scan.Rendered || !strings.Contains(scan.Excerpt, "Temperatura 5.2") {
		t.Fatalf("%+v", scan)
	}
	// ricerca con la parola scelta dall'utente
	scan, _ = FindPageValues(context.Background(), srv.URL+"/p", "temperatura")
	if len(scan.Values) == 0 || scan.Values[0].Value != "5.2" {
		t.Fatalf("con etichetta: %+v", scan)
	}
}
