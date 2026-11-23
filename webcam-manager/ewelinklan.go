package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Lettura in rete locale (modalità LAN di eWeLink): i dispositivi Sonoff con
// firmware originale annunciano il loro stato via mDNS (_ewelink._tcp) con
// i parametri cifrati (AES-128-CBC, chiave = MD5 della "devicekey" del
// dispositivo). I bridge Zigbee che lo supportano inviano così anche i
// valori dei sensori collegati (campo subDevId). La devicekey si prende una
// volta dal cloud e si conserva, poi basta la rete locale.

type lanReading struct {
	Params map[string]any
	At     time.Time
	From   string // dispositivo (bridge) che l'ha trasmessa
}

type ewelinkLAN struct {
	mu       sync.Mutex
	started  bool
	keys     map[string]string // id dispositivo -> devicekey
	uiid     map[string]int
	values   map[string]lanReading // id dispositivo/sensore -> ultimi parametri
	seen     map[string]time.Time  // dispositivi visti in LAN (anche senza chiave)
	keysPath string
}

var ewLAN = &ewelinkLAN{keys: map[string]string{}, uiid: map[string]int{}, values: map[string]lanReading{}, seen: map[string]time.Time{}}

// setKeysPath imposta il file dove conservare le chiavi (impostato da App).
func (l *ewelinkLAN) setKeysPath(p string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keysPath = p
	if b, err := os.ReadFile(p); err == nil {
		var saved struct {
			Keys map[string]string `json:"keys"`
			UIID map[string]int    `json:"uiid"`
		}
		if json.Unmarshal(b, &saved) == nil {
			for k, v := range saved.Keys {
				l.keys[k] = v
			}
			for k, v := range saved.UIID {
				l.uiid[k] = v
			}
		}
	}
}

// learn memorizza chiavi e tipi dei dispositivi letti dal cloud.
func (l *ewelinkLAN) learn(things []ewelinkThing) {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for _, t := range things {
		if t.DeviceKey != "" && l.keys[t.ID] != t.DeviceKey {
			l.keys[t.ID], changed = t.DeviceKey, true
		}
		if t.UIID != 0 && l.uiid[t.ID] != t.UIID {
			l.uiid[t.ID], changed = t.UIID, true
		}
	}
	if changed && l.keysPath != "" {
		b, _ := json.Marshal(map[string]any{"keys": l.keys, "uiid": l.uiid})
		_ = os.WriteFile(l.keysPath, b, 0o600)
	}
}

// value restituisce temperatura/umidità ricevute in LAN negli ultimi maxAge.
func (l *ewelinkLAN) value(id, variable string, maxAge time.Duration) (float64, bool) {
	l.mu.Lock()
	r, ok := l.values[id]
	uiid := l.uiid[id]
	l.mu.Unlock()
	if !ok || time.Since(r.At) > maxAge {
		return 0, false
	}
	return ewelinkValue(ewelinkThing{ID: id, UIID: uiid, Params: r.Params}, variable)
}

// status descrive cosa si vede in LAN (per il pannello).
func (l *ewelinkLAN) status() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]string{}
	for id, t := range l.seen {
		seen[id] = t.Format(time.RFC3339)
	}
	vals := map[string]string{}
	for id, r := range l.values {
		vals[id] = r.At.Format(time.RFC3339)
	}
	return map[string]any{"listening": l.started, "devices": seen, "values": vals, "keys": len(l.keys)}
}

// start avvia l'ascolto mDNS (una volta sola).
func (l *ewelinkLAN) start() {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return
	}
	l.started = true
	l.mu.Unlock()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		log.Printf("eWeLink LAN: ascolto mDNS non disponibile: %v", err)
		return
	}
	log.Printf("eWeLink LAN: in ascolto dei dispositivi Sonoff nella rete locale")
	go func() {
		defer conn.Close()
		buf := make([]byte, 9000)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			protect("eWeLink LAN", func() { l.handle(buf[:n], from) })
		}
	}()
	go func() {
		q := mdnsQuery("_ewelink._tcp.local.")
		for {
			if out, err := net.DialUDP("udp4", nil, group); err == nil {
				_, _ = out.Write(q)
				out.Close()
			}
			_, _ = conn.WriteToUDP(q, group)
			time.Sleep(60 * time.Second)
		}
	}()
}

