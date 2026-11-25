package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sensori locali (es. Sonoff TH, SNZB-02) letti senza cloud:
//   - firmware Tasmota:  http://IP/cm?cmnd=Status%2010  (JSON StatusSNS)
//   - firmware ESPHome:  web server, flusso /events e REST /sensor/<id>
//   - Home Assistant:    /api/states (qualsiasi sensore già in HA, anche Zigbee)
// Ogni lettura trovata diventa un normale dato live (URL JSON o Home Assistant).

// SensorReading è un valore letto da un dispositivo, pronto per diventare un dato live.
type SensorReading struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Unit   string `json:"unit"`
	Type   string `json:"type"`   // tipo del dato live: "http_json" o "home_assistant"
	URL    string `json:"url"`    // indirizzo da leggere
	Path   string `json:"path"`   // campo JSON (http_json) o attributo (HA)
	Entity string `json:"entity"` // home_assistant
}

// SensorDevice è un dispositivo trovato nella rete locale.
type SensorDevice struct {
	Host string `json:"host"`
	Kind string `json:"kind"` // "tasmota" | "esphome"
	Name string `json:"name"`
}

// deviceURL costruisce l'indirizzo con eventuali credenziali (Basic auth).
func deviceURL(host, user, pass, path string) string {
	host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(host), "http://"), "https://"), "/")
	u := url.URL{Scheme: "http", Host: host, Path: path}
	if user != "" {
		u.User = url.UserPassword(user, pass)
	}
	return u.String()
}

// ReadTasmota legge tutte le grandezze dei sensori collegati a un Tasmota.
func ReadTasmota(ctx context.Context, host, user, pass string) ([]SensorReading, error) {
	base := deviceURL(host, "", "", "/cm")
	q := url.Values{"cmnd": {"Status 10"}}
	if user != "" {
		q.Set("user", user)
		q.Set("password", pass)
	}
	u := base + "?" + q.Encode()
	doc, err := getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("Tasmota non risponde all'indirizzo %s: %w", host, err)
	}
	sns, err := lookupPath(doc, "StatusSNS")
	if err != nil {
		return nil, fmt.Errorf("il dispositivo non sembra un Tasmota con sensori")
	}
	unit := "°C"
	if m, ok := sns.(map[string]any); ok {
		if tu, ok := m["TempUnit"].(string); ok && tu != "" {
			unit = "°" + tu
		}
	}
	var out []SensorReading
	var walk func(v any, path, sensor string)
	walk = func(v any, path, sensor string) {
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if k == "Time" || k == "TempUnit" || k == "PressureUnit" || k == "SpeedUnit" {
					continue
				}
				s := sensor
				if _, isObj := t[k].(map[string]any); isObj {
					s = k
				}
				walk(t[k], path+"."+k, s)
			}
		case float64:
			field := path[strings.LastIndex(path, ".")+1:]
			label := friendlyKey(field)
			if sensor != "" {
				label += " (" + sensor + ")"
			}
			un := unitForKey(field)
			if strings.HasPrefix(un, "°") && strings.Contains(strings.ToLower(field), "temp") {
				un = unit
			}
			out = append(out, SensorReading{Label: label, Value: formatNumber(t, 1), Unit: un, Type: "http_json",
				URL: u, Path: strings.TrimPrefix(path, ".")})
		}
	}
	walk(sns, "StatusSNS", "")
	sortReadings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("il Tasmota risponde ma non ha sensori collegati")
	}
	return out, nil
}

