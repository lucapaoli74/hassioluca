//go:build windows

package main

import (
	"log"
	"os/exec"
	"path/filepath"
	"sync"
)

const ffmpegFirewallRule = "Webcam Manager ffmpeg"

var firewallDone sync.Map

// allowFFmpegFirewall apre il firewall di Windows a ffmpeg.exe: con RTSP su
// UDP la telecamera invia il video verso ffmpeg, che altrimenti verrebbe
// bloccato. Serve il servizio (o un prompt da amministratore).
func allowFFmpegFirewall(path string) {
	path, _ = filepath.Abs(path)
	if _, done := firewallDone.LoadOrStore(path, true); done {
		return
	}
	go func() {
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+ffmpegFirewallRule).Run()
		out, err := exec.Command("netsh", "advfirewall", "firewall", "add", "rule", "name="+ffmpegFirewallRule,
			"dir=in", "action=allow", "program="+path, "enable=yes", "profile=any").CombinedOutput()
		if err != nil {
			log.Printf("regola firewall per ffmpeg non creata (avvia come servizio o da amministratore): %v %s", err, out)
		}
	}()
}

func removeFFmpegFirewall() {
	_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+ffmpegFirewallRule).Run()
}
