package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sensore Ecowitt WS90 "Powered by Shelly" letto direttamente via Bluetooth.
//
// Questa versione del WS90 non usa la radio Ecowitt a 868 MHz ma trasmette
// in Bluetooth Low Energy nel formato aperto BTHome v2 (service data UUID
// 0xFCD2), non cifrato, un pacchetto ogni ~8 secondi alternando due tipi:
// temperatura/umidità/pressione/pioggia e vento/luce/UV. Non serve nessun
// abbinamento di Windows: basta ascoltare le trasmissioni.
//
// La pioggia arriva come contatore cumulativo (mm): pioggia oraria,
// giornaliera, ecc. si calcolano qui dalle differenze e si salvano su disco.

const ws90SourceModel = "Ecowitt WS90 (Bluetooth)"

// ---------------------------------------------------------------------------
// Decodifica BTHome v2

type bthomeObj struct {
	key    string
	size   int
	signed bool
	factor float64
}

var bthomeObjs = map[byte]bthomeObj{
	0x00: {"packet_id", 1, false, 1},
	0x01: {"battery", 1, false, 1},
	0x02: {"temp", 2, true, 0.01},
	0x03: {"humidity", 2, false, 0.01},
	0x04: {"pressure", 3, false, 0.01},
	0x05: {"lux", 3, false, 0.01},
	0x08: {"dewpt", 2, true, 0.01},
	0x0C: {"voltage", 2, false, 0.001},
	0x20: {"raining", 1, false, 1},
	0x2E: {"humidity", 1, false, 1},
	0x44: {"speed", 2, false, 0.01},
	0x45: {"temp", 2, true, 0.1},
	0x46: {"uv", 1, false, 0.1},
	0x57: {"temp", 1, true, 1},
	0x5E: {"winddir", 2, false, 0.01},
	0x5F: {"rain_total", 2, false, 0.1},
}

// dimensioni di altri oggetti BTHome, solo per poterli saltare
var bthomeSizes = map[byte]int{
	0x06: 2, 0x07: 2, 0x09: 1, 0x0A: 3, 0x0B: 3, 0x0D: 2, 0x0E: 2, 0x12: 2,
	0x13: 2, 0x14: 2, 0x3A: 1, 0x3C: 2, 0x3D: 2, 0x3E: 4, 0x3F: 2, 0x40: 2,
	0x41: 2, 0x42: 3, 0x43: 2, 0x47: 2, 0x48: 2, 0x49: 2, 0x4A: 2, 0x4B: 3,
	0x4C: 4, 0x4D: 4, 0x4E: 4, 0x4F: 4, 0x50: 4, 0x51: 2, 0x52: 2, 0x55: 4,
	0x56: 2, 0x58: 1, 0x59: 1, 0x5A: 2, 0x5B: 4, 0x5C: 4, 0x5D: 2, 0x60: 1,
	0x61: 2, 0xF0: 2, 0xF1: 4, 0xF2: 3,
}

func init() {
	for i := byte(0x0F); i < 0x2E; i++ {
		if _, ok := bthomeSizes[i]; !ok {
			bthomeSizes[i] = 1
		}
	}
}

// BTHomePacket è un pacchetto decodificato. Speeds contiene le velocità nell'ordine
// di arrivo (il WS90 ne manda due: vento medio e raffica).
type BTHomePacket struct {
	Values map[string]float64
	Speeds []float64
}

var errBTHomeEncrypted = errors.New("dati BTHome cifrati")

