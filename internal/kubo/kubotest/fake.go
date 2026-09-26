// Package kubotest is a Kubo stand-in for tests: it records the calls
// and answers like the real one (its shapes checked against Kubo 0.43 on
// 2026-09-25: key/import with a PKCS#8 PEM, add as one JSON line per
// entry with the root last, name/publish, pin/rm).
package kubotest

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/livid/exe-planet/internal/kubo"
)

// Fake is one fake Kubo; read its fields after the calls under test.
type Fake struct {
	Server *httptest.Server
	mu     sync.Mutex
	Keys   map[string]string // name -> ipns name
	Adds   []string          // each add's part names, space-joined
	Pins   map[string]bool
	Pubs   []string          // key=cid
	Fail   map[string]string // op -> message that op fails with
}

// New starts a fake with only the self key; it closes with the test.
func New(t testing.TB) *Fake {
	f := &Fake{Keys: map[string]string{"self": "k51self"}, Pins: map[string]bool{}, Fail: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v0/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Version":"0.43.0"}`))
	})
	mux.HandleFunc("POST /api/v0/key/list", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var parts []string
		for n, id := range f.Keys {
			parts = append(parts, `{"Name":"`+n+`","Id":"`+id+`"}`)
		}
		w.Write([]byte(`{"Keys":[` + strings.Join(parts, ",") + `]}`))
	})
	mux.HandleFunc("POST /api/v0/key/import", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		name := r.URL.Query().Get("arg")
		if msg := f.Fail["key/import"]; msg != "" {
			http.Error(w, `{"Message":"`+msg+`","Code":0}`, 500)
			return
		}
		if _, ok := f.Keys[name]; ok {
			http.Error(w, `{"Message":"key with name '`+name+`' already exists","Code":0}`, 500)
			return
		}
		if r.URL.Query().Get("format") != "pem-pkcs8-cleartext" {
			http.Error(w, `{"Message":"bad format"}`, 500)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		p, _ := mr.NextPart()
		b, _ := io.ReadAll(p)
		block, _ := pem.Decode(b)
		if block == nil {
			http.Error(w, `{"Message":"not a pem"}`, 500)
			return
		}
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		priv, ok := k.(ed25519.PrivateKey)
		if err != nil || !ok {
			http.Error(w, `{"Message":"not an ed25519 pkcs8 key"}`, 500)
			return
		}
		id := kubo.IPNSName(priv.Public().(ed25519.PublicKey))
		f.Keys[name] = id
		w.Write([]byte(`{"Name":"` + name + `","Id":"` + id + `"}`))
	})
	mux.HandleFunc("POST /api/v0/key/rm", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.Keys, r.URL.Query().Get("arg"))
		w.Write([]byte(`{"Keys":[]}`))
	})
	mux.HandleFunc("POST /api/v0/add", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if msg := f.Fail["add"]; msg != "" {
			http.Error(w, `{"Message":"`+msg+`"}`, 500)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var names []string
		var root string
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			// the whole path, as Kubo reads it (Part.FileName would base it)
			_, params, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
			name, _ := url.PathUnescape(params["filename"])
			names = append(names, name)
			if p.Header.Get("Content-Type") == "application/x-directory" && !strings.Contains(name, "/") {
				root = name
			}
			io.Copy(io.Discard, p)
		}
		f.Adds = append(f.Adds, strings.Join(names, " "))
		cid := "bafyadd" + strings.Repeat("x", len(f.Adds))
		f.Pins[cid] = true
		for _, n := range names {
			if n != root {
				w.Write([]byte(`{"Name":"` + n + `","Hash":"bafyfile","Size":"1"}` + "\n"))
			}
		}
		w.Write([]byte(`{"Name":"` + root + `","Hash":"` + cid + `","Size":"9"}` + "\n"))
	})
	mux.HandleFunc("POST /api/v0/name/publish", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if msg := f.Fail["name/publish"]; msg != "" {
			http.Error(w, `{"Message":"`+msg+`"}`, 500)
			return
		}
		q := r.URL.Query()
		if q.Get("lifetime") != "7200h" || q.Get("allow-offline") != "true" {
			http.Error(w, `{"Message":"unexpected options"}`, 500)
			return
		}
		key := q.Get("key")
		id, ok := f.Keys[key]
		if !ok {
			http.Error(w, `{"Message":"no key by the name of '`+key+`' was found","Code":0}`, 500)
			return
		}
		f.Pubs = append(f.Pubs, key+"="+strings.TrimPrefix(q.Get("arg"), "/ipfs/"))
		w.Write([]byte(`{"Name":"` + id + `","Value":"` + q.Get("arg") + `"}`))
	})
	mux.HandleFunc("POST /api/v0/pin/rm", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		cid := r.URL.Query().Get("arg")
		if !f.Pins[cid] {
			http.Error(w, `{"Message":"not pinned or pinned indirectly","Code":0}`, 500)
			return
		}
		delete(f.Pins, cid)
		w.Write([]byte(`{"Pins":["` + cid + `"]}`))
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

// URL is the fake's RPC address.
func (f *Fake) URL() string { return f.Server.URL }

// Snapshot copies the counters under the lock.
func (f *Fake) Snapshot() (keys map[string]string, adds, pubs []string, pins map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys = map[string]string{}
	for k, v := range f.Keys {
		keys[k] = v
	}
	pins = map[string]bool{}
	for k, v := range f.Pins {
		pins[k] = v
	}
	return keys, append([]string(nil), f.Adds...), append([]string(nil), f.Pubs...), pins
}
