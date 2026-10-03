package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

// FoundDevice è un dispositivo trovato dalla ricerca in rete.
type FoundDevice struct {
	IP          string       `json:"ip"`
	Ports       []int        `json:"ports"`
	ONVIF       bool         `json:"onvif"`
	XAddr       string       `json:"xaddr,omitempty"` // indirizzo del servizio ONVIF
	Name        string       `json:"name,omitempty"`
	Hardware    string       `json:"hardware,omitempty"`
	Brand       string       `json:"brand,omitempty"`
	Server      string       `json:"server,omitempty"` // banner HTTP
	Suggestions []Suggestion `json:"suggestions"`
	Configured  string       `json:"configured,omitempty"` // ID della webcam già configurata con questo IP
}

// Suggestion è un URL probabile per la telecamera, da provare nel pannello.
type Suggestion struct {
	Label  string `json:"label"`
	Source string `json:"source"`
	URL    string `json:"url"`
}

// Porte tipiche delle telecamere IP: RTSP, web, SDK Hikvision/Dahua/XMEye.
var scanPorts = []int{554, 80, 8080, 8000, 81, 88, 8899, 37777, 34567, 443}

// Discover cerca le telecamere nella rete locale combinando ONVIF
// WS-Discovery (multicast) e una scansione delle porte sulle sottoreti /24
// delle schede di rete del computer.
func Discover(ctx context.Context) ([]FoundDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	devices := map[string]*FoundDevice{}
	var mu sync.Mutex
	get := func(ip string) *FoundDevice {
		d := devices[ip]
		if d == nil {
			d = &FoundDevice{IP: ip}
			devices[ip] = d
		}
		return d
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, m := range wsDiscover(ctx, 4*time.Second) {
			mu.Lock()
			d := get(m.ip)
			d.ONVIF = true
			d.XAddr = m.xaddr
			d.Name, d.Hardware = m.name, m.hardware
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for ip, ports := range portScan(ctx, localHosts()) {
			mu.Lock()
			get(ip).Ports = ports
			mu.Unlock()
		}
	}()
	wg.Wait()

	// banner HTTP per riconoscere la marca
	var bw sync.WaitGroup
	for _, d := range devices {
		for _, p := range d.Ports {
			if p == 80 || p == 8080 || p == 8000 || p == 81 || p == 88 || p == 8899 {
				bw.Add(1)
				go func(d *FoundDevice, port int) {
					defer bw.Done()
					banner := httpBanner(ctx, d.IP, port)
					mu.Lock()
					if d.Server == "" {
						d.Server = banner
					}
					mu.Unlock()
				}(d, p)
				break
			}
		}
	}
	bw.Wait()

	out := []FoundDevice{}
	for _, d := range devices {
		d.Brand = guessBrand(d)
		if !d.ONVIF && d.Brand == "" && !hasAny(d.Ports, 554, 37777, 34567) {
			continue // probabilmente non è una telecamera (PC, router, stampante...)
		}
		d.Suggestions = suggestURLs(d)
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := net.ParseIP(out[i].IP).To4(), net.ParseIP(out[j].IP).To4()
		return bytes.Compare(a, b) < 0
	})
	return out, nil
}

func hasAny(ports []int, want ...int) bool {
	for _, p := range ports {
		for _, w := range want {
			if p == w {
				return true
			}
		}
	}
	return false
}

// localHosts restituisce gli indirizzi da scansionare: la /24 di ogni
// scheda di rete IPv4 attiva (reti più grandi vengono limitate alla propria /24).
func localHosts() []string {
	seen := map[string]bool{}
	var hosts []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones < 24 {
				ones = 24
			}
			mask := net.CIDRMask(ones, 32)
			base := ip.Mask(mask)
			size := 1 << (32 - ones)
			for i := 1; i < size-1; i++ {
				h := make(net.IP, 4)
				copy(h, base)
				v := uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
				v += uint32(i)
				h = net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).To4()
				if h.Equal(ip) || seen[h.String()] {
					continue
				}
				seen[h.String()] = true
				hosts = append(hosts, h.String())
			}
		}
	}
	return hosts
}

func portScan(ctx context.Context, hosts []string) map[string][]int {
	type job struct {
		ip   string
		port int
	}
	jobs := make(chan job)
	res := map[string][]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < 256; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := net.Dialer{Timeout: 600 * time.Millisecond}
			for j := range jobs {
				c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(j.ip, strconv.Itoa(j.port)))
				if err != nil {
					continue
				}
				c.Close()
				mu.Lock()
				res[j.ip] = append(res[j.ip], j.port)
				mu.Unlock()
			}
		}()
	}
