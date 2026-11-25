package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadLocalSensors(t *testing.T) {
	mux := http.NewServeMux()
	// Tasmota (es. Sonoff TH16 con sonda AM2301)
	mux.HandleFunc("/cm", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cmnd") != "Status 10" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"StatusSNS":{"Time":"2026-11-19T10:00:00","AM2301":{"Temperature":21.4,"Humidity":48.0,"DewPoint":10.1},"TempUnit":"C"}}`))
	})
	// ESPHome (web_server)
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: ping\ndata: {}\n\n")
		fmt.Fprint(w, "event: state\ndata: {\"id\":\"sensor-temperatura_esterna\",\"name\":\"Temperatura esterna\",\"value\":7.25,\"state\":\"7.2 °C\"}\n\n")
		fmt.Fprint(w, "event: state\ndata: {\"id\":\"sensor-umidita\",\"name\":\"Umidità\",\"value\":81,\"state\":\"81 %\"}\n\n")
		fmt.Fprint(w, "event: state\ndata: {\"id\":\"switch-relay\",\"state\":\"ON\"}\n\n")
	})
	mux.HandleFunc("/sensor/temperatura_esterna", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"sensor-temperatura_esterna","value":7.25,"state":"7.2 °C"}`))
	})
	// Home Assistant
	mux.HandleFunc("/api/states", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`[{"entity_id":"sensor.snzb02_temperature","state":"19.6","attributes":{"unit_of_measurement":"°C","device_class":"temperature","friendly_name":"Sonoff SNZB-02 Temperatura"}},
			{"entity_id":"sensor.snzb02_humidity","state":"55","attributes":{"unit_of_measurement":"%","device_class":"humidity","friendly_name":"Sonoff SNZB-02 Umidità"}},
			{"entity_id":"sensor.cpu","state":"12","attributes":{"unit_of_measurement":"MHz"}},
			{"entity_id":"light.sala","state":"on","attributes":{}}]`))
	})
	mux.HandleFunc("/api/states/sensor.snzb02_temperature", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"entity_id":"sensor.snzb02_temperature","state":"19.6"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ctx := context.Background()

	tas, err := ReadTasmota(ctx, host, "", "")
	if err != nil || len(tas) != 3 || tas[0].Label != "Temperatura (AM2301)" || tas[0].Unit != "°C" {
		t.Fatalf("tasmota: %+v %v", tas, err)
	}
	got, err := FetchDataSource(ctx, DataSource{Type: tas[0].Type, URL: tas[0].URL, Path: tas[0].Path, Decimals: 1, Unit: "°C"})
	if err != nil || got != "21,4°C" {
		t.Fatalf("lettura tasmota: %q %v", got, err)
	}

	esp, err := ReadESPHome(ctx, host, "", "")
	if err != nil || len(esp) != 2 || esp[0].Label != "Temperatura esterna" || esp[0].Unit != "°C" || esp[1].Unit != "%" {
		t.Fatalf("esphome: %+v %v", esp, err)
	}
	got, err = FetchDataSource(ctx, DataSource{Type: esp[0].Type, URL: esp[0].URL, Path: esp[0].Path, Decimals: 1, Unit: "°C"})
	if err != nil || got != "7,3°C" && got != "7,2°C" {
		t.Fatalf("lettura esphome: %q %v", got, err)
	}

	ha, err := ReadHomeAssistant(ctx, srv.URL, "tok")
	if err != nil || len(ha) != 2 || ha[0].Entity != "sensor.snzb02_temperature" {
		t.Fatalf("home assistant: %+v %v", ha, err)
	}
	got, err = FetchDataSource(ctx, DataSource{Type: "home_assistant", URL: ha[0].URL, Token: "tok", Entity: ha[0].Entity, Decimals: 1, Unit: "°C"})
	if err != nil || got != "19,6°C" {
		t.Fatalf("lettura HA: %q %v", got, err)
	}
}
