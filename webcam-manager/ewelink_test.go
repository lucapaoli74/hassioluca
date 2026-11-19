package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEWeLinkCloud(t *testing.T) {
	logins := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/eu/v2/user/login", func(w http.ResponseWriter, r *http.Request) {
		// l'account è nella regione "eu" ma il primo tentativo arriva su "us"
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-CK-Appid") != "app1" || r.Header.Get("Authorization") != "Sign "+ewelinkSign("secret1", body) {
			w.Write([]byte(`{"error":407,"msg":"appid invalid"}`))
			return
		}
		if !strings.Contains(string(body), `"password":"pw"`) {
			w.Write([]byte(`{"error":10003,"msg":"wrong password"}`))
			return
		}
		logins++
		w.Write([]byte(`{"error":0,"data":{"at":"TOKEN","rt":"R","region":"eu"}}`))
	})
	mux.HandleFunc("/us/v2/user/login", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":10004,"msg":"region","data":{"region":"eu"}}`))
	})
	mux.HandleFunc("/eu/v2/device/thing", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer TOKEN" {
			w.Write([]byte(`{"error":401,"msg":"token"}`))
			return
		}
		w.Write([]byte(`{"error":0,"data":{"thingList":[
			{"itemType":1,"itemData":{"deviceid":"a48001","name":"Sensore piazzola","online":true,"extra":{"uiid":1770},"params":{"temperature":"2156","humidity":"5512","battery":95}}},
			{"itemType":1,"itemData":{"deviceid":"1000aa","name":"TH16 reception","online":true,"extra":{"uiid":15},"params":{"currentTemperature":"21.5","currentHumidity":"48","switch":"on"}}},
			{"itemType":1,"itemData":{"deviceid":"1000bb","name":"Presa","online":true,"extra":{"uiid":1},"params":{"switch":"off"}}}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := ewelinkBase
	ewelinkBase = func(region string) string {
		if region == "" {
			region = "us"
		}
		return srv.URL + "/" + region
	}
	defer func() { ewelinkBase = old }()
	ewSession = &ewelinkSession{}
	ctx := context.Background()

	if _, err := ReadEWeLink(ctx, EWeLinkSettings{AppID: "app1", AppSecret: "secret1", Email: "x@y.it", Password: "sbagliata"}); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("password errata non segnalata: %v", err)
	}
	ewSession = &ewelinkSession{}
	st := EWeLinkSettings{AppID: "app1", AppSecret: "secret1", Email: "x@y.it", Password: "pw"}
	r, err := ReadEWeLink(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range r {
		t.Logf("%s = %s %s [%s %s]", v.Label, v.Value, v.Unit, v.Entity, v.Path)
	}
	if len(r) != 4 || r[0].Value != "21,6" && r[0].Value != "21,5" {
		t.Fatalf("letture: %+v", r)
	}
	ewelinkSettings = func() EWeLinkSettings { return st }
	got, err := FetchDataSource(ctx, DataSource{Type: "ewelink", Entity: "a48001", Path: "humidity", Decimals: 0, Unit: "%"})
	if err != nil || got != "55%" {
		t.Fatalf("lettura SNZB-02: %q %v", got, err)
	}
	got, err = FetchDataSource(ctx, DataSource{Type: "ewelink", Entity: "a48001", Path: "temperature", Decimals: 1, Unit: "°C"})
	if err != nil || got != "21,6°C" {
		t.Fatalf("temperatura SNZB-02: %q %v", got, err)
	}
	if logins != 1 {
		t.Fatalf("accessi a eWeLink: %d (deve riusare il token)", logins)
	}
}