loop:
	for _, h := range hosts {
		for _, p := range scanPorts {
			select {
			case jobs <- job{h, p}:
			case <-ctx.Done():
				break loop
			}
		}
	}
	close(jobs)
	wg.Wait()
	for ip := range res {
		sort.Ints(res[ip])
	}
	return res
}

var titleRe = regexp.MustCompile(`(?is)<title>(.*?)</title>`)

// httpBanner legge header Server, realm e titolo della pagina.
func httpBanner(ctx context.Context, ip string, port int) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/", ip, port), nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	parts := []string{}
	if s := resp.Header.Get("Server"); s != "" {
		parts = append(parts, s)
	}
	if s := resp.Header.Get("WWW-Authenticate"); s != "" {
		parts = append(parts, s)
	}
	if m := titleRe.FindSubmatch(body); m != nil {
		parts = append(parts, strings.TrimSpace(string(m[1])))
	}
	return strings.Join(parts, " | ")
}

var brands = []struct{ key, name string }{
	{"hikvision", "Hikvision"}, {"hikcam", "Hikvision"}, {"dnvrs", "Hikvision"},
	{"dahua", "Dahua"}, {"amcrest", "Dahua"}, {"lorex", "Dahua"},
	{"reolink", "Reolink"}, {"axis", "Axis"}, {"foscam", "Foscam"},
	{"ezviz", "EZVIZ"}, {"uniview", "Uniview"}, {"unv", "Uniview"},
	{"vivotek", "Vivotek"}, {"mobotix", "Mobotix"}, {"tp-link", "TP-Link"}, {"tapo", "TP-Link"},
	{"xmeye", "XMEye"}, {"netsurveillance", "XMEye"}, {"ipcam", "Generica"}, {"webcam", "Generica"},
	{"camera", "Generica"},
}

func guessBrand(d *FoundDevice) string {
	hay := strings.ToLower(d.Name + " " + d.Hardware + " " + d.Server)
	for _, b := range brands {
		if strings.Contains(hay, b.key) {
			return b.name
		}
	}
	switch {
	case hasAny(d.Ports, 37777):
		return "Dahua"
	case hasAny(d.Ports, 34567):
		return "XMEye"
	case hasAny(d.Ports, 8000) && hasAny(d.Ports, 554) && !hasAny(d.Ports, 80, 8080, 443):
		return "EZVIZ" // RTSP + SDK Hikvision senza pagina web: tipico delle EZVIZ
	case hasAny(d.Ports, 8000) && hasAny(d.Ports, 554):
		return "Hikvision"
	}
	return ""
}

func suggestURLs(d *FoundDevice) []Suggestion {
	ip := d.IP
	var s []Suggestion
	add := func(label, source, u string) { s = append(s, Suggestion{label, source, u}) }
	switch d.Brand {
	case "Hikvision":
		if hasAny(d.Ports, 80, 8080) {
			add("Snapshot Hikvision", "snapshot", "http://"+ip+"/ISAPI/Streaming/channels/101/picture")
		}
		add("RTSP principale", "rtsp", "rtsp://"+ip+":554/Streaming/Channels/101")
		add("RTSP secondario", "rtsp", "rtsp://"+ip+":554/Streaming/Channels/102")
		// spesso è una EZVIZ (marchio consumer di Hikvision): stessi percorsi più questi
		add("RTSP EZVIZ", "rtsp", "rtsp://"+ip+":554/H.264")
	case "EZVIZ":
		add("RTSP EZVIZ principale", "rtsp", "rtsp://"+ip+":554/h264/ch1/main/av_stream")
		add("RTSP EZVIZ secondario", "rtsp", "rtsp://"+ip+":554/h264/ch1/sub/av_stream")
		add("RTSP EZVIZ (modelli recenti)", "rtsp", "rtsp://"+ip+":554/H.264")
	case "Dahua":
		add("Snapshot Dahua", "snapshot", "http://"+ip+"/cgi-bin/snapshot.cgi")
		add("RTSP principale", "rtsp", "rtsp://"+ip+":554/cam/realmonitor?channel=1&subtype=0")
		add("RTSP secondario", "rtsp", "rtsp://"+ip+":554/cam/realmonitor?channel=1&subtype=1")
	case "Reolink":
		add("RTSP principale", "rtsp", "rtsp://"+ip+":554/h264Preview_01_main")
		add("RTSP secondario", "rtsp", "rtsp://"+ip+":554/h264Preview_01_sub")
	case "Axis":
		add("Snapshot Axis", "snapshot", "http://"+ip+"/axis-cgi/jpg/image.cgi")
		add("MJPEG Axis", "mjpeg", "http://"+ip+"/axis-cgi/mjpg/video.cgi")
		add("RTSP", "rtsp", "rtsp://"+ip+"/axis-media/media.amp")
	case "Foscam":
		add("RTSP", "rtsp", "rtsp://"+ip+":88/videoMain")
	case "Uniview":
		add("RTSP principale", "rtsp", "rtsp://"+ip+":554/unicast/c1/s0/live")
	case "TP-Link":
		add("RTSP principale", "rtsp", "rtsp://"+ip+":554/stream1")
		add("RTSP secondario", "rtsp", "rtsp://"+ip+":554/stream2")
	case "XMEye":
		add("RTSP", "rtsp", "rtsp://"+ip+":554/user=admin_password=_channel=1_stream=0.sdp")
	}
	if hasAny(d.Ports, 554) && len(s) == 0 {
		add("RTSP generico", "rtsp", "rtsp://"+ip+":554/")
	}
	return s
}

