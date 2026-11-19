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

func TestEWeLinkOAuth(t *testing.T) {
	tokenNow := "AT1"
	mux := http.NewServeMux()
	mux.HandleFunc("/eu/v2/user/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Sign "+ewelinkSign("secret1", body) || !strings.Contains(string(body), `"code":"C0DE"`) {
			w.Write([]byte(`{"error":400,"msg":"bad"}`))
			return
		}
		w.Write([]byte(`{"error":0,"data":{"accessToken":"AT1","refreshToken":"RT1","atExpiredTime":1,"rtExpiredTime":2}}`))
	})
	mux.HandleFunc("/eu/v2/user/refresh", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"rt":"RT1"`) {
			w.Write([]byte(`{"error":401,"msg":"rt"}`))
			return
		}
		tokenNow = "AT2"
		w.Write([]byte(`{"error":0,"data":{"at":"AT2","rt":"RT2"}}`))
	})
	mux.HandleFunc("/eu/v2/device/thing", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tokenNow {
			w.Write([]byte(`{"error":401,"msg":"token expired"}`))
			return
		}
		w.Write([]byte(`{"error":0,"data":{"thingList":[{"itemData":{"deviceid":"a1","name":"SNZB","online":true,"extra":{"uiid":1770},"params":{"temperature":"-245","humidity":"9120"}}}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := ewelinkBase
	ewelinkBase = func(region string) string { return srv.URL + "/" + region }
	defer func() { ewelinkBase = old }()

	st := EWeLinkSettings{AppID: "app1", AppSecret: "secret1"}
	u := EWeLinkAuthURL(st, "http://localhost:8080/api/ewelink/callback", "abcdefgh12345678")
	if !strings.HasPrefix(u, "https://c2ccdn.coolkit.cc/oauth/index.html?") || !strings.Contains(u, "clientId=app1") || !strings.Contains(u, "redirectUrl=http%3A%2F%2Flocalhost%3A8080%2Fapi%2Fewelink%2Fcallback") {
		t.Fatalf("url: %s", u)
	}
	st, err := EWeLinkExchange(context.Background(), st, "C0DE", "eu", "http://localhost:8080/api/ewelink/callback")
	if err != nil || st.AccessToken != "AT1" || st.RefreshToken != "RT1" {
		t.Fatalf("scambio: %+v %v", st, err)
	}
	ewSession = &ewelinkSession{}
	r, err := ReadEWeLink(context.Background(), st)
	if err != nil || len(r) != 2 || r[0].Value != "-2,5" && r[0].Value != "-2,4" {
		t.Fatalf("lettura: %+v %v", r, err)
	}
	// il token scade: deve rinnovarsi da solo con il refresh token e salvarlo
	tokenNow = "EXPIRED"
	saved := ""
	ewelinkSaveTokens = func(at, rt, region string) { saved = at + "/" + rt }
	defer func() { ewelinkSaveTokens = nil }()
	ewSession.fetched = ewSession.fetched.Add(-2 * 60e9)
	if _, err := ReadEWeLink(context.Background(), st); err != nil || saved != "AT2/RT2" {
		t.Fatalf("rinnovo: saved=%q err=%v", saved, err)
	}
}
