package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stazione meteo personale (Froggit, Ecowitt, Fine Offset, Bresser, Sainlogic…)
// letta in tempo reale nella rete locale, senza passare dai siti meteo:
//
//   - "push": la console invia i dati al programma (menu "Server
//     personalizzato"/"Customized" della app WS View / WSLink, protocollo
//     Ecowitt o Wunderground) ogni 16-60 secondi;
//   - "local": il programma interroga la console sulla porta 45000
//     (protocollo Ecowitt) o con la pagina /get_livedata_info;
//   - "wunderground": riserva, dal cloud Weather Underground.

// StationSettings è la configurazione della stazione meteo.
type StationSettings struct {
	Enabled        bool        `json:"enabled"`
	Name           string      `json:"name"`
	Source         string      `json:"source"`          // "push" | "local" | "wunderground"
	Host           string      `json:"host"`            // local: indirizzo IP della console
	StationID      string      `json:"station_id"`      // wunderground: ID stazione
	APIKey         string      `json:"api_key"`         // wunderground: chiave (vuota = generale)
	AcceptKey      string      `json:"accept_key"`      // push: accetta solo questo PASSKEY/ID (vuoto = tutti)
	RefreshSeconds int         `json:"refresh_seconds"` // local/wunderground: ogni quanto leggere
	Page           StationPage `json:"page"`
}

// StationPage è l'aspetto della pagina meteo; dove e ogni quanto pubblicarla
// si sceglie nei siti, come per le webcam ("Pagina meteo").
type StationPage struct {
	Title    string   `json:"title"`
	CameraID string   `json:"camera_id"` // webcam nella pagina: "" = la prima del sito, "none" = nessuna
	Fields   []string `json:"fields"`    // valori da mostrare (vuoto = quelli principali)
}

func (s *StationSettings) applyDefaults() {
	if s.Source == "" { // mai configurata: pronta a ricevere
		s.Source, s.Enabled = "push", true
	}
	if s.RefreshSeconds < 15 {
		s.RefreshSeconds = 60
	}
	if s.Source == "wunderground" && s.RefreshSeconds < 120 {
		s.RefreshSeconds = 300
	}
}

func (s StationSettings) validate() error {
	switch s.Source {
	case "push", "local", "wunderground":
	default:
		return fmt.Errorf("stazione meteo: collegamento %q sconosciuto", s.Source)
	}
	return nil
}

// stationVar descrive una grandezza della stazione.
type stationVar struct {
	Key, Label, Unit string
	Decimals         int
}

var stationVars = []stationVar{
	{"temp", "Temperatura", "°C", 1}, {"feels", "Percepita", "°C", 1},
	{"temp_min", "Minima oggi", "°C", 1}, {"temp_max", "Massima oggi", "°C", 1},
	{"humidity", "Umidità", "%", 0}, {"dewpt", "Punto di rugiada", "°C", 1},
	{"wind", "Vento", " km/h", 1}, {"gust", "Raffica", " km/h", 1}, {"gust_max", "Raffica max oggi", " km/h", 1},
	{"winddir", "Direzione vento", "°", 0}, {"winddir_text", "Direzione vento (testo)", "", 0},
	{"pressure", "Pressione", " hPa", 1}, {"pressure_abs", "Pressione assoluta", " hPa", 1},
	{"rain_rate", "Intensità pioggia", " mm/h", 1}, {"rain_day", "Pioggia oggi", " mm", 1},
	{"rain_event", "Pioggia evento", " mm", 1}, {"rain_hour", "Pioggia ultima ora", " mm", 1},
	{"rain_week", "Pioggia settimana", " mm", 1}, {"rain_month", "Pioggia mese", " mm", 1}, {"rain_year", "Pioggia anno", " mm", 1},
	{"solar", "Radiazione solare", " W/m²", 0}, {"uv", "Indice UV", "", 0},
	{"temp_in", "Temperatura interna", "°C", 1}, {"humidity_in", "Umidità interna", "%", 0},
}

func init() {
	for i := 1; i <= 8; i++ {
		stationVars = append(stationVars,
			stationVar{fmt.Sprintf("temp_ch%d", i), fmt.Sprintf("Temperatura sensore %d", i), "°C", 1},
			stationVar{fmt.Sprintf("humidity_ch%d", i), fmt.Sprintf("Umidità sensore %d", i), "%", 0})
	}
}

func stationVarInfo(key string) (stationVar, bool) {
	for _, v := range stationVars {
		if v.Key == key {
			return v, true
		}
	}
	return stationVar{}, false
}