// ParseBTHome decodifica il service data BTHome v2 (non cifrato).
func ParseBTHome(b []byte) (BTHomePacket, error) {
	p := BTHomePacket{Values: map[string]float64{}}
	if len(b) == 0 {
		return p, errors.New("pacchetto vuoto")
	}
	if b[0]&0x01 != 0 {
		return p, errBTHomeEncrypted
	}
	if b[0]>>5 != 2 {
		return p, fmt.Errorf("versione BTHome %d non supportata", b[0]>>5)
	}
	for i := 1; i < len(b); {
		id := b[i]
		i++
		if o, ok := bthomeObjs[id]; ok {
			if i+o.size > len(b) {
				return p, errors.New("pacchetto troncato")
			}
			var raw int64
			for j := o.size - 1; j >= 0; j-- {
				raw = raw<<8 | int64(b[i+j])
			}
			if o.signed && raw&(1<<(8*o.size-1)) != 0 {
				raw -= 1 << (8 * o.size)
			}
			i += o.size
			v := float64(raw) * o.factor
			if o.key == "speed" {
				p.Speeds = append(p.Speeds, v)
			} else {
				p.Values[o.key] = v
			}
			continue
		}
		if n, ok := bthomeSizes[id]; ok {
			i += n
			continue
		}
		if (id == 0x53 || id == 0x54) && i < len(b) {
			i += 1 + int(b[i])
			continue
		}
		break // oggetto sconosciuto: si tiene quanto letto
	}
	return p, nil
}

// ws90StationValues converte un pacchetto nelle grandezze della stazione
// (unità metriche del programma). La pioggia si aggiunge a parte.
func ws90StationValues(p BTHomePacket) map[string]float64 {
	out := map[string]float64{}
	v := p.Values
	if x, ok := v["temp"]; ok {
		out["temp"] = round1(x)
	}
	if x, ok := v["humidity"]; ok {
		out["humidity"] = math.Round(x)
	}
	if x, ok := v["dewpt"]; ok {
		out["dewpt"] = round1(x)
	}
	if x, ok := v["pressure"]; ok {
		out["pressure_abs"] = round1(x)
		out["pressure"] = round1(x)
	}
	if x, ok := v["winddir"]; ok {
		out["winddir"] = math.Mod(math.Round(x), 360)
	}
	if len(p.Speeds) > 0 {
		// vento medio e raffica: il medio non può superare la raffica
		lo, hi := p.Speeds[0], p.Speeds[0]
		for _, s := range p.Speeds[1:] {
			lo, hi = math.Min(lo, s), math.Max(hi, s)
		}
		out["wind"] = round1(lo * 3.6)
		out["gust"] = round1(hi * 3.6)
	}
	if x, ok := v["lux"]; ok {
		out["solar"] = math.Round(x / 126.7) // stessa conversione delle console Ecowitt
	}
	if x, ok := v["uv"]; ok {
		out["uv"] = math.Round(x)
	}
	return out
}

// ---------------------------------------------------------------------------
// Pioggia: dal contatore cumulativo ai totali per periodo

type ws90Rain struct {
	Address   string               `json:"address"`
	Raw       *float64             `json:"raw"`        // ultimo contatore letto (mm)
	Total     float64              `json:"total"`      // totale accumulato da noi (mm)
	Starts    map[string]rainStart `json:"starts"`     // periodo -> inizio
	EventFrom *float64             `json:"event_from"` // totale all'inizio dell'evento
	LastRain  time.Time            `json:"last_rain"`
	Incr      []rainIncr           `json:"incr"` // aumenti delle ultime 24 ore
}

type rainStart struct {
	Key   string  `json:"key"`
	Total float64 `json:"total"`
}

type rainIncr struct {
	T  int64   `json:"t"`
	MM float64 `json:"mm"`
}

func rainPeriodKeys(t time.Time) map[string]string {
	y, w := t.ISOWeek()
	return map[string]string{
		"day":   t.Format("2006-01-02"),
		"week":  fmt.Sprintf("%d-W%02d", y, w),
		"month": t.Format("2006-01"),
		"year":  t.Format("2006"),
	}
}

func (r *ws90Rain) periods(now time.Time) {
	if r.Starts == nil {
		r.Starts = map[string]rainStart{}
	}
	for p, k := range rainPeriodKeys(now) {
		if s, ok := r.Starts[p]; !ok || s.Key != k {
			r.Starts[p] = rainStart{Key: k, Total: r.Total}
		}
	}
}