// ReadESPHome legge le grandezze esposte dal web server di ESPHome.
func ReadESPHome(ctx context.Context, host, user, pass string) ([]SensorReading, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, deviceURL(host, user, pass, "/events"), nil)
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ESPHome non risponde all'indirizzo %s: %w", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("ESPHome chiede utente e password")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPHome risponde %s (è attivo il componente web_server?)", resp.Status)
	}
	var out []SensorReading
	seen := map[string]bool{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		var ev struct {
			ID    string   `json:"id"`
			Name  string   `json:"name"`
			Value *float64 `json:"value"`
			State string   `json:"state"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(data)), &ev) != nil || ev.Value == nil {
			continue
		}
		obj, ok := strings.CutPrefix(ev.ID, "sensor-")
		if !ok || seen[obj] {
			continue
		}
		seen[obj] = true
		label := ev.Name
		if label == "" {
			label = strings.ReplaceAll(obj, "_", " ")
		}
		unit := strings.TrimSpace(strings.TrimLeft(ev.State, "-0123456789.,"))
		if unit == "" {
			unit = unitForKey(obj)
		}
		out = append(out, SensorReading{Label: label, Value: formatNumber(*ev.Value, 1), Unit: unit, Type: "http_json",
			URL: deviceURL(host, user, pass, "/sensor/"+obj), Path: "value"})
	}
	sortReadings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("nessun sensore trovato su ESPHome (serve il componente web_server)")
	}
	return out, nil
}

// ReadHomeAssistant elenca i sensori di Home Assistant con un'unità di misura.
func ReadHomeAssistant(ctx context.Context, haURL, token string) ([]SensorReading, error) {
	base := strings.TrimRight(strings.TrimSpace(haURL), "/")
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	doc, err := getJSON(ctx, base+"/api/states", token)
	if err != nil {
		return nil, fmt.Errorf("Home Assistant: %w (indirizzo e token corretti?)", err)
	}
	list, _ := doc.([]any)
	var out []SensorReading
	for _, e := range list {
		m, _ := e.(map[string]any)
		id, _ := m["entity_id"].(string)
		state, _ := m["state"].(string)
		attrs, _ := m["attributes"].(map[string]any)
		unit, _ := attrs["unit_of_measurement"].(string)
		dc, _ := attrs["device_class"].(string)
		if !strings.HasPrefix(id, "sensor.") || unit == "" || state == "unavailable" || state == "unknown" {
			continue
		}
		if !meteoUnit(unit) && dc != "temperature" && dc != "humidity" && dc != "pressure" {
			continue
		}
		name, _ := attrs["friendly_name"].(string)
		if name == "" {
			name = id
		}
		out = append(out, SensorReading{Label: name, Value: state, Unit: unit, Type: "home_assistant", URL: base, Entity: id})
	}
	sortReadings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("in Home Assistant non ci sono sensori di temperatura o umidità")
	}
	return out, nil
}

func meteoUnit(u string) bool {
	switch u {
	case "°C", "°F", "%", "hPa", "mbar", "km/h", "m/s", "mm", "mm/h", "lx", "W/m²", "UV index":
		return true
	}
	return false
}

var reTitle = regexp.MustCompile(`(?is)<title>(.*?)</title>`)

// FindSensorDevices cerca nella rete locale i dispositivi Tasmota ed ESPHome
// (scansione leggera della porta 80, poche connessioni alla volta).
func FindSensorDevices(ctx context.Context) []SensorDevice {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	hosts := localHosts()
	jobs := make(chan string)
	var mu sync.Mutex
	var out []SensorDevice
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 3 * time.Second}
	for w := 0; w < scanWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := net.Dialer{Timeout: 600 * time.Millisecond}
			for h := range jobs {
				c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(h, "80"))
				if err != nil {
					continue
				}
				c.Close()
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+h+"/", nil)
				resp, err := client.Do(req)
				if err != nil {
					continue
				}
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()
				low := strings.ToLower(string(body))
				kind := ""
				switch {
				case strings.Contains(low, "tasmota"):
					kind = "tasmota"
				case strings.Contains(low, "esphome") || strings.Contains(low, "esp-app"):
					kind = "esphome"
				}
				if kind == "" {
					continue
				}
				name := ""
				if m := reTitle.FindSubmatch(body); m != nil {
					name = strings.TrimSpace(string(m[1]))
				}
				mu.Lock()
				out = append(out, SensorDevice{Host: h, Kind: kind, Name: name})
				mu.Unlock()
			}
		}()
	}
loop:
	for _, h := range hosts {
		select {
		case jobs <- h:
			time.Sleep(10 * time.Millisecond)
		case <-ctx.Done():
			break loop
		}
	}
	close(jobs)
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

// sortReadings mette prima le temperature, poi l'umidità, poi il resto
// (il punto di rugiada, pur in °C, va dopo).
func sortReadings(r []SensorReading) {
	rank := func(x SensorReading) int {
		l := strings.ToLower(x.Label)
		switch {
		case strings.Contains(l, "rugiad") || strings.Contains(l, "dew"):
			return 3
		case strings.HasPrefix(x.Unit, "°"):
			return 0
		case x.Unit == "%":
			return 1
		}
		return 2
	}
	sort.SliceStable(r, func(i, j int) bool { return rank(r[i]) < rank(r[j]) })
}
