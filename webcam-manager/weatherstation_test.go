package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseStationPushEcowitt(t *testing.T) {
	f := url.Values{"PASSKEY": {"ABC"}, "tempf": {"50.0"}, "humidity": {"80"}, "windspeedmph": {"10"}, "windgustmph": {"20"},
		"winddir": {"225"}, "baromrelin": {"29.92"}, "dailyrainin": {"0.5"}, "rainratein": {"0.1"}, "solarradiation": {"300.5"}, "uv": {"3"},
		"temp1f": {"32"}, "humidity1": {"55"}}
	v, proto, key := ParseStationPush(f)
	if proto != "ecowitt" || key != "ABC" {
		t.Fatal(proto, key)
	}
	want := map[string]float64{"temp": 10, "humidity": 80, "wind": 16.1, "gust": 32.2, "winddir": 225, "pressure": 1013.2, "rain_day": 12.7, "rain_rate": 2.5, "solar": 301, "uv": 3, "temp_ch1": 0, "humidity_ch1": 55}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("%s = %v, atteso %v", k, v[k], w)
		}
	}
}

func TestParseStationPushWU(t *testing.T) {
	f := url.Values{"ID": {"X1"}, "PASSWORD": {"p"}, "action": {"updateraw"}, "tempf": {"68"}, "humidity": {"50"}, "baromin": {"30.00"}, "dewptf": {"-9999"}}
	v, proto, key := ParseStationPush(f)
	if proto != "wunderground" || key != "X1" || v["temp"] != 20 || v["pressure"] != 1015.9 {
		t.Fatal(v, proto, key)
	}
	if _, ok := v["dewpt"]; ok {
		t.Fatal("-9999 deve essere ignorato")
	}
	completeStation(v)
	if v["dewpt"] < 9 || v["dewpt"] > 10 {
		t.Fatal("rugiada calcolata", v["dewpt"])
	}
}

// fakeConsole risponde come una console Ecowitt sulla porta TCP.
func fakeConsole(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	item := func(id byte, b ...byte) []byte { return append([]byte{id}, b...) }
	u16 := func(v int) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, uint16(int16(v))); return b }
	var data []byte
	data = append(data, item(0x01, u16(215)...)...)   // 21.5 interna
	data = append(data, item(0x06, 45)...)            // umidità interna
	data = append(data, item(0x09, u16(10132)...)...) // 1013.2
	data = append(data, item(0x02, u16(-35)...)...)   // -3.5
	data = append(data, item(0x07, 91)...)            // 91%
	data = append(data, item(0x0A, u16(270)...)...)
	data = append(data, item(0x0B, u16(50)...)...) // 5 m/s = 18 km/h
	data = append(data, item(0x0C, u16(100)...)...)
	data = append(data, item(0x10, u16(42)...)...) // 4.2 mm
	data = append(data, item(0x15, 0, 0, 0x9C, 0x40)...)
	data = append(data, item(0x17, 2)...)
	data = append(data, item(0xF0, 1, 2, 3)...) // campo sconosciuto: ci si ferma
	size := len(data) + 4                       // tutto tranne FF FF
	pkt := []byte{0xFF, 0xFF, 0x27, byte(size >> 8), byte(size)}
	pkt = append(pkt, data...)
	var sum byte
	for _, b := range pkt[2:] {
		sum += b
	}
	pkt = append(pkt, sum)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 5)
			if _, err := c.Read(buf); err == nil && buf[2] == 0x27 {
				c.Write(pkt)
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

func TestReadEcowittLocal(t *testing.T) {
	addr := fakeConsole(t)
	v, model, err := ReadEcowittLocal(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"temp_in": 21.5, "humidity_in": 45, "pressure": 1013.2, "temp": -3.5, "humidity": 91, "winddir": 270, "wind": 18, "gust": 36, "rain_day": 4.2, "uv": 2, "solar": 32}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("%s = %v, atteso %v (%s)", k, v[k], w, model)
		}
	}
}

func TestParseEcowittHTTP(t *testing.T) {
	body := `{"common_list":[{"id":"0x02","val":"54.5","unit":"F"},{"id":"0x07","val":"70%"},{"id":"0x0B","val":"6.5 mph"},{"id":"0x0A","val":"90"},{"id":"0x15","val":"350.12 W/m2"}],
	"rain":[{"id":"0x10","val":"0.20 in"}],"wh25":[{"intemp":"20.1","unit":"C","inhumi":"40%","rel":"1012.3 hPa","abs":"950.0 hPa"}]}`
	v, err := ParseEcowittHTTP([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"temp": 12.5, "humidity": 70, "wind": 10.5, "winddir": 90, "solar": 350.12, "rain_day": 5.1, "temp_in": 20.1, "humidity_in": 40, "pressure": 1012.3, "pressure_abs": 950}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("%s = %v, atteso %v", k, v[k], w)
		}
	}
}

