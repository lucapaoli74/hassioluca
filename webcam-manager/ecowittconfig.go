package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Configurazione della console Ecowitt (e cloni: Froggit, Sainlogic…) dalla
// rete, con gli stessi comandi dell'app WS View (porta 45000): così si
// imposta il "server personalizzato" anche senza smartphone.

const (
	cmdReadFirmware   = 0x50
	cmdReadCustomized = 0x2A
	cmdWriteCustom    = 0x2B
	cmdReadUserPath   = 0x51
	cmdWriteUserPath  = 0x52
)

// EcowittCustom è l'impostazione "Customized" della console.
type EcowittCustom struct {
	Station     string `json:"station"`
	Key         string `json:"key"`
	Server      string `json:"server"`
	Port        int    `json:"port"`
	Interval    int    `json:"interval"`
	Protocol    string `json:"protocol"` // "ecowitt" | "wunderground"
	Enabled     bool   `json:"enabled"`
	PathEcowitt string `json:"path_ecowitt"`
	PathWU      string `json:"path_wu"`
}

func consoleAddr(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(ecowittPort))
}

// ecowittRequest invia un comando e restituisce i dati della risposta
// (senza intestazione e checksum). Risposte con lunghezza su 1 byte.
func ecowittRequest(ctx context.Context, host string, cmd byte, payload []byte) ([]byte, error) {
	d := net.Dialer{Timeout: 4 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", consoleAddr(host))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
	body := append([]byte{cmd, byte(len(payload) + 3)}, payload...)
	var sum byte
	for _, b := range body {
		sum += b
	}
	pkt := append(append([]byte{0xFF, 0xFF}, body...), sum)
	if _, err := conn.Write(pkt); err != nil {
		return nil, err
	}
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, fmt.Errorf("nessuna risposta (%v)", err)
	}
	if hdr[0] != 0xFF || hdr[1] != 0xFF || hdr[2] != cmd {
		return nil, fmt.Errorf("risposta inattesa %X", hdr)
	}
	size := int(hdr[3])
	if size < 3 {
		return nil, errors.New("lunghezza non valida")
	}
	rest := make([]byte, size-2)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return nil, err
	}
	sum = hdr[2] + hdr[3]
	for _, b := range rest[:len(rest)-1] {
		sum += b
	}
	if sum != rest[len(rest)-1] {
		return nil, errors.New("checksum errato")
	}
	return rest[:len(rest)-1], nil
}

type byteReader struct {
	b   []byte
	i   int
	err error
}

