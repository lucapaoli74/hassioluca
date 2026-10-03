//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const systemdUnit = "/etc/systemd/system/webcam-manager.service"

func isService() bool { return false }

func runService(dataDir, listen string) error { return errors.New("non supportato") }

func systemDataDir() string {
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		return "/var/lib/webcam-manager"
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "webcam-manager")
	}
	return "data"
}

// installService su Linux crea e abilita un servizio systemd.
func installService(dataDir, listen string) error {
	if runtime.GOOS != "linux" {
		return errors.New("installazione automatica disponibile su Windows e Linux (systemd)")
	}
	if os.Geteuid() != 0 {
		return errors.New("esegui con sudo")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := prepareDataDir(dataDir, listen); err != nil {
		return err
	}
	args := []string{exe, "-data", dataDir}
	if listen != "" {
		args = append(args, "-listen", listen)
	}
	unit := fmt.Sprintf(`[Unit]
Description=Webcam Manager
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=%s
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`, strings.Join(args, " "))
	if err := os.WriteFile(systemdUnit, []byte(unit), 0o644); err != nil {
		return err
	}
	for _, a := range [][]string{{"daemon-reload"}, {"enable", "--now", "webcam-manager"}} {
		if out, err := exec.Command("systemctl", a...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %v %s", strings.Join(a, " "), err, out)
		}
	}
	printAccess(dataDir)
	fmt.Println("Servizio installato: partirà automaticamente a ogni avvio.")
	return nil
}

func uninstallService() error {
	if runtime.GOOS != "linux" {
		return errors.New("non supportato su questo sistema")
	}
	_ = exec.Command("systemctl", "disable", "--now", "webcam-manager").Run()
	if err := os.Remove(systemdUnit); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println("Servizio rimosso. I dati restano in", systemDataDir())
	return nil
}

func controlService(start bool) error {
	action := "stop"
	if start {
		action = "start"
	}
	out, err := exec.Command("systemctl", action, "webcam-manager").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v %s", err, out)
	}
	return nil
}
