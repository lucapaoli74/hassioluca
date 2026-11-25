package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Pubblicazione su OneDrive tramite Microsoft Graph. L'account si collega dal
// pannello con il "device code flow": il programma mostra un codice, l'utente
// lo inserisce su microsoft.com/devicelogin e accede. Il programma conserva
// solo il refresh token, con cui ottiene da sé i token di accesso.

const graphScope = "Files.ReadWrite offline_access"

func oauthBase(tenant string) string {
	if tenant == "" {
		tenant = "consumers"
	}
	return "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0"
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func postForm(ctx context.Context, u string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return json.Unmarshal(data, out)
}

// token di accesso in memoria, per sito
var (
	odMu     sync.Mutex
	odTokens = map[string]struct {
		token   string
		expires time.Time
	}{}
)

func onedriveAccessToken(ctx context.Context, store *Store, site Site) (string, error) {
	odMu.Lock()
	t, ok := odTokens[site.ID]
	odMu.Unlock()
	if ok && time.Until(t.expires) > 2*time.Minute {
		return t.token, nil
	}
	if site.OAuthRefresh == "" {
		return "", errors.New("account Microsoft non collegato: usa “Collega account” nel sito")
	}
	var tr tokenResponse
	err := postForm(ctx, oauthBase(site.OAuthTenant)+"/token", url.Values{
		"client_id":     {site.OAuthClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {site.OAuthRefresh},
		"scope":         {graphScope},
	}, &tr)
	if err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("accesso Microsoft scaduto o revocato, ricollega l'account (%s)", tr.Error)
	}
	if tr.RefreshToken != "" && tr.RefreshToken != site.OAuthRefresh {
		store.SetOAuthRefresh(site.ID, tr.RefreshToken)
	}
	odMu.Lock()
	odTokens[site.ID] = struct {
		token   string
		expires time.Time
	}{tr.AccessToken, time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)}
	odMu.Unlock()
	return tr.AccessToken, nil
}

type onedriveConn struct {
	ctx   context.Context
	token string
	dir   string
}

func openOneDrive(ctx context.Context, store *Store, site Site) (SiteConn, error) {
	tok, err := onedriveAccessToken(ctx, store, site)
	if err != nil {
		return nil, err
	}
	c := &onedriveConn{ctx: ctx, token: tok, dir: strings.Trim(site.RemoteDir, "/")}
	// verifica l'accesso leggendo il drive
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/me/drive", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OneDrive: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OneDrive risponde %s", resp.Status)
	}
	return c, nil
}

func graphPath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// Put carica il file; le cartelle mancanti vengono create da OneDrive.
// OneDrive rende visibile il file solo a caricamento completato.
func (c *onedriveConn) Put(rel string, data []byte) error {
	full := rel
	if c.dir != "" {
		full = c.dir + "/" + rel
	}
	base := "https://graph.microsoft.com/v1.0/me/drive/root:/" + graphPath(full) + ":"
	if len(data) <= 4<<20 {
		return c.do(http.MethodPut, base+"/content", data, "", nil)
	}
	// oltre 4 MB serve una sessione di caricamento
	var sess struct {
		UploadURL string `json:"uploadUrl"`
	}
	body := []byte(`{"item":{"@microsoft.graph.conflictBehavior":"replace"}}`)
	if err := c.do(http.MethodPost, base+"/createUploadSession", body, "application/json", &sess); err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(c.ctx, http.MethodPut, sess.UploadURL, bytes.NewReader(data))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data)))
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("OneDrive %s: %s", rel, resp.Status)
	}
	return nil
}

func (c *onedriveConn) do(method, u string, body []byte, ctype string, out any) error {
	req, err := http.NewRequestWithContext(c.ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return fmt.Errorf("OneDrive: %s %s", resp.Status, e.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *onedriveConn) Delete(rel string) error {
	full := rel
	if c.dir != "" {
		full = c.dir + "/" + rel
	}
	return c.do(http.MethodDelete, "https://graph.microsoft.com/v1.0/me/drive/root:/"+graphPath(full), nil, "", nil)
}

func (c *onedriveConn) Close() error { return nil }

// ---- collegamento dell'account dal pannello ----

// DeviceFlow è un collegamento in corso.
type DeviceFlow struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Message         string `json:"message"`
	Status          string `json:"status"` // "pending" | "ok" | "error"
	Error           string `json:"error,omitempty"`
}

type deviceFlows struct {
	mu    sync.Mutex
	flows map[string]*DeviceFlow
}

var oneDriveFlows = &deviceFlows{flows: map[string]*DeviceFlow{}}

// StartDeviceFlow avvia il collegamento: restituisce l'ID del flusso e il
// codice da mostrare. Al termine il refresh token viene salvato nel sito.
func StartDeviceFlow(store *Store, siteID, clientID, tenant string) (string, *DeviceFlow, error) {
	if clientID == "" {
		return "", nil, errors.New("serve l'ID applicazione (client ID) registrato su Azure")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var dc struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Message         string `json:"message"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := postForm(ctx, oauthBase(tenant)+"/devicecode", url.Values{"client_id": {clientID}, "scope": {graphScope}}, &dc); err != nil {
		return "", nil, err
	}
	if dc.DeviceCode == "" {
		return "", nil, fmt.Errorf("Microsoft: %s %s", dc.Error, dc.ErrorDesc)
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	flow := &DeviceFlow{UserCode: dc.UserCode, VerificationURI: dc.VerificationURI, Message: dc.Message, Status: "pending"}
	oneDriveFlows.mu.Lock()
	oneDriveFlows.flows[id] = flow
	oneDriveFlows.mu.Unlock()

	go func() {
		interval := time.Duration(max(dc.Interval, 5)) * time.Second
		deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
		set := func(status, errMsg string) {
			oneDriveFlows.mu.Lock()
			flow.Status, flow.Error = status, errMsg
			oneDriveFlows.mu.Unlock()
		}
		for time.Now().Before(deadline) {
			time.Sleep(interval)
			var tr tokenResponse
			err := postForm(context.Background(), oauthBase(tenant)+"/token", url.Values{
				"client_id":   {clientID},
				"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
				"device_code": {dc.DeviceCode},
			}, &tr)
			if err != nil {
				continue
			}
			switch tr.Error {
			case "authorization_pending":
				continue
			case "slow_down":
				interval += 5 * time.Second
				continue
			case "":
				store.SetOAuthRefresh(siteID, tr.RefreshToken)
				odMu.Lock()
				delete(odTokens, siteID)
				odMu.Unlock()
				set("ok", "")
				return
			default:
				set("error", tr.ErrorDesc)
				return
			}
		}
		set("error", "tempo scaduto, riprova")
	}()
	return id, flow, nil
}

func GetDeviceFlow(id string) (DeviceFlow, bool) {
	oneDriveFlows.mu.Lock()
	defer oneDriveFlows.mu.Unlock()
	f, ok := oneDriveFlows.flows[id]
	if !ok {
		return DeviceFlow{}, false
	}
	return *f, true
}