// add registra un nuovo valore del contatore; restituisce true se lo stato è cambiato.
func (r *ws90Rain) add(address string, raw float64, now time.Time) bool {
	raw = math.Round(raw*10) / 10
	if !strings.EqualFold(r.Address, address) {
		r.Address, r.Raw = address, nil // sensore diverso: si riparte dal suo contatore
	}
	r.periods(now)
	inc := 0.0
	switch {
	case r.Raw == nil:
	case raw >= *r.Raw:
		inc = raw - *r.Raw
	case *r.Raw > 6000 && raw < 500:
		inc = raw + 6553.6 - *r.Raw // contatore a 16 bit ripartito da zero
	default:
		inc = raw // sensore riavviato: contatore ripartito da zero
	}
	if inc > 100 {
		log.Printf("WS90: salto di pioggia anomalo (%.1f mm) ignorato", inc)
		inc = 0
	}
	changed := r.Raw == nil || *r.Raw != raw
	r.Raw = &raw
	if inc > 0 {
		if r.EventFrom == nil || now.Sub(r.LastRain) > 24*time.Hour {
			t := r.Total
			r.EventFrom = &t
		}
		r.Total = math.Round((r.Total+inc)*100) / 100
		r.LastRain = now
		r.Incr = append(r.Incr, rainIncr{now.Unix(), math.Round(inc*100) / 100})
	}
	cut := now.Add(-24 * time.Hour).Unix()
	i := 0
	for i < len(r.Incr) && r.Incr[i].T < cut {
		i++
	}
	r.Incr = r.Incr[i:]
	return changed || inc > 0
}

// values restituisce le grandezze di pioggia della stazione (mm, mm/h).
func (r *ws90Rain) values(now time.Time) map[string]float64 {
	r.periods(now)
	last := func(d time.Duration) float64 {
		s, cut := 0.0, now.Add(-d).Unix()
		for _, x := range r.Incr {
			if x.T >= cut {
				s += x.MM
			}
		}
		return s
	}
	since := func(p string) float64 { return math.Max(0, r.Total-r.Starts[p].Total) }
	event := 0.0
	if r.EventFrom != nil && now.Sub(r.LastRain) <= 24*time.Hour {
		event = math.Max(0, r.Total-*r.EventFrom)
	}
	return map[string]float64{
		"rain_rate":  round1(last(10*time.Minute) * 6),
		"rain_event": round1(event),
		"rain_hour":  round1(last(time.Hour)),
		"rain_day":   round1(since("day")),
		"rain_week":  round1(since("week")),
		"rain_month": round1(since("month")),
		"rain_year":  round1(since("year")),
	}
}

// ---------------------------------------------------------------------------
// Ricevitore Bluetooth

// bleAdv è una trasmissione Bluetooth ricevuta (dal backend della piattaforma).
type bleAdv struct {
	Address string
	Name    string
	RSSI    int
	BTHome  []byte // service data 0xFCD2, nil se assente
}

// WS90Device è un sensore BTHome visto nelle vicinanze.
type WS90Device struct {
	Address   string             `json:"address"`
	Name      string             `json:"name"`
	RSSI      int                `json:"rssi"`
	LastSeen  time.Time          `json:"last_seen"`
	Packets   int                `json:"packets"`
	Encrypted bool               `json:"encrypted"`
	Battery   float64            `json:"battery"`
	Values    map[string]float64 `json:"-"`
	lastPID   float64
	hasPID    bool
}

// WS90Hub ascolta il Bluetooth e passa i dati del sensore associato alla stazione meteo.
type WS90Hub struct {
	mu        sync.Mutex
	rainPath  string
	rain      ws90Rain
	rainSaved time.Time
	update    func(vals map[string]float64, source, model string)
	altitude  func() int // metri s.l.m., per la pressione relativa
	devices   map[string]*WS90Device
	target    string    // indirizzo del sensore associato ("" = nessuno)
	needUntil time.Time // ricerca in corso fino a…
	cancel    context.CancelFunc
	running   bool
	started   time.Time
	scanErr   string
	advCount  int       // trasmissioni Bluetooth ricevute (di qualsiasi dispositivo)
	lastAdv   time.Time // ultima trasmissione di qualsiasi dispositivo
	lastOwn   time.Time // ultimo pacchetto del sensore associato
}

// ws90Hub è il ricevitore usato dalla stazione meteo con collegamento "bluetooth".
var ws90Hub *WS90Hub

// bleScanFunc ascolta le trasmissioni Bluetooth finché ctx non è annullato
// (implementato per piattaforma; sostituibile nei test).
var bleScanFunc = bleScan

