package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StationProbe fa quello che fa l'app WS View per trovare la console:
// chiede alla stazione (UDP 46000, diretto e in broadcast) su quale porta
// accetta i comandi, ascolta gli annunci spontanei, prova tutte le porte TCP
// e su ogni porta aperta prova i comandi di configurazione.
type StationProbeResult struct {
	Host       string   `json:"host"`
	Report     string   `json:"report"`
	ConfigAddr string   `json:"config_addr,omitempty"` // host:porta dove la console accetta i comandi
	OpenPorts  []int    `json:"open_ports"`
	Announced  []string `json:"announced,omitempty"`
}

func StationProbe(ctx context.Context, host string) StationProbeResult {
	res := StationProbeResult{Host: host, OpenPorts: []int{}}
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	line("Analisi della stazione %s · %s", host, time.Now().Format("02/01/2006 15:04"))

	// 1. annuncio UDP: la console risponde con MAC, IP e porta dei comandi
	var mu sync.Mutex
	ports := map[int]bool{}
	got := func(d StationDevice) {
		mu.Lock()
		defer mu.Unlock()
		res.Announced = append(res.Announced, fmt.Sprintf("%s %s %s", d.Host, d.MAC, d.Name))
		h, p, err := net.SplitHostPort(d.Host)
		if err != nil {
			h, p = d.Host, strconv.Itoa(ecowittPort)
		}
		if h == host {
			n, _ := strconv.Atoi(p)
			ports[n] = true
		}
	}
	if conn, err := net.ListenUDP("udp4", &net.UDPAddr{}); err == nil {
		go func() {
			for i := 0; i < 3; i++ {
				_, _ = conn.WriteToUDP(ecowittCmd(0x12), &net.UDPAddr{IP: net.ParseIP(host), Port: 46000})
				_, _ = conn.WriteToUDP(ecowittCmd(0x12), &net.UDPAddr{IP: net.IPv4bcast, Port: 46000})
				time.Sleep(time.Second)
			}
		}()
		buf := make([]byte, 512)
		_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			if d, ok := parseEcowittBroadcast(buf[:n]); ok {
				got(d)
			} else {
				line("  risposta UDP da %s: %X", from, buf[:min(n, 40)])
			}
		}
		conn.Close()
	}
	if len(res.Announced) > 0 {
		line("[1] Risposte all'annuncio (come WS View):")
		for _, a := range res.Announced {
			line("    %s", a)
		}
	} else {
		line("[1] Nessuna risposta all'annuncio UDP 46000 (la console non si presenta come le Ecowitt)")
	}

	// 2. tutte le porte TCP
	line("[2] Porte TCP aperte (1-65535):")
	open := scanAllPorts(ctx, host)
	res.OpenPorts = open
	if len(open) == 0 {
		line("    nessuna: la stazione non accetta collegamenti dalla rete")
	}
	for _, p := range open {
		ports[p] = true
		line("    %d aperta", p)
	}

	// 3. comandi di configurazione su ogni porta candidata
	line("[3] Comandi di configurazione:")
	cand := []int{}
	for p := range ports {
		cand = append(cand, p)
	}
	sort.Ints(cand)
	for _, p := range cand {
		addr := net.JoinHostPort(host, strconv.Itoa(p))
		cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		fw, err := ecowittRequest(cctx, addr, cmdReadFirmware, nil)
		cancel()
		if err != nil {
			line("    porta %d: %v", p, err)
			continue
		}
		v := ""
		if len(fw) > 1 {
			v = string(fw[1:])
		}
		line("    porta %d: RISPONDE (firmware %s)", p, v)
		if res.ConfigAddr == "" {
			res.ConfigAddr = addr
		}
	}
	if len(cand) == 0 {
		line("    nessuna porta da provare")
	}
	if res.ConfigAddr != "" {
		line("\nRISULTATO: la stazione si può impostare da qui (%s).", res.ConfigAddr)
	} else {
		line("\nRISULTATO: la stazione non accetta comandi dalla rete. Si imposta solo con l'app sul telefono collegato alla stessa rete, oppure si usano i dati di Weather Underground.")
	}
	res.Report = b.String()
	return res
}

// scanAllPorts prova tutte le porte TCP di un solo indirizzo.
func scanAllPorts(ctx context.Context, host string) []int {
	ports := make(chan int, 512)
	var mu sync.Mutex
	var open []int
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := net.Dialer{Timeout: 700 * time.Millisecond}
			for p := range ports {
				c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(p)))
				if err == nil {
					c.Close()
					mu.Lock()
					open = append(open, p)
					mu.Unlock()
				}
			}
		}()
	}
	for p := 1; p <= 65535 && ctx.Err() == nil; p++ {
		ports <- p
	}
	close(ports)
	wg.Wait()
	sort.Ints(open)
	return open
}