func mdnsQuery(service string) []byte {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	_ = b.StartQuestions()
	_ = b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(service), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET})
	msg, _ := b.Finish()
	return msg
}

// handle legge i record TXT eWeLink di un pacchetto mDNS.
func (l *ewelinkLAN) handle(pkt []byte, from *net.UDPAddr) {
	var p dnsmessage.Parser
	if _, err := p.Start(pkt); err != nil {
		return
	}
	_ = p.SkipAllQuestions()
	var txts [][]string
	collect := func(next func() (dnsmessage.ResourceHeader, error), body func() (dnsmessage.TXTResource, error), skip func() error) {
		for {
			h, err := next()
			if err != nil {
				return
			}
			if h.Type == dnsmessage.TypeTXT && strings.Contains(strings.ToLower(h.Name.String()), "_ewelink") {
				if r, err := body(); err == nil {
					txts = append(txts, r.TXT)
				}
				continue
			}
			if skip() != nil {
				return
			}
		}
	}
	collect(p.AnswerHeader, p.TXTResource, p.SkipAnswer)
	collect(p.AuthorityHeader, p.TXTResource, p.SkipAuthority)
	collect(p.AdditionalHeader, p.TXTResource, p.SkipAdditional)
	for _, txt := range txts {
		l.handleTXT(txt, from)
	}
}

func (l *ewelinkLAN) handleTXT(txt []string, from *net.UDPAddr) {
	kv := map[string]string{}
	for _, t := range txt {
		if k, v, ok := strings.Cut(t, "="); ok {
			kv[k] = v
		}
	}
	id := kv["id"]
	if id == "" {
		return
	}
	data := kv["data1"] + kv["data2"] + kv["data3"] + kv["data4"]
	l.mu.Lock()
	l.seen[id] = time.Now()
	key := l.keys[id]
	l.mu.Unlock()
	var params map[string]any
	if kv["encrypt"] == "true" {
		if key == "" {
			return // serve la chiave dal cloud
		}
		plain, err := ewelinkDecrypt(key, kv["iv"], data)
		if err != nil {
			return
		}
		if json.Unmarshal(plain, &params) != nil {
			return
		}
	} else if json.Unmarshal([]byte(data), &params) != nil {
		return
	}
	target := id
	if sub, ok := params["subDevId"].(string); ok && sub != "" {
		target = sub // valore di un sensore collegato al bridge
	}
	src := ""
	if from != nil {
		src = from.IP.String()
	}
	l.mu.Lock()
	old := l.values[target]
	merged := map[string]any{}
	for k, v := range old.Params {
		merged[k] = v
	}
	for k, v := range params {
		merged[k] = v
	}
	if _, ok := l.values[target]; !ok {
		log.Printf("eWeLink LAN: ricevuti dati di %s da %s", target, src)
	}
	l.values[target] = lanReading{Params: merged, At: time.Now(), From: src}
	l.mu.Unlock()
}

// ewelinkDecrypt decifra i dati LAN (AES-128-CBC, chiave MD5(devicekey)).
func ewelinkDecrypt(devicekey, ivB64, dataB64 string) ([]byte, error) {
	sum := md5.Sum([]byte(devicekey))
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil || len(iv) != aes.BlockSize {
		return nil, errors.New("iv non valido")
	}
	ct, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil || len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("dati non validi")
	}
	block, _ := aes.NewCipher(sum[:])
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	pad := int(out[len(out)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(out) {
		return nil, errors.New("chiave sbagliata")
	}
	return out[:len(out)-pad], nil
}
