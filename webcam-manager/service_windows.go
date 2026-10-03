//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "WebcamManager"
	serviceDisplay = "Webcam Manager"
	firewallRule   = "Webcam Manager"
)

func isService() bool {
	ok, _ := svc.IsWindowsService()
	return ok
}

func systemDataDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "WebcamManager")
}

type winService struct{ dataDir, listen string }

func (w winService) Execute(_ []string, req <-chan svc.ChangeRequest, st chan<- svc.Status) (bool, uint32) {
	st <- svc.Status{State: svc.StartPending}
	setupFileLog(w.dataDir)
	app, err := NewApp(w.dataDir, w.listen)
	if err != nil {
		log.Printf("avvio non riuscito: %v", err)
		return true, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	st <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				st <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				st <- svc.Status{State: svc.StopPending}
				cancel()
				<-done
				return false, 0
			}
		case err := <-done:
			if errors.Is(err, errRestart) {
				// codice d'uscita non nullo: le azioni di ripristino riavviano
				// il servizio con il nuovo eseguibile
				log.Printf("riavvio dopo aggiornamento")
				return true, 2
			}
			if err != nil {
				log.Printf("arresto per errore: %v", err)
				return true, 1 // Windows riavvierà il servizio (azioni di ripristino)
			}
			return false, 0
		}
	}
}

func runService(dataDir, listen string) error {
	return svc.Run(serviceName, winService{dataDir: dataDir, listen: listen})
}

// installService registra il programma come servizio di Windows con avvio
// automatico (ritardato, così la rete è pronta), riavvio automatico in caso di
// errore e una regola nel firewall per il pannello e la ricerca ONVIF.
func installService(dataDir, listen string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)
	if err := prepareDataDir(dataDir, listen); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("serve un prompt come amministratore (%w)", err)
	}
	defer m.Disconnect()
	if old, err := m.OpenService(serviceName); err == nil {
		old.Close()
		return errors.New("il servizio è già installato: esegui prima \"webcam-manager uninstall\"")
	}
	args := []string{"-data", dataDir}
	if listen != "" {
		args = append(args, "-listen", listen)
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName:      serviceDisplay,
		Description:      "Acquisisce le webcam e le pubblica sui siti (pannello web sulla porta configurata).",
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true,
	}, args...)
	if err != nil {
		return fmt.Errorf("creazione servizio: %w", err)
	}
	defer s.Close()
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 24*60*60)
	// riavvia anche quando il servizio si ferma "volontariamente" con errore
	// (succede dopo un aggiornamento automatico)
	_ = s.SetRecoveryActionsOnNonCrashFailures(true)

	_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+firewallRule).Run()
	if out, err := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRule, "dir=in", "action=allow", "program="+exe, "enable=yes", "profile=any").CombinedOutput(); err != nil {
		fmt.Printf("attenzione: regola firewall non creata: %v %s\n", err, out)
	}

	// h24: il PC non deve andare in sospensione quando è alimentato a rete
	_ = exec.Command("powercfg", "/change", "standby-timeout-ac", "0").Run()
	_ = exec.Command("powercfg", "/change", "hibernate-timeout-ac", "0").Run()

	if err := s.Start(); err != nil {
		return fmt.Errorf("servizio installato ma non avviato: %w", err)
	}
	printAccess(dataDir)
	fmt.Println("Servizio installato: partirà automaticamente a ogni accensione del computer.")
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("serve un prompt come amministratore (%w)", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return errors.New("il servizio non è installato")
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State != svc.Stopped {
		_, _ = s.Control(svc.Stop)
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			if st, err := s.Query(); err != nil || st.State == svc.Stopped {
				break
			}
		}
	}
	if err := s.Delete(); err != nil {
		return err
	}
	_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+firewallRule).Run()
	removeFFmpegFirewall()
	fmt.Println("Servizio rimosso. Configurazione e storico restano in", systemDataDir())
	return nil
}

func controlService(start bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("serve un prompt come amministratore (%w)", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return errors.New("il servizio non è installato")
	}
	defer s.Close()
	if start {
		return s.Start()
	}
	_, err = s.Control(svc.Stop)
	return err
}
