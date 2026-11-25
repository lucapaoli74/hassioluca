package main

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// pacchetti reali del WS90 "Powered by Shelly" (SBWS-90CM), 25/11/2026
const (
	ws90Climate = "4000bf016404e68b0108cf030cc8002e3545c4005f0000"
	ws90Wind    = "4000c005000000200044000044000046005e1860"
)

func TestParseBTHomeWS90(t *testing.T) {
	p, err := ParseBTHome(mustHex(t, ws90Climate))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"packet_id": 191, "battery": 100, "pressure": 1013.5, "dewpt": 9.75, "humidity": 53, "temp": 19.6, "rain_total": 0}
	for k, v := range want {
		if got := p.Values[k]; abs(got-v) > 0.001 {
			t.Errorf("%s = %v, atteso %v", k, got, v)
		}
	}
	p, err = ParseBTHome(mustHex(t, ws90Wind))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Speeds) != 2 || p.Values["winddir"] != 246 || p.Values["lux"] != 0 {
		t.Errorf("pacchetto vento: %+v", p)
	}
	// vento 3.0 m/s e raffica 5.5 m/s, in ordine inverso: medio = minore
	p, _ = ParseBTHome(mustHex(t, "4000c1050000002000442602442c010046005e1860"))
	v := ws90StationValues(p)
	if v["wind"] != 10.8 || v["gust"] != 19.8 {
		t.Errorf("vento/raffica km/h = %v/%v", v["wind"], v["gust"])
	}
	if _, err := ParseBTHome([]byte{0x41, 0x00}); err != errBTHomeEncrypted {
		t.Errorf("cifrato: %v", err)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestSeaLevelPressure(t *testing.T) {
	if p := seaLevelPressure(900, 1000); p < 1012 || p > 1015 {
		t.Errorf("900 hPa a 1000 m = %v hPa sul mare", p)
	}
	if seaLevelPressure(1013.5, 0) != 1013.5 {
		t.Error("altitudine 0: pressione invariata")
	}
}

func TestWS90Rain(t *testing.T) {
	var r ws90Rain
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	r.add("AA", 12.0, now) // primo valore: solo base
	r.add("AA", 14.5, now.Add(time.Minute))
	r.add("AA", 16.0, now.Add(2*time.Minute))
	r.add("AA", 0.3, now.Add(3*time.Minute)) // sensore riavviato
	v := r.values(now.Add(3 * time.Minute))
	if v["rain_day"] != 4.3 || v["rain_event"] != 4.3 || v["rain_hour"] != 4.3 {
		t.Errorf("pioggia: %v", v)
	}
	// giorno dopo: la giornaliera riparte, il mese no
	next := now.Add(24 * time.Hour)
	r.add("AA", 1.3, next)
	v = r.values(next)
	if v["rain_day"] != 1.0 || v["rain_month"] != 5.3 {
		t.Errorf("giorno dopo: %v", v)
	}
	// sensore diverso: nessun salto
	r.add("BB", 50, next.Add(time.Minute))
	if r.values(next.Add(time.Minute))["rain_day"] != 1.0 {
		t.Error("il cambio di sensore non deve contare pioggia")
	}
}

func TestWS90HubFeedsStation(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]float64
	h := NewWS90Hub(filepath.Join(t.TempDir(), "rain.json"), func(v map[string]float64, source, model string) {
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	})
	feed := make(chan bleAdv, 10)
	old := bleScanFunc
	bleScanFunc = func(ctx context.Context, cb func(bleAdv)) error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case a := <-feed:
				cb(a)
			}
		}
	}
	defer func() { bleScanFunc = old }()

	h.Follow("e4:0a:75:b6:ce:bb")
	feed <- bleAdv{Address: "E4:0A:75:B6:CE:BB", Name: "SBWS-90CM", RSSI: -50, BTHome: mustHex(t, ws90Climate)}
	feed <- bleAdv{Address: "E4:0A:75:B6:CE:BB", RSSI: -50, BTHome: mustHex(t, ws90Climate)} // doppione
	feed <- bleAdv{Address: "11:22:33:44:55:66", Name: "altro", RSSI: -80, BTHome: mustHex(t, ws90Wind)}
	feed <- bleAdv{Address: "E4:0A:75:B6:CE:BB", RSSI: -52, BTHome: mustHex(t, ws90Wind)}
	feed <- bleAdv{Address: "AA:AA:AA:AA:AA:AA", Name: "cuffie", RSSI: -60}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	if len(got) != 2 {
		t.Fatalf("aggiornamenti alla stazione = %d, attesi 2", len(got))
	}
	if got[0]["temp"] != 19.6 || got[0]["rain_day"] != 0 || got[1]["winddir"] != 246 {
		t.Errorf("valori: %v / %v", got[0], got[1])
	}
	mu.Unlock()

	devs, _ := h.Discover(context.Background(), 10*time.Millisecond)
	if len(devs) != 2 || devs[0].Name != "SBWS-90CM" || devs[0].Battery != 100 {
		t.Errorf("dispositivi trovati: %+v", devs)
	}
	if st := h.Status(); st["target"] != "E4:0A:75:B6:CE:BB" || st["device"] == nil {
		t.Errorf("stato: %v", st)
	}
	h.Follow("")
	time.Sleep(10 * time.Millisecond)
}
