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
	vals, _, err := FindPageValues(context.Background(), srv.URL)
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