func NewWS90Hub(rainPath string, update func(vals map[string]float64, source, model string)) *WS90Hub {
	h := &WS90Hub{rainPath: rainPath, update: update, devices: map[string]*WS90Device{}}
	if b, err := os.ReadFile(rainPath); err == nil {
		_ = json.Unmarshal(b, &h.rain)
	}
	return h
}

// seaLevelPressure riporta la pressione assoluta al livello del mare (formula barometrica).
func seaLevelPressure(abs float64, altitude int) float64 {
	if altitude <= 0 {
		return abs
	}
	return round1(abs * math.Pow(1-0.0065*float64(altitude)/288.15, -5.255))
}

func normBLEAddr(a string) string { return strings.ToUpper(strings.TrimSpace(a)) }

// Follow imposta il sensore associato ("" = nessuno) e avvia o ferma l'ascolto.
func (h *WS90Hub) Follow(address string) {
	h.mu.Lock()
	h.target = normBLEAddr(address)
	h.mu.Unlock()
	h.ensure()
}

// ensure avvia l'ascolto se serve; si ferma da solo quando non serve più.
func (h *WS90Hub) ensure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running || !h.neededLocked(time.Now()) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel, h.running, h.started = cancel, true, time.Now()
	go h.loop(ctx)
}

func (h *WS90Hub) neededLocked(now time.Time) bool {
	return h.target != "" || now.Before(h.needUntil)
}

