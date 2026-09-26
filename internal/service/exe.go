package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Exe is the slice of exe's API a site's expose uses: the routes that
// put a hostname in front of a local backend (POST and DELETE
// /v1/routes, exe 0454005), with exe's own token and zone.
type Exe struct {
	URL    string // http://127.0.0.1:7777
	Token  string // api_token from exe's config.json, may be empty
	Domain string // cloudflare.domain, the zone hostnames are made in
	HTTP   *http.Client
}

// LoadExe reads the token and the zone from exe's config.json. A missing
// file is not an error: the node then has exe without a zone, and expose
// says so when asked.
func LoadExe(rawURL, configPath string) (*Exe, error) {
	if rawURL == "" {
		return nil, nil
	}
	e := &Exe{URL: strings.TrimRight(rawURL, "/"), HTTP: &http.Client{Timeout: 3 * time.Minute}}
	b, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return e, nil
		}
		return nil, fmt.Errorf("exe config: %w", err)
	}
	var cfg struct {
		APIToken   string `json:"api_token"`
		Cloudflare struct {
			Domain string `json:"domain"`
		} `json:"cloudflare"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("exe config: %w", err)
	}
	e.Token, e.Domain = cfg.APIToken, strings.ToLower(strings.TrimSpace(cfg.Cloudflare.Domain))
	return e, nil
}

// AddRoute publishes host to backend through exe: proxy route, DNS record
// and tunnel ingress. exe's warnings (a DNS or ingress step it could not
// finish) come back as strings; the route itself is in place when err is
// nil.
func (e *Exe) AddRoute(ctx context.Context, host, backend string) ([]string, error) {
	if e == nil {
		return nil, errors.New("exe is not configured on this node")
	}
	body, _ := json.Marshal(map[string]string{"host": host, "backend": backend})
	var res struct {
		Warnings []string `json:"warnings"`
	}
	if err := e.call(ctx, "POST", "/v1/routes", bytes.NewReader(body), &res); err != nil {
		return nil, err
	}
	return res.Warnings, nil
}

// DeleteRoute takes the hostname down again.
func (e *Exe) DeleteRoute(ctx context.Context, host string) error {
	if e == nil {
		return errors.New("exe is not configured on this node")
	}
	return e.call(ctx, "DELETE", "/v1/routes/"+url.PathEscape(host), nil, nil)
}

// HubPublish has exe sign one op with the node's own key and send it to
// a hub (POST /v1/hub/publish): the node is the hub's admin, so this is
// how a site's key is invited past the gate (PLAN.md, Replies from the
// hub).
func (e *Exe) HubPublish(ctx context.Context, hub, typ string, body any) error {
	if e == nil {
		return errors.New("exe is not configured on this node")
	}
	b, _ := json.Marshal(map[string]any{"hub": hub, "type": typ, "body": body})
	return e.call(ctx, "POST", "/v1/hub/publish", bytes.NewReader(b), nil)
}

func (e *Exe) call(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, e.URL+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if e.Token != "" {
		req.Header.Set("Authorization", "Bearer "+e.Token)
	}
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("exe: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var ae struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &ae) == nil && ae.Error != "" {
			return fmt.Errorf("exe: %s", ae.Error)
		}
		return fmt.Errorf("exe: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}