// defaultPageFields sono i valori mostrati nella pagina se non si sceglie.
var defaultPageFields = []string{"temp", "humidity", "dewpt", "wind", "gust", "pressure", "rain_rate", "rain_day", "solar", "uv"}

// StationValue è un valore pronto da mostrare.
type StationValue struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Text  string  `json:"text"` // già formattato con unità
	Unit  string  `json:"unit"`
}

// StationReading è l'ultima lettura completa.
type StationReading struct {
	Time   time.Time          `json:"time"`
	Source string             `json:"source"`
	Values map[string]float64 `json:"values"`
	Model  string             `json:"model,omitempty"`
}

// StationPush descrive l'ultimo invio ricevuto dalla console.
type StationPush struct {
	Time     time.Time `json:"time"`
	From     string    `json:"from"`
	Path     string    `json:"path"`
	Protocol string    `json:"protocol"`
	Key      string    `json:"key"` // PASSKEY o ID della stazione
	Count    int       `json:"count"`
	Rejected string    `json:"rejected,omitempty"`
}

type stationSample struct {
	T int64              `json:"t"`
	V map[string]float64 `json:"v"`
}

type stationState struct {
	Reading StationReading  `json:"reading"`
	Day     string          `json:"day"`
	MinT    float64         `json:"min_t"`
	MaxT    float64         `json:"max_t"`
	MinAt   time.Time       `json:"min_at"`
	MaxAt   time.Time       `json:"max_at"`
	GustMax float64         `json:"gust_max"`
	Hist    []stationSample `json:"hist"` // campioni ogni 5 minuti, 7 giorni
	Days    []stationDay    `json:"days"` // riepilogo di ogni giorno, conservato
}

// stationDay riassume un giorno: tmin, tmax, hmin, hmax, rain, gust.
type stationDay struct {
	D string             `json:"d"`
	V map[string]float64 `json:"v"`
}

func (st *stationState) updateDay(day string, vals map[string]float64) {
	if n := len(st.Days); n == 0 || st.Days[n-1].D != day {
		st.Days = append(st.Days, stationDay{D: day, V: map[string]float64{}})
	}
	v := st.Days[len(st.Days)-1].V
	minmax := func(minK, maxK string, x float64) {
		if old, ok := v[minK]; !ok || x < old {
			v[minK] = x
		}
		if old, ok := v[maxK]; !ok || x > old {
			v[maxK] = x
		}
	}
	if t, ok := vals["temp"]; ok {
		minmax("tmin", "tmax", t)
	}
	if hm, ok := vals["humidity"]; ok {
		minmax("hmin", "hmax", hm)
	}
	if r, ok := vals["rain_day"]; ok && r >= v["rain"] {
		v["rain"] = r
	}
	if g, ok := vals["gust"]; ok && g > v["gust"] {
		v["gust"] = g
	}
	if len(st.Days) > 4000 {
		st.Days = st.Days[len(st.Days)-4000:]
	}
}

// StationHub riceve e conserva i dati della stazione e pubblica la pagina.
type StationHub struct {
	mu       sync.RWMutex
	path     string
	st       stationState
	push     StationPush
	readErr  string
	cfg      func() Config
	publishF func(ctx context.Context, site Site, files map[string][]byte) error
	isDue    func(siteID string, p Publication) bool
	setPub   func(siteID string, p Publication, now time.Time, err error)
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	lastSave time.Time
}

// stationHub è il ricevitore usato dai dati live di tipo "station".
var stationHub *StationHub

func NewStationHub(path string, cfg func() Config, publish func(ctx context.Context, site Site, files map[string][]byte) error) *StationHub {
	h := &StationHub{path: path, cfg: cfg, publishF: publish}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &h.st)
	}
	return h
}

