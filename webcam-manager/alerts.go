package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AlertSettings configura l'invio di email quando qualcosa non funziona.
type AlertSettings struct {
	Enabled        bool   `json:"enabled"`
	SMTPHost       string `json:"smtp_host"`       // es. smtp.gmail.com
	SMTPPort       int    `json:"smtp_port"`       // 587 (STARTTLS) o 465 (SSL)
	Security       string `json:"security"`        // "starttls" | "tls" | "none"
	Username       string `json:"username"`        //
	Password       string `json:"password"`        // per Gmail: "password per le app"
	From           string `json:"from"`            // mittente
	To             string `json:"to"`              // destinatari separati da virgola
	AfterMinutes   int    `json:"after_minutes"`   // avvisa se l'errore dura da almeno N minuti
	RepeatHours    int    `json:"repeat_hours"`    // ripeti l'avviso ogni N ore se persiste (0 = mai)
	NotifyRecovery bool   `json:"notify_recovery"` // avvisa anche quando il problema si risolve
}

// Alerter controlla periodicamente lo stato e invia le email.
type Alerter struct {
	app *App

	mu     sync.Mutex
	active map[string]*alertState // problemi già segnalati
}

type alertState struct {
	desc     string
	lastSent time.Time
}

type problem struct {
	key, desc string
	since     time.Time
}

func NewAlerter(app *App) *Alerter {
	return &Alerter{app: app, active: map[string]*alertState{}}
}

func (a *Alerter) Loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		protect("notifiche", func() { a.check(time.Now()) })
	}
}

// problems raccoglie gli errori in corso da più di AfterMinutes.
func (a *Alerter) problems(cfg Config, now time.Time) []problem {
	limit := time.Duration(cfg.Alerts.AfterMinutes) * time.Minute
	cams, pubs, sites := a.app.sched.Status()
	names := map[string]string{}
	var out []problem
	for _, c := range cfg.Cameras {
		names[c.ID] = c.Name
		st, ok := cams[c.ID]
		if !c.Enabled || !ok || st.OutOfHours || st.FailingSince.IsZero() || now.Sub(st.FailingSince) < limit {
			continue
		}
		out = append(out, problem{"cam|" + c.ID, fmt.Sprintf("Webcam \"%s\" non raggiungibile: %s", c.Name, st.LastError), st.FailingSince})
	}
	siteNames := map[string]string{}
	for _, s := range cfg.Sites {
		if s.Enabled {
			siteNames[s.ID] = s.Name
		}
	}
	for _, p := range pubs {
		name, ok := siteNames[p.SiteID]
		if !ok || p.FailingSince.IsZero() || now.Sub(p.FailingSince) < limit {
			continue
		}
		out = append(out, problem{"pub|" + p.SiteID + "|" + p.Filename, fmt.Sprintf("Pubblicazione di %s su \"%s\" non riuscita: %s", p.Filename, name, p.LastError), p.FailingSince})
	}
	for id, s := range sites {
		name, ok := siteNames[id]
		if !ok || s.FailingSince.IsZero() || now.Sub(s.FailingSince) < limit {
			continue
		}
		out = append(out, problem{"hist|" + id, fmt.Sprintf("Storico su \"%s\" non aggiornato: %s", name, s.HistoryError), s.FailingSince})
	}
	a.app.sched.mu.Lock()
	low, free := a.app.sched.diskLow, a.app.sched.diskFree
	a.app.sched.mu.Unlock()
	if low {
		out = append(out, problem{"disk", fmt.Sprintf("Disco quasi pieno (%s liberi): lo storico locale è sospeso. Riduci la conservazione dello storico o libera spazio.", humanBytes(free)), now})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

func (a *Alerter) check(now time.Time) {
	cfg := a.app.store.Get()
	if !cfg.Alerts.Enabled {
		return
	}
	current := a.problems(cfg, now)
	a.mu.Lock()
	var newOrRepeat, resolved []string
	seen := map[string]bool{}
	for _, p := range current {
		seen[p.key] = true
		st := a.active[p.key]
		repeat := cfg.Alerts.RepeatHours > 0 && st != nil && now.Sub(st.lastSent) >= time.Duration(cfg.Alerts.RepeatHours)*time.Hour
		if st == nil || repeat {
			newOrRepeat = append(newOrRepeat, fmt.Sprintf("• %s (dalle %s)", p.desc, p.since.Format("02/01 15:04")))
			a.active[p.key] = &alertState{desc: p.desc, lastSent: now}
		}
	}
	for k, st := range a.active {
		if !seen[k] {
			resolved = append(resolved, "• "+st.desc)
			delete(a.active, k)
		}
	}
	a.mu.Unlock()

	where := hostname()
	if cfg.Location.Name != "" {
		where = cfg.Location.Name + " (" + hostname() + ")"
	}
	if len(newOrRepeat) > 0 {
		body := "Su " + where + " ci sono problemi:\n\n" + strings.Join(newOrRepeat, "\n") +
			"\n\nApri il pannello di Webcam Manager per i dettagli."
		a.send(cfg.Alerts, fmt.Sprintf("[Webcam Manager] %d problemi su %s", len(newOrRepeat), where), body)
	}
	if len(resolved) > 0 && cfg.Alerts.NotifyRecovery {
		sort.Strings(resolved)
		a.send(cfg.Alerts, "[Webcam Manager] Problemi risolti su "+where, "Su "+where+" sono tornati a funzionare:\n\n"+strings.Join(resolved, "\n"))
	}
}

func (a *Alerter) send(s AlertSettings, subject, body string) {
	if err := SendMail(s, subject, body); err != nil {
		log.Printf("invio email non riuscito: %v", err)
	} else {
		log.Printf("email inviata: %s", subject)
	}
}

// SendMail invia un'email in testo semplice.
func SendMail(s AlertSettings, subject, body string) error {
	if s.SMTPHost == "" || s.To == "" {
		return errors.New("indica server SMTP e destinatari")
	}
	from := s.From
	if from == "" {
		from = s.Username
	}
	var to []string
	for _, t := range strings.Split(s.To, ",") {
		if t = strings.TrimSpace(t); t != "" {
			to = append(to, t)
		}
	}
	port := s.SMTPPort
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(s.SMTPHost, strconv.Itoa(port))
	msg := "From: " + from + "\r\nTo: " + strings.Join(to, ", ") + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) +
		"\r\nDate: " + time.Now().Format(time.RFC1123Z) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" +
		strings.ReplaceAll(body, "\n", "\r\n") + "\r\n"

	dialer := &net.Dialer{Timeout: 20 * time.Second}
	var conn net.Conn
	var err error
	tlsCfg := &tls.Config{ServerName: s.SMTPHost}
	if s.Security == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connessione a %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	c, err := smtp.NewClient(conn, s.SMTPHost)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.Security == "starttls" || s.Security == "" {
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.SMTPHost)); err != nil {
			return fmt.Errorf("accesso SMTP rifiutato: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, t := range to {
		if err := c.Rcpt(t); err != nil {
			return fmt.Errorf("destinatario %s: %w", t, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
