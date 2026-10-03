// Webcam Manager: acquisisce immagini da telecamere IP, aggiunge
// sovrimpressioni (scritte, loghi, data/ora, dati live) e le pubblica sui
// siti via FTP/FTPS/SFTP, insieme a un mini-sito con lo storico navigabile.
// Tutto è contenuto in un unico eseguibile con pannello web integrato.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var version = "dev" // impostata in fase di build con -ldflags "-X main.version=..."

const credits = "made by Paoli Luca 2026 · paoli.lu@gmail.com"

// App raccoglie tutti i componenti dell'applicazione.
type App struct {
	dataDir  string
	store    *Store
	data     *DataManager
	renderer *Renderer
	archive  *Archive
	tl       *Timelapse
	sync     *SyncState
	sched    *Scheduler
	live     *LiveHub
	updater  *Updater
	alerter  *Alerter
	listen   string
	restartc chan struct{}

	reloadMu sync.Mutex
	srv      *http.Server
}

func (a *App) logoDir() string { return filepath.Join(a.dataDir, "logos") }

// Reload applica una nuova configurazione senza riavviare il programma.
func (a *App) Reload() {
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	cfg := a.store.Get()
	a.data.Restart(cfg.DataSources)
	a.sched.Restart()
}

// NewApp apre (o crea) la cartella dati.
func NewApp(dataDir, listen string) (*App, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("cartella dati %s: %w", dataDir, err)
	}
	store, created, err := LoadStore(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return nil, err
	}
	if created {
		cfg := store.Get()
		log.Printf("creata una nuova configurazione in %s", filepath.Join(dataDir, "config.json"))
		log.Printf("accesso al pannello: utente %q, password %q (cambiala in Impostazioni)", cfg.AdminUser, cfg.AdminPassword)
	}
	archive, err := NewArchive(filepath.Join(dataDir, "archive"))
	if err != nil {
		return nil, err
	}
	a := &App{dataDir: dataDir, store: store, archive: archive, listen: listen}
	a.data = NewDataManager()
	wuGlobalKey = func() string { return store.Get().WUApiKey }
	a.renderer = NewRenderer(a.logoDir(), a.data)
	a.renderer.store = store
	a.sync = LoadSyncState(filepath.Join(dataDir, "history-sync.json"))
	a.tl = NewTimelapse(filepath.Join(dataDir, "timelapse"))
	a.sched = NewScheduler(store, a.renderer, archive, a.tl, a.sync, dataDir)
	a.live = NewLiveHub()
	a.restartc = make(chan struct{}, 1)
	a.updater = NewUpdater(store, a.RequestRestart)
	a.alerter = NewAlerter(a)
	return a, nil
}

// errRestart indica che il programma deve ripartire (es. dopo un aggiornamento).
var errRestart = errors.New("riavvio richiesto")

// RequestRestart fa terminare Run con errRestart.
func (a *App) RequestRestart() {
	select {
	case a.restartc <- struct{}{}:
	default:
	}
}

// Run avvia il lavoro e il pannello web, e si ferma quando ctx viene annullato.
func (a *App) Run(ctx context.Context) error {
	a.Reload()
	addr := a.listen
	if addr == "" {
		addr = a.store.Get().Listen
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("impossibile aprire %s (porta già in uso?): %w", addr, err)
	}
	a.srv = &http.Server{Handler: (&Server{app: a}).Handler(), ReadHeaderTimeout: 15 * time.Second}
	log.Printf("Webcam Manager %s (%s) — pannello su http://%s", version, credits, displayAddr(ln.Addr()))
	uctx, ucancel := context.WithCancel(ctx)
	defer ucancel()
	go a.updater.Loop(uctx)
	go a.alerter.Loop(uctx)
	errc := make(chan error, 1)
	go func() { errc <- a.srv.Serve(ln) }()
	var result error
	select {
	case <-ctx.Done():
	case <-a.restartc:
		result = errRestart
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.srv.Shutdown(shutdown)
	return result
}

func displayAddr(addr net.Addr) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || !tcp.IP.IsUnspecified() {
		return addr.String()
	}
	return fmt.Sprintf("localhost:%d", tcp.Port)
}

// defaultDataDir: se accanto all'eseguibile esiste già una cartella "data"
// (modalità portatile) si usa quella, altrimenti la cartella di sistema.
func defaultDataDir() string {
	if exe, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(exe), "data")
		if st, err := os.Stat(local); err == nil && st.IsDir() {
			return local
		}
	}
	return systemDataDir()
}

func usage() {
	fmt.Fprintf(os.Stderr, `Webcam Manager %s — %s

Uso:
  webcam-manager [opzioni]          avvia il programma (pannello web)
  webcam-manager install [opzioni]  installa come servizio con avvio automatico
  webcam-manager uninstall          rimuove il servizio (i dati restano)
  webcam-manager start | stop       avvia / ferma il servizio
  webcam-manager version            mostra la versione

Opzioni:
`, version, credits)
	flag.PrintDefaults()
}

func main() {
	cmd := ""
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("webcam-manager", flag.ExitOnError)
	dataDir := fs.String("data", defaultDataDir(), "cartella di configurazione, loghi e storico")
	listen := fs.String("listen", "", "indirizzo del pannello (es. :8080); vuoto = quello in configurazione")
	fs.StringVar(&adminPasswordFlag, "admin-password", "", "con install: imposta la password del pannello")
	fs.StringVar(&locationFlag, "location", "", "con install: nome della località (es. \"Camping Coggiolo Sant'Anna Pelago (MO)\")")
	fs.IntVar(&altitudeFlag, "altitude", -1, "con install: altitudine in metri s.l.m.")
	fs.Usage = usage
	flag.CommandLine = fs
	_ = fs.Parse(args)

	var err error
	switch cmd {
	case "":
		err = run(*dataDir, *listen)
	case "install":
		err = installService(*dataDir, *listen)
	case "uninstall":
		err = uninstallService()
	case "start":
		err = controlService(true)
	case "stop":
		err = controlService(false)
	case "version":
		fmt.Println(version, "-", credits)
	case "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
}

func run(dataDir, listen string) error {
	if isService() {
		return runService(dataDir, listen)
	}
	app, err := NewApp(dataDir, listen)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = app.Run(ctx)
	if errors.Is(err, errRestart) {
		if os.Getenv("INVOCATION_ID") != "" {
			os.Exit(3) // sotto systemd: ci pensa Restart=always
		}
		return relaunch()
	}
	return err
}

// relaunch avvia la nuova versione dell'eseguibile con gli stessi argomenti.
func relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Start()
}
