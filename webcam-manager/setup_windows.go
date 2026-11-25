//go:build windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Installazione guidata in console: con un doppio clic sull'eseguibile, se il
// servizio non c'è ancora, il programma fa poche domande, chiede i permessi
// di amministratore, si copia in Program Files e si installa come servizio.

func installDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "Webcam Manager")
}

// serviceState: "assente", "in esecuzione" o "fermo".
func serviceState() string {
	m, err := mgr.Connect()
	if err != nil {
		// senza diritti di amministratore si può comunque leggere lo stato
		h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
		if err != nil {
			return "sconosciuto"
		}
		defer windows.CloseServiceHandle(h)
		name, _ := windows.UTF16PtrFromString(serviceName)
		s, err := windows.OpenService(h, name, windows.SERVICE_QUERY_STATUS)
		if err != nil {
			return "assente"
		}
		defer windows.CloseServiceHandle(s)
		var st windows.SERVICE_STATUS
		if windows.QueryServiceStatus(s, &st) == nil && st.CurrentState == windows.SERVICE_RUNNING {
			return "in esecuzione"
		}
		return "fermo"
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return "assente"
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State == svc.Running {
		return "in esecuzione"
	}
	return "fermo"
}

func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// runElevated rilancia l'eseguibile come amministratore (finestra UAC).
func runElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var quoted []string
	for _, a := range args {
		quoted = append(quoted, syscall.EscapeArg(a))
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	if err := windows.ShellExecute(0, verb, file, params, dir, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("permesso di amministratore negato o non disponibile: %w", err)
	}
	return nil
}

func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func panelURL(dataDir string) string {
	port := "8080"
	if store, _, err := LoadStore(filepath.Join(dataDir, "config.json")); err == nil {
		l := store.Get().Listen
		if i := strings.LastIndex(l, ":"); i >= 0 {
			port = l[i+1:]
		}
	}
	return "http://localhost:" + port
}

func pause(in *bufio.Reader) {
	fmt.Print("\nPremi Invio per chiudere...")
	_, _ = in.ReadString('\n')
}

func ask(in *bufio.Reader, question, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", question, def)
	} else {
		fmt.Printf("%s: ", question)
	}
	s, _ := in.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

// interactiveSetup è il menu mostrato con il doppio clic. Restituisce true se
// ha gestito lui l'avvio (installazione, servizio già attivo…); false se il
// programma deve partire normalmente in questa finestra.
func interactiveSetup(dataDir string) bool {
	in := bufio.NewReader(os.Stdin)
	fmt.Printf("Webcam Manager %s — %s\n\n", version, credits)
	switch serviceState() {
	case "in esecuzione":
		url := panelURL(dataDir)
		fmt.Println("Webcam Manager è già installato come servizio ed è IN ESECUZIONE.")
		fmt.Println("Pannello:", url)
		exe, _ := os.Executable()
		if inst := filepath.Join(installDir(), "webcam-manager.exe"); !strings.EqualFold(exe, inst) {
			fmt.Println("\n1) Apri il pannello")
			fmt.Println("2) Aggiorna il servizio con QUESTO eseguibile (versione " + version + ")")
			if ask(in, "Scelta", "1") == "2" {
				return elevateAndWait(in, []string{"setup-install", "-data", dataDir, "-update"})
			}
		}
		openBrowser(url)
		return true
	case "fermo":
		fmt.Println("Webcam Manager è installato come servizio ma è FERMO.")
		if strings.ToLower(ask(in, "Lo avvio adesso? (S/N)", "S")) != "n" {
			return elevateAndWait(in, []string{"start-and-open", "-data", dataDir})
		}
		return true
	}

	fmt.Println("Webcam Manager non è ancora installato come servizio.")
	fmt.Println("Installandolo partirà da solo a ogni accensione del PC e resterà sempre attivo (h24).")
	fmt.Println()
	fmt.Println("1) Installa come servizio (consigliato)")
	fmt.Println("2) Avvia solo adesso, in questa finestra (si ferma chiudendola)")
	if ask(in, "Scelta", "1") == "2" {
		return false
	}
	// un'altra copia aperta occuperebbe la porta del pannello
	if url := panelURL(dataDir); portBusy(url) {
		fmt.Println("\nWebcam Manager è già aperto in un'altra finestra (" + url + ").")
		fmt.Println("Chiudi quella finestra nera e poi fai di nuovo doppio clic qui.")
		pause(in)
		return true
	}
	cfg := Config{}
	if store, _, err := LoadStore(filepath.Join(dataDir, "config.json")); err == nil {
		cfg = store.Get()
	}
	fmt.Println()
	loc := ask(in, "Nome della località", cfg.Location.Name)
	alt := ask(in, "Altitudine in metri s.l.m.", map[bool]string{true: strconv.Itoa(cfg.Location.Altitude), false: ""}[cfg.Location.Altitude > 0])
	pwd := ask(in, "Password del pannello (vuoto = lascia quella attuale)", "")
	args := []string{"setup-install", "-data", dataDir}
	if loc != "" {
		args = append(args, "-location", loc)
	}
	if n, err := strconv.Atoi(alt); err == nil && n >= 0 {
		args = append(args, "-altitude", strconv.Itoa(n))
	}
	if pwd != "" {
		args = append(args, "-admin-password", pwd)
	}
	fmt.Println("\nWindows ora chiederà il permesso di amministratore: rispondi Sì.")
	return elevateAndWait(in, args)
}

func elevateAndWait(in *bufio.Reader, args []string) bool {
	if err := runElevated(args); err != nil {
		fmt.Println("ERRORE:", err)
		pause(in)
		return true
	}
	fmt.Println("Operazione avviata nella finestra di amministratore.")
	time.Sleep(2 * time.Second)
	return true
}

// setupInstall gira come amministratore: copia l'eseguibile (e ffmpeg) in
// Program Files e installa il servizio da lì.
func setupInstall(dataDir, listen string, update bool) error {
	in := bufio.NewReader(os.Stdin)
	defer pause(in)
	if !isAdmin() {
		return errors.New("servono i permessi di amministratore")
	}
	src, _ := os.Executable()
	dir := installDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, "webcam-manager.exe")

	if serviceState() != "assente" {
		fmt.Println("Arresto del servizio attuale...")
		_ = controlService(false)
		for i := 0; i < 30 && serviceState() == "in esecuzione"; i++ {
			time.Sleep(500 * time.Millisecond)
		}
		if !update {
			if err := uninstallService(); err != nil {
				return err
			}
		}
	}
	if !strings.EqualFold(src, dst) {
		fmt.Println("Copia del programma in", dir)
		_ = os.Remove(dst + ".old")
		_ = os.Rename(dst, dst+".old")
		if err := copyFile(src, dst); err != nil {
			return err
		}
		if ff, err := ffmpegPath(); err == nil && !strings.EqualFold(filepath.Dir(ff), dir) {
			_ = copyFile(ff, filepath.Join(dir, "ffmpeg.exe"))
		}
	}
	if update {
		fmt.Println("Riavvio del servizio aggiornato...")
		if err := controlService(true); err != nil {
			return err
		}
	} else {
		args := []string{"install", "-data", dataDir}
		if listen != "" {
			args = append(args, "-listen", listen)
		}
		if adminPasswordFlag != "" {
			args = append(args, "-admin-password", adminPasswordFlag)
		}
		if locationFlag != "" {
			args = append(args, "-location", locationFlag)
		}
		if altitudeFlag >= 0 {
			args = append(args, "-altitude", strconv.Itoa(altitudeFlag))
		}
		cmd := exec.Command(dst, args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("installazione del servizio non riuscita: %w", err)
		}
	}
	fmt.Println("\nFATTO. Webcam Manager è installato in", dir)
	fmt.Println("e partirà da solo a ogni accensione del PC.")
	time.Sleep(3 * time.Second) // lascia partire il pannello
	openBrowser(panelURL(dataDir))
	return nil
}

func startAndOpen(dataDir string) error {
	in := bufio.NewReader(os.Stdin)
	defer pause(in)
	if err := controlService(true); err != nil {
		return err
	}
	fmt.Println("Servizio avviato.")
	time.Sleep(3 * time.Second)
	openBrowser(panelURL(dataDir))
	return nil
}

func portBusy(url string) bool {
	port := url[strings.LastIndex(url, ":")+1:]
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// launchedByDoubleClick: la console è stata creata apposta per questo
// processo (doppio clic da Esplora file) e non ci sono altri programmi collegati.
func launchedByDoubleClick() bool {
	var ids [4]uint32
	n, err := getConsoleProcessList(ids[:])
	return err == nil && n <= 1
}

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

func getConsoleProcessList(ids []uint32) (uint32, error) {
	r, _, err := procGetConsoleProcessList.Call(uintptr(unsafePointer(&ids[0])), uintptr(len(ids)))
	if r == 0 {
		return 0, err
	}
	return uint32(r), nil
}

func unsafePointer(p *uint32) unsafe.Pointer { return unsafe.Pointer(p) }