// ---------------- ONVIF WS-Discovery ----------------

type wsMatch struct{ ip, xaddr, name, hardware string }

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func probeMessage() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
<e:Header><w:MessageID>uuid:` + uuid() + `</w:MessageID><w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To><w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header>
<e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`)
}

type probeMatches struct {
	Matches []struct {
		XAddrs string `xml:"XAddrs"`
		Scopes string `xml:"Scopes"`
	} `xml:"Body>ProbeMatches>ProbeMatch"`
}

// wsDiscover invia la Probe ONVIF da ogni scheda di rete e raccoglie le risposte.
func wsDiscover(ctx context.Context, wait time.Duration) []wsMatch {
	group := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702}
	var mu sync.Mutex
	found := map[string]wsMatch{}
	var wg sync.WaitGroup
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagMulticast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		ifc := ifc
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.ListenPacket("udp4", ":0")
			if err != nil {
				return
			}
			defer c.Close()
			p := ipv4.NewPacketConn(c)
			_ = p.SetMulticastInterface(&ifc)
			_ = p.SetMulticastTTL(2)
			for i := 0; i < 2; i++ { // UDP: meglio ripetere
				_, _ = c.WriteTo(probeMessage(), group)
			}
			deadline := time.Now().Add(wait)
			if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
				deadline = d
			}
			_ = c.SetReadDeadline(deadline)
			buf := make([]byte, 65536)
			for {
				n, from, err := c.ReadFrom(buf)
				if err != nil {
					return
				}
				var pm probeMatches
				if xml.Unmarshal(buf[:n], &pm) != nil {
					continue
				}
				for _, m := range pm.Matches {
					ip := from.(*net.UDPAddr).IP.String()
					w := wsMatch{ip: ip}
					for _, x := range strings.Fields(m.XAddrs) {
						if u, err := url.Parse(x); err == nil && u.Hostname() == ip {
							w.xaddr = x
							break
						}
						if w.xaddr == "" {
							w.xaddr = x
						}
					}
					for _, sc := range strings.Fields(m.Scopes) {
						if v, ok := strings.CutPrefix(sc, "onvif://www.onvif.org/name/"); ok {
							w.name, _ = url.PathUnescape(v)
						}
						if v, ok := strings.CutPrefix(sc, "onvif://www.onvif.org/hardware/"); ok {
							w.hardware, _ = url.PathUnescape(v)
						}
					}
					mu.Lock()
					found[ip] = w
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	out := make([]wsMatch, 0, len(found))
	for _, m := range found {
		out = append(out, m)
	}
	return out
}

// ---------------- ONVIF: lettura di marca, profili e URL ----------------

// OnvifInfo descrive una telecamera ONVIF interrogata con le sue credenziali.
type OnvifInfo struct {
	Manufacturer string         `json:"manufacturer"`
	Model        string         `json:"model"`
	Firmware     string         `json:"firmware"`
	Profiles     []OnvifProfile `json:"profiles"`
}

type OnvifProfile struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	SnapshotURI string `json:"snapshot_uri"`
	StreamURI   string `json:"stream_uri"`
}

type onvifClient struct {
	xaddr, user, pass string
	offset            time.Duration // differenza fra orologio della telecamera e il nostro
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (c *onvifClient) call(ctx context.Context, endpoint, body string, out any) error {
	header := ""
	if c.user != "" {
		nonce := make([]byte, 16)
		_, _ = rand.Read(nonce)
		created := time.Now().Add(c.offset).UTC().Format("2006-01-02T15:04:05.000Z")
		h := sha1.New()
		h.Write(nonce)
		h.Write([]byte(created))
		h.Write([]byte(c.pass))
		digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
		header = `<s:Header><Security s:mustUnderstand="1" xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"><UsernameToken><Username>` +
			xmlEscape(c.user) + `</Username><Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` +
			digest + `</Password><Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` +
			base64.StdEncoding.EncodeToString(nonce) + `</Nonce><Created xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">` +
			created + `</Created></UsernameToken></Security></s:Header>`
	}
	env := `<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` + header + `<s:Body>` + body + `</s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(env))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || bytes.Contains(data, []byte("NotAuthorized")) {
			return fmt.Errorf("credenziali ONVIF rifiutate")
		}
		return fmt.Errorf("ONVIF ha risposto %s", resp.Status)
	}
	return xml.Unmarshal(data, out)
}

// QueryOnvif legge informazioni, profili e URL di snapshot/stream.
func QueryOnvif(ctx context.Context, xaddr, user, pass string) (*OnvifInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := &onvifClient{xaddr: xaddr, user: user, pass: pass}

	// L'autenticazione ONVIF dipende dall'ora: ci allineiamo all'orologio della telecamera.
	var dt struct {
		D struct {
			Y  int `xml:"Year"`
			Mo int `xml:"Month"`
			Dd int `xml:"Day"`
		} `xml:"Body>GetSystemDateAndTimeResponse>SystemDateAndTime>UTCDateTime>Date"`
		T struct {
			H int `xml:"Hour"`
			M int `xml:"Minute"`
			S int `xml:"Second"`
		} `xml:"Body>GetSystemDateAndTimeResponse>SystemDateAndTime>UTCDateTime>Time"`
	}
	anon := &onvifClient{xaddr: xaddr}
	if err := anon.call(ctx, xaddr, `<GetSystemDateAndTime xmlns="http://www.onvif.org/ver10/device/wsdl"/>`, &dt); err == nil && dt.D.Y > 2000 {
		cam := time.Date(dt.D.Y, time.Month(dt.D.Mo), dt.D.Dd, dt.T.H, dt.T.M, dt.T.S, 0, time.UTC)
		c.offset = time.Until(cam)
	}

	info := &OnvifInfo{Profiles: []OnvifProfile{}}
	var di struct {
		R struct {
			Manufacturer string `xml:"Manufacturer"`
			Model        string `xml:"Model"`
			Firmware     string `xml:"FirmwareVersion"`
		} `xml:"Body>GetDeviceInformationResponse"`
	}
	if err := c.call(ctx, xaddr, `<GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/>`, &di); err != nil {
		return nil, err
	}
	info.Manufacturer, info.Model, info.Firmware = di.R.Manufacturer, di.R.Model, di.R.Firmware

	var caps struct {
		Media string `xml:"Body>GetCapabilitiesResponse>Capabilities>Media>XAddr"`
	}
	media := xaddr
	if err := c.call(ctx, xaddr, `<GetCapabilities xmlns="http://www.onvif.org/ver10/device/wsdl"><Category>Media</Category></GetCapabilities>`, &caps); err == nil && caps.Media != "" {
		media = caps.Media
	}

	var pr struct {
		Profiles []struct {
			Token string `xml:"token,attr"`
			Name  string `xml:"Name"`
			W     int    `xml:"VideoEncoderConfiguration>Resolution>Width"`
			H     int    `xml:"VideoEncoderConfiguration>Resolution>Height"`
		} `xml:"Body>GetProfilesResponse>Profiles"`
	}
	if err := c.call(ctx, media, `<GetProfiles xmlns="http://www.onvif.org/ver10/media/wsdl"/>`, &pr); err != nil {
		return nil, fmt.Errorf("profili: %w", err)
	}
	for i, p := range pr.Profiles {
		if i >= 4 {
			break
		}
		op := OnvifProfile{Token: p.Token, Name: p.Name, Width: p.W, Height: p.H}
		var su, st struct {
			URI string `xml:"Body>*>MediaUri>Uri"`
		}
		tok := xmlEscape(p.Token)
		if c.call(ctx, media, `<GetSnapshotUri xmlns="http://www.onvif.org/ver10/media/wsdl"><ProfileToken>`+tok+`</ProfileToken></GetSnapshotUri>`, &su) == nil {
			op.SnapshotURI = su.URI
		}
		if c.call(ctx, media, `<GetStreamUri xmlns="http://www.onvif.org/ver10/media/wsdl"><StreamSetup><Stream xmlns="http://www.onvif.org/ver10/schema">RTP-Unicast</Stream><Transport xmlns="http://www.onvif.org/ver10/schema"><Protocol>RTSP</Protocol></Transport></StreamSetup><ProfileToken>`+tok+`</ProfileToken></GetStreamUri>`, &st) == nil {
			op.StreamURI = st.URI
		}
		info.Profiles = append(info.Profiles, op)
	}
	return info, nil
}