// Restart (ri)avvia la lettura periodica e la pubblicazione della pagina.
func (h *StationHub) Restart(s StationSettings) {
	if h.cancel != nil {
		h.cancel()
		h.wg.Wait()
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	if !s.Enabled {
		return
	}
	if s.Source == "local" || s.Source == "wunderground" {
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			for {
				protect("stazione meteo", func() { h.pull(ctx, s) })
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(s.RefreshSeconds) * time.Second):
				}
			}
		}()
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		// attende la prima lettura prima di pubblicare
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
		for {
			protect("pagina meteo", func() {
				pctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
				defer cancel()
				h.PublishDue(pctx, false)
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()
}

func (h *StationHub) pull(ctx context.Context, s StationSettings) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	vals, model, err := ReadStation(ctx, s)
	h.mu.Lock()
	if err != nil {
		if h.readErr != err.Error() {
			log.Printf("stazione meteo: %v", err)
		}
		h.readErr = err.Error()
		h.mu.Unlock()
		return
	}
	h.readErr = ""
	h.mu.Unlock()
	h.Update(vals, s.Source, model)
}

// ReadStation legge una volta la stazione con il collegamento indicato.
func ReadStation(ctx context.Context, s StationSettings) (map[string]float64, string, error) {
	switch s.Source {
	case "local":
		if strings.TrimSpace(s.Host) == "" {
			return nil, "", errors.New("manca l'indirizzo IP della console")
		}
		return ReadEcowittLocal(ctx, strings.TrimSpace(s.Host))
	case "wunderground":
		key := s.APIKey
		if key == "" && wuGlobalKey != nil {
			key = wuGlobalKey()
		}
		vals, _, err := WUObservation(ctx, key, s.StationID)
		if err != nil {
			return nil, "", err
		}
		m := map[string]string{"temp": "temp", "humidity": "humidity", "dewpt": "dewpt", "windSpeed": "wind", "windGust": "gust",
			"winddir": "winddir", "pressure": "pressure", "precipRate": "rain_rate", "precipTotal": "rain_day", "uv": "uv", "solarRadiation": "solar"}
		out := map[string]float64{}
		for _, v := range vals {
			if k, ok := m[v.Variable]; ok {
				out[k] = v.Value
			}
		}
		return out, "Weather Underground " + s.StationID, nil
	}
	return nil, "", errors.New("con il collegamento \"server personalizzato\" è la console a inviare i dati: vedi le istruzioni")
}

// Update registra una nuova lettura (completa i valori calcolati, minime e
// massime del giorno e la serie delle ultime 24 ore).
func (h *StationHub) Update(vals map[string]float64, source, model string) {
	if len(vals) == 0 {
		return
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	// i valori che questo invio non contiene restano quelli dell'invio
	// precedente, se recente (alcune console inviano a gruppi)
	if prev := h.st.Reading; now.Sub(prev.Time) < 10*time.Minute {
		for k, v := range prev.Values {
			switch k {
			case "dewpt", "feels", "temp_min", "temp_max", "gust_max":
				continue
			}
			if _, ok := vals[k]; !ok {
				vals[k] = v
			}
		}
	}
	completeStation(vals)
	day := now.Format("2006-01-02")
	if h.st.Day != day {
		h.st.Day = day
		h.st.MinT, h.st.MaxT, h.st.GustMax = math.Inf(1), math.Inf(-1), 0
		h.st.MinAt, h.st.MaxAt = time.Time{}, time.Time{}
	}
	if t, ok := vals["temp"]; ok {
		if h.st.MinAt.IsZero() || t < h.st.MinT {
			h.st.MinT, h.st.MinAt = t, now
		}
		if h.st.MaxAt.IsZero() || t > h.st.MaxT {
			h.st.MaxT, h.st.MaxAt = t, now
		}
		vals["temp_min"], vals["temp_max"] = h.st.MinT, h.st.MaxT
	}
	if g, ok := vals["gust"]; ok && g > h.st.GustMax {
		h.st.GustMax = g
	}
	if _, ok := vals["gust_max"]; !ok {
		if _, ok := vals["gust"]; ok {
			vals["gust_max"] = h.st.GustMax
		}
	}
	h.st.updateDay(day, vals)
	h.st.Reading = StationReading{Time: now, Source: source, Values: vals, Model: model}
	// un campione ogni 5 minuti, 7 giorni
	if n := len(h.st.Hist); n == 0 || now.Unix()-h.st.Hist[n-1].T >= 300 {
		v := map[string]float64{}
		for _, k := range []string{"temp", "humidity", "pressure", "wind", "gust", "rain_day", "rain_rate", "solar"} {
			if x, ok := vals[k]; ok {
				v[k] = x
			}
		}
		h.st.Hist = append(h.st.Hist, stationSample{T: now.Unix(), V: v})
		cut := now.Add(-7 * 24 * time.Hour).Unix()
		i := 0
		for i < len(h.st.Hist) && h.st.Hist[i].T < cut {
			i++
		}
		h.st.Hist = h.st.Hist[i:]
	}
	if now.Sub(h.lastSave) > time.Minute {
		h.lastSave = now
		if b, err := json.Marshal(h.st); err == nil {
			_ = writeFileAtomic(h.path, b)
		}
	}
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Reading restituisce l'ultima lettura (copia).
func (h *StationHub) Reading() StationReading {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r := h.st.Reading
	r.Values = map[string]float64{}
	for k, v := range h.st.Reading.Values {
		r.Values[k] = v
	}
	return r
}

// Value restituisce un valore formattato, per i dati live "station".
func (h *StationHub) Value(key string) (any, error) {
	r := h.Reading()
	if r.Time.IsZero() {
		return nil, errors.New("nessun dato ricevuto dalla stazione meteo")
	}
	if age := time.Since(r.Time); age > 20*time.Minute {
		return nil, fmt.Errorf("nessun dato dalla stazione da %d minuti", int(age.Minutes()))
	}
	if key == "winddir_text" {
		d, ok := r.Values["winddir"]
		if !ok {
			return nil, errors.New("la stazione non invia la direzione del vento")
		}
		return windCardinal(d), nil
	}
	v, ok := r.Values[key]
	if !ok {
		return nil, fmt.Errorf("la stazione non invia %q", key)
	}
	return v, nil
}

// Values restituisce le grandezze attuali, in ordine, pronte da mostrare.
func (h *StationHub) Values() []StationValue {
	return stationValueList(h.Reading().Values)
}

func stationValueList(vals map[string]float64) []StationValue {
	out := []StationValue{}
	for _, v := range stationVars {
		if v.Key == "winddir_text" {
			if d, ok := vals["winddir"]; ok {
				out = append(out, StationValue{Key: v.Key, Label: v.Label, Value: d, Text: windCardinal(d)})
			}
			continue
		}
		x, ok := vals[v.Key]
		if !ok || math.IsNaN(x) || math.IsInf(x, 0) {
			continue
		}
		out = append(out, StationValue{Key: v.Key, Label: v.Label, Value: x, Unit: v.Unit, Text: formatValue(x, v.Decimals, v.Unit)})
	}
	return out
}

// Status per il pannello.
func (h *StationHub) Status() map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]any{
		"reading": h.st.Reading, "values": stationValueList(h.st.Reading.Values), "push": h.push,
		"error": h.readErr, "samples": len(h.st.Hist), "days": len(h.st.Days),
	}
}

// completeStation aggiunge punto di rugiada e temperatura percepita se mancano.
func completeStation(v map[string]float64) {
	t, okT := v["temp"]
	rh, okH := v["humidity"]
	if okT && okH && rh > 0 {
		if _, ok := v["dewpt"]; !ok {
			a, b := 17.62, 243.12
			g := math.Log(rh/100) + a*t/(b+t)
			v["dewpt"] = round1(b * g / (a - g))
		}
	}
	if _, ok := v["feels"]; !ok && okT {
		f := t
		w := v["wind"]
		switch {
		case t <= 10 && w > 4.8:
			f = 13.12 + 0.6215*t - 11.37*math.Pow(w, 0.16) + 0.3965*t*math.Pow(w, 0.16)
		case t >= 27 && okH:
			tf := t*9/5 + 32
			hi := -42.379 + 2.04901523*tf + 10.14333127*rh - 0.22475541*tf*rh - 6.83783e-3*tf*tf -
				5.481717e-2*rh*rh + 1.22874e-3*tf*tf*rh + 8.5282e-4*tf*rh*rh - 1.99e-6*tf*tf*rh*rh
			f = (hi - 32) * 5 / 9
		}
		v["feels"] = round1(f)
	}
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

var cardinals = []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSO", "SO", "OSO", "O", "ONO", "NO", "NNO"}

func windCardinal(deg float64) string {
	i := int(math.Mod(deg+11.25+360, 360) / 22.5)
	return cardinals[i%16]
}

// ---------------------------------------------------------------------------
// Ricezione dei dati inviati dalla console (server personalizzato)

// IsStationPushPath dice se l'indirizzo è uno di quelli usati dalle console.
func IsStationPushPath(p string) bool {
	p = strings.ToLower(strings.TrimRight(p, "/"))
	return p == "/data/report" || p == "/meteo" || p == "/station" ||
		strings.HasSuffix(p, "updateweatherstation.php") || strings.HasPrefix(p, "/data/report")
}

// HandlePush riceve i dati della console (protocollo Ecowitt o Wunderground).
func (h *StationHub) HandlePush(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	_ = r.ParseForm()
	form := r.Form
	vals, proto, key := ParseStationPush(form)
	from, _, _ := net.SplitHostPort(r.RemoteAddr)
	accept := ""
	if h.cfg != nil {
		accept = strings.TrimSpace(h.cfg().Station.AcceptKey)
	}
	h.mu.Lock()
	h.push = StationPush{Time: time.Now(), From: from, Path: r.URL.Path, Protocol: proto, Key: key, Count: h.push.Count + 1}
	switch {
	case len(vals) == 0:
		h.push.Rejected = "nessun valore meteo riconosciuto"
	case accept != "" && !strings.EqualFold(accept, key):
		h.push.Rejected = "stazione diversa da quella accettata"
	}
	rejected := h.push.Rejected
	h.mu.Unlock()
	// si registra anche se la stazione non è ancora attivata, così il
	// pannello mostra subito che i dati arrivano
	if rejected == "" {
		h.Update(vals, "push", strings.TrimSpace(form.Get("model")+" "+form.Get("stationtype")+form.Get("softwaretype")))
	}
	if proto == "wunderground" {
		io.WriteString(w, "success\n")
		return
	}
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "OK")
}

// ParseStationPush converte i campi inviati dalla console in unità metriche.
func ParseStationPush(f url.Values) (map[string]float64, string, string) {
	proto := "ecowitt"
	key := f.Get("PASSKEY")
	if key == "" && (f.Get("ID") != "" || f.Get("action") != "") {
		proto, key = "wunderground", f.Get("ID")
	}
	num := func(name string) (float64, bool) {
		s := strings.TrimSpace(f.Get(name))
		if s == "" {
			return 0, false
		}
		x, err := strconv.ParseFloat(s, 64)
		if err != nil || x <= -999 || math.IsNaN(x) {
			return 0, false
		}
		return x, true
	}
	out := map[string]float64{}
	setF := func(k string, names ...string) {
		for _, n := range names {
			if x, ok := num(n); ok {
				out[k] = round1((x - 32) * 5 / 9)
				return
			}
		}
	}
	setMul := func(k string, mul float64, dec int, names ...string) {
		for _, n := range names {
			if x, ok := num(n); ok {
				p := math.Pow(10, float64(dec))
				out[k] = math.Round(x*mul*p) / p
				return
			}
		}
	}
	setF("temp", "tempf")
	setF("temp_in", "tempinf", "indoortempf")
	setF("dewpt", "dewptf")
	setMul("humidity", 1, 0, "humidity")
	setMul("humidity_in", 1, 0, "humidityin", "indoorhumidity")
	setMul("wind", 1.609344, 1, "windspeedmph")
	setMul("gust", 1.609344, 1, "windgustmph")
	setMul("gust_max", 1.609344, 1, "maxdailygust")
	setMul("winddir", 1, 0, "winddir")
	setMul("pressure", 33.8639, 1, "baromrelin", "baromin")
	setMul("pressure_abs", 33.8639, 1, "baromabsin", "absbaromin")
	setMul("rain_rate", 25.4, 1, "rainratein", "rrain_piezo", "rainin")
	setMul("rain_event", 25.4, 1, "eventrainin", "erain_piezo")
	setMul("rain_hour", 25.4, 1, "hourlyrainin", "hrain_piezo")
	setMul("rain_day", 25.4, 1, "dailyrainin", "drain_piezo")
	setMul("rain_week", 25.4, 1, "weeklyrainin", "wrain_piezo")
	setMul("rain_month", 25.4, 1, "monthlyrainin", "mrain_piezo")
	setMul("rain_year", 25.4, 1, "yearlyrainin", "yrain_piezo")
	setMul("solar", 1, 0, "solarradiation")
	setMul("uv", 1, 0, "uv", "UV")
	for i := 1; i <= 8; i++ {
		setF(fmt.Sprintf("temp_ch%d", i), fmt.Sprintf("temp%df", i))
		setMul(fmt.Sprintf("humidity_ch%d", i), 1, 0, fmt.Sprintf("humidity%d", i))
	}
	return out, proto, key
}

// ---------------------------------------------------------------------------
// Lettura diretta dalla console (protocollo Ecowitt, porta 45000)

const ecowittPort = 45000

func ecowittCmd(cmd byte) []byte {
	b := []byte{0xFF, 0xFF, cmd, 3}
	return append(b, cmd+3)
}

// ReadEcowittLocal legge i dati attuali dalla console.
func ReadEcowittLocal(ctx context.Context, host string) (map[string]float64, string, error) {
	h, port := host, strconv.Itoa(ecowittPort)
	if hh, pp, err := net.SplitHostPort(host); err == nil {
		h, port = hh, pp
	}
	vals, err := ecowittTCPLive(ctx, net.JoinHostPort(h, port))
	if err == nil && len(vals) > 0 {
		return vals, "Ecowitt (porta " + port + ")", nil
	}
	hv, herr := ecowittHTTPLive(ctx, h)
	if herr == nil && len(hv) > 0 {
		return hv, "Ecowitt (pagina locale)", nil
	}
	if err == nil {
		err = errors.New("nessun valore")
	}
	return nil, "", fmt.Errorf("la console %s non risponde alla lettura diretta (%v; pagina locale: %v). Usa il collegamento \"server personalizzato\"", h, err, herr)
}

func ecowittTCPLive(ctx context.Context, addr string) (map[string]float64, error) {
	d := net.Dialer{Timeout: 4 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
	if _, err := conn.Write(ecowittCmd(0x27)); err != nil {
		return nil, err
	}
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, fmt.Errorf("nessuna risposta (%v)", err)
	}
	if hdr[0] != 0xFF || hdr[1] != 0xFF || hdr[2] != 0x27 {
		return nil, errors.New("risposta non valida")
	}
	size := int(binary.BigEndian.Uint16(hdr[3:5]))
	if size < 4 || size > 4096 {
		return nil, errors.New("lunghezza non valida")
	}
	rest := make([]byte, size-3)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return nil, err
	}
	var sum byte
	for _, b := range append(hdr[2:], rest[:len(rest)-1]...) {
		sum += b
	}
	if sum != rest[len(rest)-1] {
		return nil, errors.New("checksum errato")
	}
	return ParseEcowittLive(rest[:len(rest)-1]), nil
}

