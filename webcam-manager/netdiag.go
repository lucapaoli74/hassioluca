package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// networkDiagnosis raccoglie le informazioni di rete utili quando la
// telecamera non risponde: ping, altre porte, test di Windows, programmi già
// collegati alla telecamera e antivirus installato.
func networkDiagnosis(ctx context.Context, hostPort string) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	host, port, _ := net.SplitHostPort(hostPort)
	line("\n[0] Rete")

	// ping
	args := []string{"-c", "2", "-W", "2", host}
	if runtime.GOOS == "windows" {
		args = []string{"-n", "2", "-w", "2000", host}
	}
	out := runCmd(ctx, 10*time.Second, "ping", args...)
	ok := strings.Contains(strings.ToLower(out), "ttl=")
	line("    ping %s: %s", host, map[bool]string{true: "risponde", false: "NON risponde"}[ok])

	// altre porte della telecamera
	for _, p := range []string{port, "80", "8000", "443"} {
		start := time.Now()
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, p), 3*time.Second)
		if err == nil {
			c.Close()
			line("    porta %s: aperta (%d ms)", p, time.Since(start).Milliseconds())
		} else {
			line("    porta %s: %s", p, shortNetErr(err))
		}
	}

	if runtime.GOOS != "windows" {
		return b.String()
	}
	// lo stesso test fatto da Windows
	tnc := runCmd(ctx, 25*time.Second, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf("(Test-NetConnection %s -Port %s -WarningAction SilentlyContinue).TcpTestSucceeded", host, port))
	line("    test di Windows (Test-NetConnection porta %s): %s", port, strings.TrimSpace(tnc))

	// programmi collegati in questo momento alla telecamera
	ns := runCmd(ctx, 10*time.Second, "netstat", "-ano", "-p", "TCP")
	found := false
	sc := bufio.NewScanner(strings.NewReader(ns))
	fields := regexp.MustCompile(`\s+`)
	for sc.Scan() {
		f := fields.Split(strings.TrimSpace(sc.Text()), -1)
		if len(f) < 5 || !strings.HasPrefix(f[2], host+":") {
			continue
		}
		name := strings.TrimSpace(runCmd(ctx, 5*time.Second, "powershell", "-NoProfile", "-Command",
			"(Get-Process -Id "+f[4]+" -ErrorAction SilentlyContinue).ProcessName"))
		line("    collegamento attivo: %s → %s (%s) programma: %s [PID %s]", f[1], f[2], f[3], name, f[4])
		found = true
	}
	if !found {
		line("    nessun programma di questo PC è collegato ora alla telecamera")
	}

	// antivirus
	av := runCmd(ctx, 15*time.Second, "powershell", "-NoProfile", "-Command",
		"(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntivirusProduct -ErrorAction SilentlyContinue).displayName -join ', '")
	if av = strings.TrimSpace(av); av == "" {
		av = "non rilevato"
	}
	line("    antivirus: %s", av)
	line("    questo programma gira come: %s", whoami(ctx))
	return b.String()
}

func shortNetErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "timeout"):
		return "nessuna risposta (bloccata o filtrata)"
	case strings.Contains(s, "refused"):
		return "chiusa"
	}
	return s
}

func whoami(ctx context.Context) string {
	if runtime.GOOS == "windows" {
		return strings.TrimSpace(runCmd(ctx, 5*time.Second, "whoami"))
	}
	return strings.TrimSpace(runCmd(ctx, 5*time.Second, "id", "-un"))
}

// runCmd esegue un comando e ne restituisce l'output (vuoto in caso di errore).
func runCmd(ctx context.Context, timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out, _ := cmd.CombinedOutput()
	return string(out)
}
