package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SiteConn è una connessione aperta verso un sito. Put scrive un file in un
// percorso relativo alla cartella del sito (con "/" come separatore), creando
// le sottocartelle mancanti. Ogni file viene caricato con un nome temporaneo
// e poi rinominato, così i visitatori non vedono mai un'immagine a metà.
type SiteConn interface {
	Put(rel string, data []byte) error
	Delete(rel string) error
	Close() error
}

// OpenSite si collega al sito e verifica credenziali e cartella.
func OpenSite(ctx context.Context, store *Store, site Site) (SiteConn, error) {
	switch site.Protocol {
	case "folder":
		if err := os.MkdirAll(site.RemoteDir, 0o755); err != nil {
			return nil, err
		}
		probe := filepath.Join(site.RemoteDir, ".webcam-manager-test")
		if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
			return nil, fmt.Errorf("cartella non scrivibile: %w", err)
		}
		_ = os.Remove(probe)
		return folderConn{dir: site.RemoteDir}, nil
	case "ftp", "ftps":
		return openFTP(ctx, site)
	case "sftp":
		return openSFTP(ctx, store, site)
	case "onedrive":
		return openOneDrive(ctx, store, site)
	}
	return nil, fmt.Errorf("protocollo %q sconosciuto", site.Protocol)
}

// TestSite prova davvero la pubblicazione: connessione, accesso alla
// cartella, caricamento di un file di prova (con rinomina, come per le
// immagini) e sua cancellazione. Restituisce i passi riusciti.
func TestSite(ctx context.Context, store *Store, site Site) ([]string, error) {
	var steps []string
	c, err := OpenSite(ctx, store, site)
	if err != nil {
		return steps, err
	}
	defer c.Close()
	steps = append(steps, "connessione e accesso riusciti", "cartella raggiungibile")
	name := "webcam-manager-test.txt"
	if err := c.Put(name, []byte("Test di pubblicazione di Webcam Manager: questo file si può cancellare.\n")); err != nil {
		return steps, fmt.Errorf("caricamento file di prova non riuscito: %w", err)
	}
	steps = append(steps, "caricamento e rinomina di un file di prova riusciti")
	if err := c.Delete(name); err != nil {
		steps = append(steps, "attenzione: impossibile cancellare il file di prova "+name+" ("+err.Error()+")")
	} else {
		steps = append(steps, "file di prova cancellato")
	}
	return steps, nil
}

// ---- cartella locale (es. cartella di un web server sullo stesso PC) ----

type folderConn struct{ dir string }

func (f folderConn) Put(rel string, data []byte) error {
	dst := filepath.Join(f.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func (f folderConn) Delete(rel string) error {
	return os.Remove(filepath.Join(f.dir, filepath.FromSlash(rel)))
}

func (folderConn) Close() error { return nil }

// ---- FTP / FTPS ----

type ftpConn struct {
	c    *ftp.ServerConn
	dirs map[string]bool // cartelle già create in questa sessione
}

func openFTP(ctx context.Context, site Site) (SiteConn, error) {
	addr := net.JoinHostPort(site.Host, strconv.Itoa(site.Port))
	opts := []ftp.DialOption{ftp.DialWithContext(ctx), ftp.DialWithTimeout(30 * time.Second)}
	if site.Protocol == "ftps" {
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{ServerName: site.Host}))
	}
	c, err := ftp.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connessione FTP: %w", err)
	}
	user := site.Username
	if user == "" {
		user = "anonymous"
	}
	if err := c.Login(user, site.Password); err != nil {
		_ = c.Quit()
		return nil, fmt.Errorf("login FTP: %w", err)
	}
	if site.RemoteDir != "" {
		if err := c.ChangeDir(site.RemoteDir); err != nil {
			_ = c.Quit()
			return nil, fmt.Errorf("cartella remota %q: %w", site.RemoteDir, err)
		}
	}
	return &ftpConn{c: c, dirs: map[string]bool{".": true}}, nil
}

func (f *ftpConn) mkdirAll(dir string) {
	if f.dirs[dir] {
		return
	}
	parts := strings.Split(dir, "/")
	for i := range parts {
		d := strings.Join(parts[:i+1], "/")
		if !f.dirs[d] {
			_ = f.c.MakeDir(d) // fallisce se esiste già: va bene
			f.dirs[d] = true
		}
	}
}

func (f *ftpConn) Put(rel string, data []byte) error {
	f.mkdirAll(path.Dir(rel))
	tmp := rel + ".tmp"
	if err := f.c.Stor(tmp, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("upload %s: %w", rel, err)
	}
	if err := f.c.Rename(tmp, rel); err != nil {
		// alcuni server non sovrascrivono con RNTO: cancella e riprova
		_ = f.c.Delete(rel)
		if err := f.c.Rename(tmp, rel); err != nil {
			return fmt.Errorf("rinomina %s: %w", rel, err)
		}
	}
	return nil
}

func (f *ftpConn) Delete(rel string) error { return f.c.Delete(rel) }

func (f *ftpConn) Close() error { return f.c.Quit() }

// ---- SFTP ----

type sftpConn struct {
	ssh *ssh.Client
	sf  *sftp.Client
	dir string
}

func openSFTP(ctx context.Context, store *Store, site Site) (SiteConn, error) {
	cfg := &ssh.ClientConfig{
		User:    site.Username,
		Auth:    []ssh.AuthMethod{ssh.Password(site.Password)},
		Timeout: 30 * time.Second,
		// Trust on first use: la prima impronta viene memorizzata nella
		// configurazione, le successive devono coincidere.
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp := ssh.FingerprintSHA256(key)
			if site.HostKey == "" {
				store.SetHostKey(site.ID, fp)
				return nil
			}
			if fp != site.HostKey {
				return fmt.Errorf("l'impronta SSH del server è cambiata (%s, attesa %s): se è legittimo svuota il campo nel sito", fp, site.HostKey)
			}
			return nil
		},
	}
	addr := net.JoinHostPort(site.Host, strconv.Itoa(site.Port))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connessione SFTP: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("login SFTP: %w", err)
	}
	client := ssh.NewClient(sc, chans, reqs)
	sf, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("SFTP: %w", err)
	}
	dir := site.RemoteDir
	if dir == "" {
		dir = "."
	}
	if st, err := sf.Stat(dir); err != nil || !st.IsDir() {
		sf.Close()
		client.Close()
		return nil, fmt.Errorf("cartella remota %q non trovata", dir)
	}
	return &sftpConn{ssh: client, sf: sf, dir: dir}, nil
}

func (s *sftpConn) Put(rel string, data []byte) error {
	dst := path.Join(s.dir, rel)
	if err := s.sf.MkdirAll(path.Dir(dst)); err != nil {
		return fmt.Errorf("cartella %s: %w", path.Dir(rel), err)
	}
	tmp := dst + ".tmp"
	f, err := s.sf.Create(tmp)
	if err != nil {
		return fmt.Errorf("upload %s: %w", rel, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("upload %s: %w", rel, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("upload %s: %w", rel, err)
	}
	if err := s.sf.PosixRename(tmp, dst); err != nil {
		_ = s.sf.Remove(dst)
		if err := s.sf.Rename(tmp, dst); err != nil {
			return fmt.Errorf("rinomina %s: %w", rel, err)
		}
	}
	return nil
}

func (s *sftpConn) Delete(rel string) error { return s.sf.Remove(path.Join(s.dir, rel)) }

func (s *sftpConn) Close() error {
	s.sf.Close()
	return s.ssh.Close()
}