// ecowittItems: lunghezza in byte di ogni campo dei dati attuali.
func ecowittItemLen(id byte) int {
	switch {
	case id >= 0x01 && id <= 0x05, id >= 0x08 && id <= 0x11, id == 0x16, id == 0x19, id >= 0x1A && id <= 0x21, id == 0x2A:
		return 2
	case id == 0x06, id == 0x07, id == 0x17, id >= 0x22 && id <= 0x29:
		return 1
	case id >= 0x12 && id <= 0x15:
		return 4
	case id == 0x18:
		return 6
	case id >= 0x2B && id <= 0x3A:
		if (id-0x2B)%2 == 0 {
			return 2
		}
		return 1
	case id >= 0x4D && id <= 0x53:
		return 2
	case id == 0x4C:
		return 16
	}
	return 0
}

// ParseEcowittLive decodifica i campi dei dati attuali (si ferma al primo
// campo sconosciuto: i dati principali vengono sempre per primi).
func ParseEcowittLive(b []byte) map[string]float64 {
	out := map[string]float64{}
	s16 := func(p []byte) float64 { return float64(int16(binary.BigEndian.Uint16(p))) }
	u16 := func(p []byte) float64 { return float64(binary.BigEndian.Uint16(p)) }
	u32 := func(p []byte) float64 { return float64(binary.BigEndian.Uint32(p)) }
	for i := 0; i < len(b); {
		id := b[i]
		n := ecowittItemLen(id)
		if n == 0 || i+1+n > len(b) {
			break
		}
		p := b[i+1 : i+1+n]
		switch {
		case id == 0x01:
			out["temp_in"] = s16(p) / 10
		case id == 0x02:
			out["temp"] = s16(p) / 10
		case id == 0x03:
			out["dewpt"] = s16(p) / 10
		case id == 0x06:
			out["humidity_in"] = float64(p[0])
		case id == 0x07:
			out["humidity"] = float64(p[0])
		case id == 0x08:
			out["pressure_abs"] = u16(p) / 10
		case id == 0x09:
			out["pressure"] = u16(p) / 10
		case id == 0x0A:
			out["winddir"] = u16(p)
		case id == 0x0B:
			out["wind"] = round1(u16(p) / 10 * 3.6)
		case id == 0x0C:
			out["gust"] = round1(u16(p) / 10 * 3.6)
		case id == 0x0D:
			out["rain_event"] = u16(p) / 10
		case id == 0x0E:
			out["rain_rate"] = u16(p) / 10
		case id == 0x0F:
			out["rain_hour"] = u16(p) / 10
		case id == 0x10:
			out["rain_day"] = u16(p) / 10
		case id == 0x11:
			out["rain_week"] = u16(p) / 10
		case id == 0x12:
			out["rain_month"] = u32(p) / 10
		case id == 0x13:
			out["rain_year"] = u32(p) / 10
		case id == 0x15:
			out["solar"] = math.Round(u32(p) / 10 / 126.7)
		case id == 0x17:
			out["uv"] = float64(p[0])
		case id == 0x19:
			out["gust_max"] = round1(u16(p) / 10 * 3.6)
		case id >= 0x1A && id <= 0x21:
			out[fmt.Sprintf("temp_ch%d", id-0x1A+1)] = s16(p) / 10
		case id >= 0x22 && id <= 0x29:
			out[fmt.Sprintf("humidity_ch%d", id-0x22+1)] = float64(p[0])
		}
		i += 1 + n
	}
	return out
}

