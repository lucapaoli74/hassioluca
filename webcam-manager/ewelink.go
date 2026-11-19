package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Lettura dei sensori Sonoff (es. Zigbee SNZB-02 su bridge Sonoff, TH10/16
// con firmware originale) dal cloud eWeLink, con le API ufficiali v2 di
// CoolKit. Servono le chiavi di un'applicazione creata gratis su
// dev.ewelink.cc (APPID e APP SECRET) e l'account dell'app eWeLink.

// EWeLinkSettings sono le credenziali per il cloud eWeLink.
type EWeLinkSettings struct {
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	CountryCode string `json:"country_code"` // es. "+39"
	Region      string `json:"region"`       // eu, us, as, cn (ricavata al primo accesso)
}

// ewelinkBase restituisce l'indirizzo delle API per la regione (sostituibile nei test).
var ewelinkBase = func(region string) string {
	if b := os.Getenv("WEBCAM_MANAGER_EWELINK_BASE"); b != "" { // solo per le prove
		return b
	}
	if region == "cn" {
		return "https://cn-apia.coolkit.cn"
	}
	if region == "" {
		region = "eu"
	}
	return "https://" + region + "-apia.coolkit.cc"
}

type ewelinkSession struct {
	mu      sync.Mutex
	key     string // credenziali a cui si riferisce il token
	token   string
	region  string
	things  []ewelinkThing
	fetched time.Time
}

var ewSession = &ewelinkSession{}

type ewelinkThing struct {
	ID     string
	Name   string
	UIID   int
	Online bool
	Params map[string]any
}

func ewelinkSign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

type ewelinkResp struct {
	Error int             `json:"error"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
}

func ewelinkDo(ctx context.Context, req *http.Request) (*ewelinkResp, error) {
	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("eWeLink non raggiungibile: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var r ewelinkResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("risposta eWeLink non valida (%s)", resp.Status)
	}
	return &r, nil
}

// login accede all'account e memorizza il token (seguendo il cambio di regione).
func (s *ewelinkSession) login(ctx context.Context, st EWeLinkSettings) error {
	if st.AppID == "" || st.AppSecret == "" {
		return errors.New("servono APPID e APP SECRET della tua applicazione su dev.ewelink.cc")
	}
	if st.Email == "" || st.Password == "" {
		return errors.New("servono email e password dell'account eWeLink")
	}
	cc := st.CountryCode
	if cc == "" {
		cc = "+39"
	}
	body, _ := json.Marshal(map[string]string{"email": st.Email, "password": st.Password, "countryCode": cc})
	region := st.Region
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, ewelinkBase(region)+"/v2/user/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CK-Appid", st.AppID)
		req.Header.Set("Authorization", "Sign "+ewelinkSign(st.AppSecret, body))
		r, err := ewelinkDo(ctx, req)
		if err != nil {
			return err
		}
		var d struct {
			At     string `json:"at"`
			Region string `json:"region"`
		}
		_ = json.Unmarshal(r.Data, &d)
		if r.Error == 10004 && d.Region != "" && d.Region != region {
			region = d.Region // account in un'altra regione: si riprova lì
			continue
		}
		if r.Error != 0 || d.At == "" {
			switch r.Error {
			case 10001, 10014, 407:
				return fmt.Errorf("eWeLink: APPID o APP SECRET non validi (%d %s)", r.Error, r.Msg)
			case 10003, 10011, 10013:
				return fmt.Errorf("eWeLink: email o password errate (%d)", r.Error)
			}
			return fmt.Errorf("eWeLink: accesso non riuscito (%d %s)", r.Error, r.Msg)
		}
		if d.Region != "" {
			region = d.Region
		}
		s.token, s.region = d.At, region
		return nil
	}
	return errors.New("eWeLink: regione dell'account non trovata")
}

// devices restituisce i dispositivi dell'account (in cache per 60 secondi:
// eWeLink limita il numero di richieste).
func (s *ewelinkSession) devices(ctx context.Context, st EWeLinkSettings) ([]ewelinkThing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := st.AppID + "|" + st.Email + "|" + st.Password
	if s.key != key {
		s.key, s.token, s.things = key, "", nil
	}
	if s.things != nil && time.Since(s.fetched) < 60*time.Second {
		return s.things, nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		if s.token == "" {
			if err := s.login(ctx, st); err != nil {
				return nil, err
			}
		}
		req, _ := http.NewRequest(http.MethodGet, ewelinkBase(s.region)+"/v2/device/thing?num=0", nil)
		req.Header.Set("Authorization", "Bearer "+s.token)
		req.Header.Set("X-CK-Appid", st.AppID)
		r, err := ewelinkDo(ctx, req)
		if err != nil {
			return nil, err
		}
		if r.Error == 401 || r.Error == 402 || r.Error == 406 {
			s.token = "" // token scaduto: nuovo accesso
			continue
		}
		if r.Error != 0 {
			return nil, fmt.Errorf("eWeLink: elenco dispositivi non disponibile (%d %s)", r.Error, r.Msg)
		}
		var d struct {
			ThingList []struct {
				ItemData struct {
					DeviceID string         `json:"deviceid"`
					Name     string         `json:"name"`
					Online   bool           `json:"online"`
					Params   map[string]any `json:"params"`
					Extra    struct {
						UIID int `json:"uiid"`
					} `json:"extra"`
				} `json:"itemData"`
			} `json:"thingList"`
		}
		if err := json.Unmarshal(r.Data, &d); err != nil {
			return nil, fmt.Errorf("eWeLink: risposta non valida")
		}
		var out []ewelinkThing
		for _, t := range d.ThingList {
			it := t.ItemData
			if it.DeviceID == "" {
				continue
			}
			out = append(out, ewelinkThing{ID: it.DeviceID, Name: it.Name, UIID: it.Extra.UIID, Online: it.Online, Params: it.Params})
		}
		s.things, s.fetched = out, time.Now()
		return out, nil
	}
	return nil, errors.New("eWeLink: accesso scaduto, riprova")
}

// ewelinkValue legge temperatura o umidità dai parametri del dispositivo.
// I sensori Zigbee (SNZB-02) le inviano moltiplicate per 100 ("2156" = 21,56 °C),
// i TH10/16 come "currentTemperature": "21.5".
func ewelinkValue(t ewelinkThing, variable string) (float64, bool) {
	keys := map[string][]string{
		"temperature": {"currentTemperature", "temperature", "temp"},
		"humidity":    {"currentHumidity", "humidity", "humi"},
	}[variable]
	for _, k := range keys {
		raw, ok := t.Params[k]
		if !ok {
			continue
		}
		var v float64
		switch x := raw.(type) {
		case float64:
			v = x
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil {
				continue // es. "unavailable"
			}
			v = f
		default:
			continue
		}
		if (k == "temperature" || k == "humidity") && (t.UIID == 1770 || t.UIID == 7014 || math.Abs(v) >= 100) {
			v /= 100
		}
		return v, true
	}
	return 0, false
}

// ReadEWeLink elenca temperatura e umidità di tutti i sensori dell'account.
func ReadEWeLink(ctx context.Context, st EWeLinkSettings) ([]SensorReading, error) {
	things, err := ewSession.devices(ctx, st)
	if err != nil {
		return nil, err
	}
	var out []SensorReading
	for _, t := range things {
		for _, v := range []struct{ key, label, unit string }{{"temperature", "Temperatura", "°C"}, {"humidity", "Umidità", "%"}} {
			val, ok := ewelinkValue(t, v.key)
			if !ok {
				continue
			}
			label := v.label + " – " + t.Name
			if !t.Online {
				label += " (offline)"
			}
			out = append(out, SensorReading{Label: label, Value: formatNumber(val, 1), Unit: v.unit, Type: "ewelink",
				Entity: t.ID, Path: v.key})
		}
	}
	sortReadings(out)
	if len(out) == 0 {
		return nil, errors.New("nell'account eWeLink non ci sono sensori di temperatura o umidità")
	}
	return out, nil
}

// readEWeLinkSource legge il valore di un dato live di tipo "ewelink".
func readEWeLinkSource(ctx context.Context, ds DataSource, st EWeLinkSettings) (any, error) {
	things, err := ewSession.devices(ctx, st)
	if err != nil {
		return nil, err
	}
	for _, t := range things {
		if t.ID != ds.Entity {
			continue
		}
		if !t.Online {
			return nil, fmt.Errorf("il sensore %q è offline in eWeLink", t.Name)
		}
		if v, ok := ewelinkValue(t, ds.Path); ok {
			return v, nil
		}
		return nil, fmt.Errorf("il sensore %q non ha il valore %s", t.Name, ds.Path)
	}
	return nil, fmt.Errorf("sensore %s non trovato nell'account eWeLink", ds.Entity)
}