func (h *WS90Hub) loop(ctx context.Context) {
	defer func() {
		h.mu.Lock()
		h.running, h.cancel = false, nil
		h.mu.Unlock()
	}()
	for {
		scanCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			var err error
			protect("Bluetooth WS90", func() { err = bleScanFunc(scanCtx, h.handle) })
			done <- err
		}()
		restart := false
		tick := time.NewTicker(5 * time.Second)
	watch:
		for {
			select {
			case err := <-done:
				h.mu.Lock()
				msg := "l'ascolto Bluetooth si è interrotto"
				if err != nil {
					msg = "Bluetooth non disponibile: " + err.Error()
				}
				if h.scanErr != msg {
					log.Printf("WS90: %s", msg)
				}
				h.scanErr = msg
				h.mu.Unlock()
				restart = true
				break watch
			case <-tick.C:
				h.mu.Lock()
				now := time.Now()
				needed := h.neededLocked(now)
				// se il sensore associato tace da 5 minuti si riavvia l'ascolto
				stale := h.target != "" && now.Sub(maxTime(h.lastOwn, h.started)) > 5*time.Minute
				if stale {
					h.started = now
				}
				h.mu.Unlock()
				if !needed {
					stop()
					<-done
					tick.Stop()
					return
				}
				if stale {
					log.Printf("WS90: nessun dato dal sensore da 5 minuti, riavvio l'ascolto Bluetooth")
					stop()
					<-done
					break watch
				}
			}
		}
		tick.Stop()
		stop()
		wait := time.Second
		if restart {
			wait = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		h.mu.Lock()
		if !h.neededLocked(time.Now()) {
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// handle riceve ogni trasmissione Bluetooth.
func (h *WS90Hub) handle(a bleAdv) {
	now := time.Now()
	addr := normBLEAddr(a.Address)
	h.mu.Lock()
	h.advCount++
	h.lastAdv = now
	if h.scanErr != "" {
		h.scanErr = ""
	}
	isWS90 := strings.HasPrefix(strings.ToUpper(a.Name), "SBWS-90")
	if a.BTHome == nil && !isWS90 {
		h.mu.Unlock()
		return
	}
	d := h.devices[addr]
	if d == nil {
		if len(h.devices) > 200 { // tanti dispositivi BTHome: si tengono i recenti
			h.pruneLocked(now)
		}
		d = &WS90Device{Address: addr}
		h.devices[addr] = d
	}
	if a.Name != "" {
		d.Name = a.Name
	}
	d.RSSI, d.LastSeen = a.RSSI, now
	if a.BTHome == nil {
		h.mu.Unlock()
		return
	}
	pkt, err := ParseBTHome(a.BTHome)
	if err != nil {
		d.Encrypted = errors.Is(err, errBTHomeEncrypted)
		h.mu.Unlock()
		return
	}
	d.Encrypted = false
	if pid, ok := pkt.Values["packet_id"]; ok {
		if d.hasPID && pid == d.lastPID {
			h.mu.Unlock()
			return // stesso pacchetto ripetuto
		}
		d.lastPID, d.hasPID = pid, true
	}
	d.Packets++
	if b, ok := pkt.Values["battery"]; ok {
		d.Battery = b
	}
	vals := ws90StationValues(pkt)
	if p, ok := vals["pressure_abs"]; ok && h.altitude != nil {
		vals["pressure"] = seaLevelPressure(p, h.altitude())
	}
	if d.Values == nil {
		d.Values = map[string]float64{}
	}
	for k, v := range vals {
		d.Values[k] = v
	}
	if addr != h.target || h.target == "" {
		h.mu.Unlock()
		return
	}
	h.lastOwn = now
	saveRain := false
	if raw, ok := pkt.Values["rain_total"]; ok {
		saveRain = h.rain.add(addr, raw, now)
	}
	for k, v := range h.rain.values(now) {
		vals[k] = v
	}
	var rainJSON []byte
	if saveRain || now.Sub(h.rainSaved) > 10*time.Minute {
		h.rainSaved = now
		rainJSON, _ = json.Marshal(h.rain)
	}
	update := h.update
	h.mu.Unlock()
	if rainJSON != nil {
		if err := writeFileAtomic(h.rainPath, rainJSON); err != nil {
			log.Printf("WS90: pioggia non salvata: %v", err)
		}
	}
	if update != nil {
		update(vals, "bluetooth", ws90SourceModel)
	}
}

func (h *WS90Hub) pruneLocked(now time.Time) {
	for k, d := range h.devices {
		if now.Sub(d.LastSeen) > 10*time.Minute && k != h.target {
			delete(h.devices, k)
		}
	}
}

// Discover ascolta per la durata indicata e restituisce i sensori BTHome
// vicini, dal segnale più forte.
func (h *WS90Hub) Discover(ctx context.Context, d time.Duration) ([]WS90Device, string) {
	h.mu.Lock()
	h.needUntil = time.Now().Add(d + 30*time.Second)
	h.mu.Unlock()
	h.ensure()
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cut := time.Now().Add(-2 * time.Minute)
	out := []WS90Device{}
	for _, x := range h.devices {
		if x.LastSeen.After(cut) {
			c := *x
			c.Values = map[string]float64{}
			for k, v := range x.Values {
				c.Values[k] = v
			}
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		wi, wj := strings.HasPrefix(out[i].Name, "SBWS-90"), strings.HasPrefix(out[j].Name, "SBWS-90")
		if wi != wj {
			return wi
		}
		return out[i].RSSI > out[j].RSSI
	})
	msg := h.scanErr
	if msg == "" && h.advCount == 0 {
		msg = "il Bluetooth di questo PC non ha ricevuto nessuna trasmissione"
	}
	return out, msg
}

// Status per il pannello.
func (h *WS90Hub) Status() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := map[string]any{
		"target": h.target, "running": h.running, "error": h.scanErr,
		"adv_count": h.advCount, "last_adv": h.lastAdv, "last_packet": h.lastOwn,
	}
	if d := h.devices[h.target]; d != nil && h.target != "" {
		st["device"] = d
	}
	if h.running && h.advCount == 0 && time.Since(h.started) > 2*time.Minute {
		st["hint"] = "no_adv"
	}
	return st
}

// Values restituisce gli ultimi valori del sensore associato (per "Prova lettura").
func (h *WS90Hub) Values(address string) (map[string]float64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := h.devices[normBLEAddr(address)]
	if d == nil || len(d.Values) == 0 || time.Since(d.LastSeen) > 5*time.Minute {
		return nil, errors.New("nessun dato recente dal WS90 via Bluetooth")
	}
	out := map[string]float64{}
	for k, v := range d.Values {
		out[k] = v
	}
	for k, v := range h.rain.values(time.Now()) {
		out[k] = v
	}
	return out, nil
}