var numUnitRe = regexp.MustCompile(`(-?\d+(?:[.,]\d+)?)\s*([^\d\s][^\s]*)?`)

// ecowittHTTPLive legge /get_livedata_info (console GW1100, GW2000, WS3900…).
func ecowittHTTPLive(ctx context.Context, host string) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, err := getBody(ctx, "http://"+host+"/get_livedata_info", "")
	if err != nil {
		return nil, err
	}
	return ParseEcowittHTTP(body)
}

// ParseEcowittHTTP decodifica la risposta JSON di /get_livedata_info.
func ParseEcowittHTTP(body []byte) (map[string]float64, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, errors.New("risposta non in formato Ecowitt")
	}
	out := map[string]float64{}
	conv := func(val, unit string) (float64, string, bool) {
		m := numUnitRe.FindStringSubmatch(val)
		if m == nil {
			return 0, "", false
		}
		x, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		if err != nil {
			return 0, "", false
		}
		u := strings.ToLower(strings.TrimSpace(unit))
		if u == "" {
			u = strings.ToLower(m[2])
		}
		return x, u, true
	}
	toC := func(x float64, u string) float64 {
		if strings.Contains(u, "f") {
			return round1((x - 32) * 5 / 9)
		}
		return x
	}
	toKmh := func(x float64, u string) float64 {
		switch {
		case strings.Contains(u, "mph"):
			return round1(x * 1.609344)
		case strings.Contains(u, "m/s"):
			return round1(x * 3.6)
		case strings.Contains(u, "knot"):
			return round1(x * 1.852)
		}
		return x
	}
	toMM := func(x float64, u string) float64 {
		if strings.HasPrefix(u, "in") {
			return round1(x * 25.4)
		}
		return x
	}
	toHPa := func(x float64, u string) float64 {
		switch {
		case strings.Contains(u, "inhg"):
			return round1(x * 33.8639)
		case strings.Contains(u, "mmhg"):
			return round1(x * 1.33322)
		}
		return x
	}
	var common []map[string]string
	_ = json.Unmarshal(doc["common_list"], &common)
	for _, it := range common {
		x, u, ok := conv(it["val"], it["unit"])
		if !ok {
			continue
		}
		switch strings.ToLower(it["id"]) {
		case "0x02":
			out["temp"] = toC(x, u)
		case "0x03":
			out["dewpt"] = toC(x, u)
		case "3":
			out["feels"] = toC(x, u)
		case "0x07":
			out["humidity"] = x
		case "0x0a":
			out["winddir"] = x
		case "0x0b":
			out["wind"] = toKmh(x, u)
		case "0x0c":
			out["gust"] = toKmh(x, u)
		case "0x19":
			out["gust_max"] = toKmh(x, u)
		case "0x15":
			switch {
			case strings.Contains(u, "w/m"):
				out["solar"] = x
			case strings.Contains(u, "klux"):
				out["solar"] = math.Round(x * 1000 / 126.7)
			case strings.Contains(u, "lux"):
				out["solar"] = math.Round(x / 126.7)
			}
		case "0x17":
			out["uv"] = x
		}
	}
	for _, list := range []string{"rain", "piezoRain"} {
		var rain []map[string]string
		_ = json.Unmarshal(doc[list], &rain)
		for _, it := range rain {
			x, u, ok := conv(it["val"], it["unit"])
			if !ok {
				continue
			}
			k := map[string]string{"0x0d": "rain_event", "0x0e": "rain_rate", "0x10": "rain_day", "0x11": "rain_week", "0x12": "rain_month", "0x13": "rain_year"}[strings.ToLower(it["id"])]
			if _, done := out[k]; k != "" && !done {
				out[k] = toMM(x, u)
			}
		}
	}
	var wh25 []map[string]string
	_ = json.Unmarshal(doc["wh25"], &wh25)
	for _, it := range wh25 {
		if x, u, ok := conv(it["intemp"], it["unit"]); ok {
			out["temp_in"] = toC(x, u)
		}
		if x, _, ok := conv(it["inhumi"], ""); ok {
			out["humidity_in"] = x
		}
		if x, u, ok := conv(it["rel"], ""); ok {
			out["pressure"] = toHPa(x, u)
		}
		if x, u, ok := conv(it["abs"], ""); ok {
			out["pressure_abs"] = toHPa(x, u)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("nessun valore nella risposta")
	}
	return out, nil
}