func TestStationPushPageAndOverlay(t *testing.T) {
	data, out := t.TempDir(), t.TempDir()
	cfgJSON := fmt.Sprintf(`{"station":{"enabled":true,"source":"push"},
		"data_sources":[{"id":"t","name":"T","type":"station","variable":"temp","decimals":1,"unit":"°C","refresh_seconds":30},
		{"id":"d","name":"D","type":"station","variable":"winddir_text","decimals":0,"refresh_seconds":30}],
		"sites":[{"id":"locale","name":"Locale","enabled":true,"protocol":"folder","remote_dir":%q,
		"publications":[{"camera_id":"_meteo","filename":"meteo.html","interval_seconds":300}]}]}`, out)
	os.WriteFile(filepath.Join(data, "config.json"), []byte(cfgJSON), 0o600)
	app, err := NewApp(data, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer((&Server{app: app}).withAuth((&Server{app: app}).Handler()))
	defer srv.Close()
	// la console invia senza password del pannello
	resp, err := http.PostForm(srv.URL+"/data/report/", url.Values{"PASSKEY": {"K"}, "tempf": {"41"}, "humidity": {"60"}, "winddir": {"10"}, "windspeedmph": {"5"}})
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err, resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/weatherstation/updateweatherstation.php?ID=X&PASSWORD=Y&action=updateraw&tempf=42.8&humidity=61")
	var buf [16]byte
	n, _ := resp.Body.Read(buf[:])
	if !strings.HasPrefix(string(buf[:n]), "success") {
		t.Fatal(string(buf[:n]))
	}
	v, err := FetchDataSource(context.Background(), app.store.Get().DataSources[0])
	if err != nil || v != "6,0°C" {
		t.Fatal(v, err)
	}
	if v, _ := FetchDataSource(context.Background(), app.store.Get().DataSources[1]); v != "N" {
		t.Fatal("direzione", v)
	}
	errs := app.station.PublishDue(context.Background(), true)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	page, _ := os.ReadFile(filepath.Join(out, "meteo.html"))
	js, _ := os.ReadFile(filepath.Join(out, "meteo.json"))
	if !strings.Contains(string(page), `"temp":{"l":"Temperatura"`) || strings.Contains(string(page), "__DATA__") {
		t.Fatalf("pagina: %.300s", page)
	}
	var d map[string]any
	if err := json.Unmarshal(js, &d); err != nil || d["json"] != "meteo.json" {
		t.Fatal(err, d["json"])
	}
	_, pubs, _ := app.sched.Status()
	if len(pubs) != 1 || pubs[0].LastError != "" {
		t.Fatalf("stato pubblicazioni: %+v", pubs)
	}
	// stazione bloccata su una chiave: le altre vengono scartate
	cfg := app.store.Get()
	cfg.Station.AcceptKey = "K"
	app.store.Replace(cfg)
	http.PostForm(srv.URL+"/data/report/", url.Values{"PASSKEY": {"ALTRA"}, "tempf": {"100"}})
	if r := app.station.Reading(); r.Values["temp"] == round1((100-32)*5/9.0) {
		t.Fatal("dato di una stazione non accettata")
	}
	_ = time.Second
}

func TestStationCondition(t *testing.T) {
	noon := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	if c, _ := stationCondition(map[string]float64{"solar": 850, "temp": 25}, 44.2, 10.6, noon); c != "sun" {
		t.Fatal(c)
	}
	if c, _ := stationCondition(map[string]float64{"solar": 120}, 44.2, 10.6, noon); c != "cloudy" {
		t.Fatal(c)
	}
	if c, _ := stationCondition(map[string]float64{"rain_rate": 1.5, "temp": 0.5}, 44.2, 10.6, noon); c != "snow" {
		t.Fatal(c)
	}
	if c, n := stationCondition(map[string]float64{"temp": 5}, 44.2, 10.6, time.Date(2026, 6, 21, 23, 0, 0, 0, time.UTC)); c != "night" || !n {
		t.Fatal(c)
	}
}
