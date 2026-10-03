package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// wuMetric elenca le grandezze Weather Underground che stanno nel blocco
// "metric" della risposta (le altre sono al primo livello).
var wuMetric = map[string]bool{
	"temp": true, "heatIndex": true, "dewpt": true, "windChill": true, "windSpeed": true,
	"windGust": true, "pressure": true, "precipRate": true, "precipTotal": true, "elev": true,
}

// wuGlobalKey restituisce la chiave Weather Underground generale (impostata
// da App), usata dalle fonti che non hanno una chiave propria.
var wuGlobalKey func() string

// DataValue è l'ultimo valore letto da una fonte di dati live.
type DataValue struct {
	Value     string    `json:"value"`      // valore già formattato (con unità)
	UpdatedAt time.Time `json:"updated_at"` //
	Error     string    `json:"error,omitempty"`
}

// DataManager aggiorna periodicamente le fonti di dati live e ne conserva
// l'ultimo valore valido.
type DataManager struct {
	mu     sync.RWMutex
	values map[string]DataValue
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewDataManager() *DataManager {
	return &DataManager{values: map[string]DataValue{}}
}

// Restart ferma i lettori attuali e ne avvia uno per ogni fonte configurata.
func (m *DataManager) Restart(sources []DataSource) {
	if m.cancel != nil {
		m.cancel()
		m.wg.Wait()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	keep := map[string]bool{}
	for _, ds := range sources {
		keep[ds.ID] = true
		m.wg.Add(1)
		go func(ds DataSource) {
			defer m.wg.Done()
			for {
				protect("dato live "+ds.ID, func() { m.refresh(ctx, ds) })
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(ds.RefreshSeconds) * time.Second):
				}
			}
		}(ds)
	}
	m.mu.Lock()
	for id := range m.values {
		if !keep[id] {
			delete(m.values, id)
		}
	}
	m.mu.Unlock()
}

func (m *DataManager) refresh(ctx context.Context, ds DataSource) {
	val, err := FetchDataSource(ctx, ds)
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := m.values[ds.ID]
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		cur.Error = err.Error()
		log.Printf("dato live %s: %v", ds.ID, err)
	} else {
		cur = DataValue{Value: val, UpdatedAt: time.Now()}
	}
	m.values[ds.ID] = cur
}

// Value restituisce il valore attuale (stringa vuota se non ancora disponibile).
func (m *DataManager) Value(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.values[id].Value
}

func (m *DataManager) Snapshot() map[string]DataValue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]DataValue, len(m.values))
	for k, v := range m.values {
		out[k] = v
	}
	return out
}

// FetchDataSource legge una volta la fonte e restituisce il valore formattato.
func FetchDataSource(ctx context.Context, ds DataSource) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var raw any
	var err error
	switch ds.Type {
	case "open_meteo":
		q := url.Values{}
		q.Set("latitude", strconv.FormatFloat(ds.Latitude, 'f', 5, 64))
		q.Set("longitude", strconv.FormatFloat(ds.Longitude, 'f', 5, 64))
		q.Set("current", ds.Variable)
		var doc any
		doc, err = getJSON(ctx, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), "")
		if err == nil {
			raw, err = lookupPath(doc, "current."+ds.Variable)
		}
	case "wunderground":
		q := url.Values{}
		q.Set("stationId", ds.Station)
		q.Set("format", "json")
		q.Set("units", "m")
		q.Set("numericPrecision", "decimal")
		key := ds.Token
		if key == "" && wuGlobalKey != nil {
			key = wuGlobalKey()
		}
		q.Set("apiKey", key)
		var doc any
		doc, err = getJSON(ctx, "https://api.weather.com/v2/pws/observations/current?"+q.Encode(), "")
		if err == nil {
			p := "observations.0." + ds.Variable
			if wuMetric[ds.Variable] {
				p = "observations.0.metric." + ds.Variable
			}
			raw, err = lookupPath(doc, p)
		}
	case "home_assistant":
		u := strings.TrimRight(ds.URL, "/") + "/api/states/" + url.PathEscape(ds.Entity)
		var doc any
		doc, err = getJSON(ctx, u, ds.Token)
		if err == nil {
			p := ds.Path
			if p == "" {
				p = "state"
			}
			raw, err = lookupPath(doc, p)
		}
	case "http_json":
		var doc any
		doc, err = getJSON(ctx, ds.URL, ds.Token)
		if err == nil {
			raw, err = lookupPath(doc, ds.Path)
		}
	case "http_text":
		var body []byte
		body, err = getBody(ctx, ds.URL, ds.Token)
		raw = strings.TrimSpace(string(body))
	default:
		err = fmt.Errorf("tipo %q sconosciuto", ds.Type)
	}
	if err != nil {
		return "", err
	}
	return formatValue(raw, ds.Decimals, ds.Unit), nil
}

func getBody(ctx context.Context, u, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "webcam-manager")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s risponde %s", req.URL.Host, resp.Status)
	}
	return body, nil
}

func getJSON(ctx context.Context, u, token string) (any, error) {
	body, err := getBody(ctx, u, token)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("risposta non JSON: %w", err)
	}
	return doc, nil
}

// lookupPath segue un percorso puntato tipo "main.temp" o "list.0.value".
func lookupPath(doc any, path string) (any, error) {
	if path == "" {
		return doc, nil
	}
	cur := doc
	for _, part := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("campo %q non trovato", part)
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(v) {
				return nil, fmt.Errorf("indice %q non valido", part)
			}
			cur = v[i]
		default:
			return nil, fmt.Errorf("campo %q non trovato", part)
		}
	}
	return cur, nil
}

func formatValue(raw any, decimals int, unit string) string {
	var s string
	switch v := raw.(type) {
	case float64:
		s = formatNumber(v, decimals)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil && decimals >= 0 {
			s = formatNumber(f, decimals)
		} else {
			s = v
		}
	case bool:
		s = map[bool]string{true: "sì", false: "no"}[v]
	case nil:
		s = "—"
	default:
		b, _ := json.Marshal(v)
		s = string(b)
	}
	if unit != "" {
		s += unit
	}
	return s
}

// formatNumber usa la virgola decimale all'italiana.
func formatNumber(f float64, decimals int) string {
	s := strconv.FormatFloat(f, 'f', decimals, 64)
	return strings.Replace(s, ".", ",", 1)
}