func (r *byteReader) str() string {
	if r.err != nil || r.i >= len(r.b) {
		r.err = errors.New("dati troppo corti")
		return ""
	}
	n := int(r.b[r.i])
	r.i++
	if r.i+n > len(r.b) {
		r.err = errors.New("dati troppo corti")
		return ""
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s
}

func (r *byteReader) u8() int {
	if r.err != nil || r.i >= len(r.b) {
		r.err = errors.New("dati troppo corti")
		return 0
	}
	r.i++
	return int(r.b[r.i-1])
}

func (r *byteReader) u16() int {
	if r.err != nil || r.i+2 > len(r.b) {
		r.err = errors.New("dati troppo corti")
		return 0
	}
	r.i += 2
	return int(binary.BigEndian.Uint16(r.b[r.i-2:]))
}

func (r *byteReader) done() error {
	if r.err == nil && r.i != len(r.b) {
		r.err = fmt.Errorf("%d byte in più", len(r.b)-r.i)
	}
	return r.err
}

// ParseEcowittCustom decodifica le risposte 0x2A e 0x51 (formato verificato:
// ogni byte deve tornare, altrimenti la console usa un formato diverso e non
// si scrive nulla).
func ParseEcowittCustom(custom, path []byte) (EcowittCustom, error) {
	r := &byteReader{b: custom}
	c := EcowittCustom{Station: r.str(), Key: r.str(), Server: r.str(), Port: r.u16(), Interval: r.u16()}
	if r.u8() == 0 {
		c.Protocol = "ecowitt"
	} else {
		c.Protocol = "wunderground"
	}
	c.Enabled = r.u8() == 1
	if err := r.done(); err != nil {
		return c, fmt.Errorf("formato delle impostazioni non riconosciuto (%v): %s", err, hex.EncodeToString(custom))
	}
	p := &byteReader{b: path}
	c.PathEcowitt, c.PathWU = p.str(), p.str()
	if err := p.done(); err != nil {
		return c, fmt.Errorf("formato del percorso non riconosciuto (%v): %s", err, hex.EncodeToString(path))
	}
	return c, nil
}

func packStr(b []byte, s string) []byte { return append(append(b, byte(len(s))), s...) }

func packEcowittCustom(c EcowittCustom) ([]byte, []byte) {
	var b []byte
	b = packStr(b, c.Station)
	b = packStr(b, c.Key)
	b = packStr(b, c.Server)
	b = binary.BigEndian.AppendUint16(b, uint16(c.Port))
	b = binary.BigEndian.AppendUint16(b, uint16(c.Interval))
	proto := byte(0)
	if c.Protocol == "wunderground" {
		proto = 1
	}
	en := byte(0)
	if c.Enabled {
		en = 1
	}
	b = append(b, proto, en)
	return b, packStr(packStr(nil, c.PathEcowitt), c.PathWU)
}

// ConsoleInfo è quello che si sa della console.
type ConsoleInfo struct {
	Host     string         `json:"host"`
	API      bool           `json:"api"` // risponde sulla porta 45000
	APIError string         `json:"api_error,omitempty"`
	Firmware string         `json:"firmware,omitempty"`
	Custom   *EcowittCustom `json:"custom,omitempty"`
	WebUI    string         `json:"web_ui,omitempty"` // indirizzo della pagina di configurazione
	Title    string         `json:"title,omitempty"`  // titolo della pagina web del dispositivo
	Live     []StationValue `json:"live,omitempty"`   // dati letti direttamente (pagina locale)
	LiveFrom string         `json:"live_from,omitempty"`
}

// ReadEcowittConsole legge firmware e impostazioni del server personalizzato.
func ReadEcowittConsole(ctx context.Context, host string) ConsoleInfo {
	info := ConsoleInfo{Host: host}
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	// pagina web di configurazione (console "PRO")
	hctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	if req, err := http.NewRequestWithContext(hctx, "GET", "http://"+h+"/", nil); err == nil {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				info.WebUI = "http://" + h + "/"
			}
		}
	}
	cancel()
	if info.WebUI != "" {
		info.Title = pageTitle(ctx, h)
		if v, err := readObserverLive(ctx, h); err == nil {
			completeStation(v)
			info.Live, info.LiveFrom = stationValueList(v), "http://"+h+"/livedata.htm"
		} else if v, err := ecowittHTTPLive(ctx, h); err == nil {
			completeStation(v)
			info.Live, info.LiveFrom = stationValueList(v), "http://"+h+"/get_livedata_info"
		}
	}
	if fw, err := ecowittRequest(ctx, host, cmdReadFirmware, nil); err == nil {
		info.API = true
		if len(fw) > 1 {
			info.Firmware = strings.TrimSpace(string(fw[1:]))
		}
	} else {
		info.APIError = err.Error()
		return info
	}
	custom, err := ecowittRequest(ctx, host, cmdReadCustomized, nil)
	if err != nil {
		info.APIError = err.Error()
		return info
	}
	path, err := ecowittRequest(ctx, host, cmdReadUserPath, nil)
	if err != nil {
		info.APIError = err.Error()
		return info
	}
	c, err := ParseEcowittCustom(custom, path)
	if err != nil {
		info.APIError = err.Error()
		return info
	}
	info.Custom = &c
	return info
}

// ConfigureEcowittConsole imposta il server personalizzato verso questo PC
// (protocollo Ecowitt, /data/report/) e verifica rileggendo.
func ConfigureEcowittConsole(ctx context.Context, host, server string, port, interval int) (EcowittCustom, error) {
	info := ReadEcowittConsole(ctx, host)
	if info.Custom == nil {
		if info.APIError == "" {
			info.APIError = "nessuna risposta"
		}
		return EcowittCustom{}, fmt.Errorf("la console non si lascia configurare dalla rete: %s", info.APIError)
	}
	c := *info.Custom
	c.Server, c.Port, c.Protocol, c.Enabled, c.PathEcowitt = server, port, "ecowitt", true, "/data/report/"
	if interval < 16 {
		interval = 60
	}
	c.Interval = interval
	cb, pb := packEcowittCustom(c)
	if _, err := ecowittRequest(ctx, host, cmdWriteCustom, cb); err != nil {
		return c, fmt.Errorf("scrittura impostazioni: %v", err)
	}
	if _, err := ecowittRequest(ctx, host, cmdWriteUserPath, pb); err != nil {
		return c, fmt.Errorf("scrittura percorso: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	after := ReadEcowittConsole(ctx, host)
	if after.Custom == nil {
		return c, fmt.Errorf("impostazioni scritte ma non rilette: %s", after.APIError)
	}
	a := *after.Custom
	if a.Server != c.Server || a.Port != c.Port || !a.Enabled || a.Protocol != "ecowitt" || a.PathEcowitt != c.PathEcowitt {
		return a, fmt.Errorf("la console non ha accettato le impostazioni (ora: server %s porta %d attivo %v)", a.Server, a.Port, a.Enabled)
	}
	return a, nil
}

// pcIPFor sceglie l'indirizzo di questo PC nella stessa rete della console.
func pcIPFor(host string) string {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	ips := localIPs()
	target := net.ParseIP(h).To4()
	if target != nil {
		for _, ip := range ips {
			p := net.ParseIP(ip).To4()
			if p != nil && p[0] == target[0] && p[1] == target[1] && p[2] == target[2] {
				return ip
			}
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}