// StationDevice è una console trovata nella rete.
type StationDevice struct {
	Host   string             `json:"host"`
	MAC    string             `json:"mac,omitempty"`
	Name   string             `json:"name,omitempty"`
	Values map[string]float64 `json:"values,omitempty"`
	Error  string             `json:"error,omitempty"`
}

// DiscoverStations cerca le console nella rete: annuncio UDP Ecowitt
// (porta 46000) e porta 45000 aperta.
func DiscoverStations(ctx context.Context) []StationDevice {
	found := map[string]*StationDevice{}
	var mu sync.Mutex
	add := func(d StationDevice) {
		mu.Lock()
		defer mu.Unlock()
		if old, ok := found[d.Host]; ok {
			if d.MAC != "" {
				old.MAC, old.Name = d.MAC, d.Name
			}
			return
		}
		found[d.Host] = &d
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); udpDiscover(ctx, add) }()
	go func() {
		defer wg.Done()
		hosts := make(chan string)
		var sw sync.WaitGroup
		for i := 0; i < scanWorkers; i++ {
			sw.Add(1)
			go func() {
				defer sw.Done()
				for h := range hosts {
					c, err := net.DialTimeout("tcp", net.JoinHostPort(h, strconv.Itoa(ecowittPort)), 500*time.Millisecond)
					if err == nil {
						c.Close()
						add(StationDevice{Host: h})
					}
				}
			}()
		}
		for _, h := range localHosts() {
			if ctx.Err() != nil {
				break
			}
			hosts <- h
		}
		close(hosts)
		sw.Wait()
	}()
	wg.Wait()
	out := []StationDevice{}
	for _, d := range found {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		vals, _, err := ReadEcowittLocal(rctx, d.Host)
		cancel()
		if err != nil {
			d.Error = err.Error()
		} else {
			completeStation(vals)
			d.Values = vals
		}
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func udpDiscover(ctx context.Context, add func(StationDevice)) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return
	}
	defer conn.Close()
	msg := ecowittCmd(0x12)
	targets := []string{"255.255.255.255"}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
				ip := ipn.IP.To4()
				bc := make(net.IP, 4)
				for i := range bc {
					bc[i] = ip[i] | ^ipn.Mask[len(ipn.Mask)-4+i]
				}
				targets = append(targets, bc.String())
			}
		}
	}
	for _, t := range targets {
		_, _ = conn.WriteToUDP(msg, &net.UDPAddr{IP: net.ParseIP(t), Port: 46000})
	}
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 512)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		_ = conn.SetReadDeadline(deadline)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if d, ok := parseEcowittBroadcast(buf[:n]); ok {
			add(d)
		}
	}
}

func parseEcowittBroadcast(b []byte) (StationDevice, bool) {
	if len(b) < 18 || b[0] != 0xFF || b[1] != 0xFF || b[2] != 0x12 {
		return StationDevice{}, false
	}
	d := StationDevice{
		MAC:  fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", b[5], b[6], b[7], b[8], b[9], b[10]),
		Host: net.IP(b[11:15]).String(),
	}
	if port := binary.BigEndian.Uint16(b[15:17]); port != 0 && port != ecowittPort {
		d.Host = net.JoinHostPort(d.Host, strconv.Itoa(int(port)))
	}
	if n := int(b[17]); 18+n <= len(b) {
		d.Name = strings.TrimSpace(string(b[18 : 18+n]))
	}
	return d, true
}

// localIPs elenca gli indirizzi IPv4 di questo computer nella rete locale.
func localIPs() []string {
	out := []string{}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip := ipn.IP.To4(); ip != nil && !ip.IsLinkLocalUnicast() {
					out = append(out, ip.String())
				}
			}
		}
	}
	return out
}
