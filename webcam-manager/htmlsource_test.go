package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageValues(t *testing.T) {
	temp := "4,3"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><style>.x{}</style><script>var t=99;</script></head><body>
			<h1>Lago Santo - Rifugio Vittoria</h1>
			<table><tr><td>Temperatura aria</td><td><b>` + temp + `&nbsp;°C</b></td></tr>
			<tr><td>Umidità relativa:</td><td>87 %</td></tr>
			<tr><td>Vento</td><td>12,5 km/h</td></tr>
			<tr><td>Pressione</td><td>1013.2 hPa</td></tr></table></body></html>`))
	}))
	defer srv.Close()
	scan, err := FindPageValues(context.Background(), srv.URL, "")
	vals := scan.Values
	if err != nil {
		t.Fatal(err)
	}
	var tv *FoundValue
	for i, v := range vals {
		t.Logf("%s %s ← %q  [%s]", v.Value, v.Unit, v.Context, v.Pattern)
		if strings.Contains(v.Context, "Temperatura") {
			tv = &vals[i]
		}
	}
	if tv == nil || tv.Value != "4,3" || tv.Unit != "°C" || tv.Context != "Temperatura aria" {
		t.Fatalf("temperatura non trovata: %+v", vals)
	}
	// il valore cambia: il dato live deve leggere quello nuovo
	temp = "-2,8"
	ds := DataSource{ID: "t", Type: "html", URL: srv.URL, Path: tv.Pattern, Decimals: 1, Unit: "°C"}
	got, err := FetchDataSource(context.Background(), ds)
	if err != nil || got != "-2,8°C" {
		t.Fatalf("got %q %v", got, err)
	}
	if len(vals) < 4 {
		t.Fatalf("attesi almeno 4 valori, trovati %d", len(vals))
	}
}

func TestPageValuesLoadedByJavaScript(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json-page", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><div id="t">--</div><script>fetch('/dati/meteo.json?v=2').then(r=>r.json()).then(d=>{t.innerText=d.current.temp})</script></body></html>`))
	})
	mux.HandleFunc("/dati/meteo.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stazione":"Lago Santo","current":{"temp":3.4,"hum":88,"wind_speed":5.2,"time":"12:00"}}`))
	})
	mux.HandleFunc("/iframe-page", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Meteo</h1><iframe src="/widget.html"></iframe></body></html>`))
	})
	mux.HandleFunc("/widget.html", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<div>Temperatura esterna <b>2,1 °C</b></div>`))
	})
	mux.HandleFunc("/script-page", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><span id="x"></span><script>var temperatura = -1.5; var umidita = 93;</script></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()

	find := func(page, ctxWord string) FoundValue {
		scan, err := FindPageValues(ctx, srv.URL+page, "")
		vals := scan.Values
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range vals {
			t.Logf("%s: %s %s ← %s [%s %s]", page, v.Value, v.Unit, v.Context, v.Kind, v.Source)
			if strings.Contains(strings.ToLower(v.Context), ctxWord) {
				return v
			}
		}
		t.Fatalf("%s: valore %q non trovato", page, ctxWord)
		return FoundValue{}
	}

	// dati da file JSON → si legge come "URL JSON"
	v := find("/json-page", "temp")
	if v.Kind != "json" || v.Value != "3.4" || v.Pattern != "current.temp" || v.Context != "Temperatura (current.temp)" {
		t.Fatalf("json: %+v", v)
	}
	got, err := FetchDataSource(ctx, DataSource{Type: "http_json", URL: v.Source, Path: v.Pattern, Decimals: 1, Unit: "°C"})
	if err != nil || got != "3,4°C" {
		t.Fatalf("lettura json: %q %v", got, err)
	}
	// riquadro incorporato
	v = find("/iframe-page", "temperatura")
	got, err = FetchDataSource(ctx, DataSource{Type: "html", URL: v.Source, Path: v.Pattern, Decimals: 1, Unit: "°C"})
	if err != nil || got != "2,1°C" {
		t.Fatalf("lettura iframe: %q %v", got, err)
	}
	// variabile in uno script
	v = find("/script-page", "temperatura")
	got, err = FetchDataSource(ctx, DataSource{Type: "html", Variable: "raw", URL: v.Source, Path: v.Pattern, Decimals: 1, Unit: "°C"})
	if err != nil || got != "-1,5°C" {
		t.Fatalf("lettura script: %q %v", got, err)
	}
}
