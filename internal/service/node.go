package service

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/livid/exe-planet/internal/kubo"
	"github.com/livid/exe-planet/internal/site"
)

// Node is a site's publishing on this node, kept beside its build in
// <state>/<id>/node.json: whether it is exposed through exe and under
// which name, whether it goes to IPFS, and what was last published. It
// is per node on purpose; the Workspace carries the site, not where it
// is served from.
type Node struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Expose        bool      `json:"expose"`
	Subdomain     string    `json:"subdomain,omitempty"`
	Hostname      string    `json:"hostname,omitempty"`
	RouteWarnings []string  `json:"route_warnings,omitempty"`
	IPFS          bool      `json:"ipfs"`
	CID           string    `json:"cid,omitempty"`
	Published     time.Time `json:"published,omitempty"`
	PublishedHash string    `json:"published_hash,omitempty"`
	BuiltHash     string    `json:"built_hash,omitempty"`
	PublishError  string    `json:"publish_error,omitempty"`
}

// NodeInfo is a Node as the API answers it, with what the node derives.
type NodeInfo struct {
	Node
	URL        string `json:"url,omitempty"`
	IPNS       string `json:"ipns,omitempty"`
	IPNSURL    string `json:"ipns_url,omitempty"`
	CIDURL     string `json:"cid_url,omitempty"`
	Publishing bool   `json:"publishing"`
	Key        bool   `json:"key"`
	Domain     string `json:"domain,omitempty"`
	Exe        bool   `json:"exe"`
	Kubo       bool   `json:"kubo"`
}

// NodeUpdate carries what PUT may change; nil leaves a field alone.
type NodeUpdate struct {
	Expose    *bool   `json:"expose"`
	Subdomain *string `json:"subdomain"`
	IPFS      *bool   `json:"ipfs"`
}

// Caps is what this node can publish with.
type Caps struct {
	Domain      string `json:"domain,omitempty"`
	Exe         bool   `json:"exe"`
	Kubo        bool   `json:"kubo"`
	KuboVersion string `json:"kubo_version,omitempty"`
	KuboError   string `json:"kubo_error,omitempty"`
	Backend     string `json:"backend,omitempty"`
}

// The IPNS lifetime Planet publishes with; Kubo republishes on its own.
const ipnsLifetime = "7200h"

type pubState struct {
	running bool
	again   bool
	force   bool
}

func (s *Service) nodePath(id string) string { return filepath.Join(s.State, id, "node.json") }
func (s *Service) keyPath(id string) string  { return filepath.Join(s.State, id, "ipns.key") }
func keyName(id string) string               { return "exe-planet-" + id }

// loadNode reads a site's node file; none yet is an empty Node.
func (s *Service) loadNode(id string) Node {
	var n Node
	b, err := os.ReadFile(s.nodePath(id))
	if err == nil {
		json.Unmarshal(b, &n)
	}
	n.ID = id
	return n
}

func (s *Service) saveNode(n Node) error {
	if err := os.MkdirAll(filepath.Dir(s.nodePath(n.ID)), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(n, "", "  ")
	tmp := s.nodePath(n.ID) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.nodePath(n.ID))
}

// loadHosts reads every node file at start, so exposed sites answer
// their hostnames from the first request.
func (s *Service) loadHosts() {
	entries, err := os.ReadDir(s.State)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n := s.loadNode(e.Name())
		if n.Expose && n.Hostname != "" {
			s.hosts[n.Hostname] = e.Name()
		}
	}
}

// HostDir is the built folder a public hostname is served from, or ""
// when the host is not one of ours.
func (s *Service) HostDir(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	s.mu.Lock()
	id := s.hosts[host]
	s.mu.Unlock()
	if id == "" {
		return ""
	}
	dir := s.PublicDir(id)
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}

// Caps says what the node has to publish with; Kubo is asked, briefly.
func (s *Service) Caps(ctx context.Context) Caps {
	c := Caps{Exe: s.Exe != nil, Kubo: s.Kubo != nil, Backend: s.Backend}
	if s.Exe != nil {
		c.Domain = s.Exe.Domain
	}
	if s.Kubo != nil {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if v, err := s.Kubo.Version(ctx); err != nil {
			c.KuboError = err.Error()
		} else {
			c.KuboVersion = v
		}
	}
	return c
}

// Node answers a site's publishing on this node.
func (s *Service) Node(slug string) (NodeInfo, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return NodeInfo{}, err
	}
	return s.info(st), nil
}

func (s *Service) info(st *site.Site) NodeInfo {
	n := s.loadNode(st.ID)
	n.Slug = st.Slug
	in := NodeInfo{Node: n, IPNS: st.IPNS, Exe: s.Exe != nil, Kubo: s.Kubo != nil}
	if s.Exe != nil {
		in.Domain = s.Exe.Domain
	}
	if n.Expose && n.Hostname != "" {
		in.URL = "https://" + n.Hostname + "/"
	}
	if st.IPNS != "" {
		in.IPNSURL = "https://eth.sucks/ipns/" + st.IPNS + "/"
	}
	if n.CID != "" {
		in.CIDURL = "https://eth.sucks/ipfs/" + n.CID + "/"
	}
	if _, err := os.Stat(s.keyPath(st.ID)); err == nil {
		in.Key = true
	}
	s.mu.Lock()
	if p := s.publishing[st.Slug]; p != nil {
		in.Publishing = p.running
	}
	s.mu.Unlock()
	return in
}

// SetNode changes a site's publishing: a route through exe added or
// removed, a key made and lent to Kubo, and a publish or rebuild set off
// when the change calls for one.
func (s *Service) SetNode(ctx context.Context, slug string, u NodeUpdate) (NodeInfo, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return NodeInfo{}, err
	}
	n := s.loadNode(st.ID)
	n.Slug = slug
	if u.Subdomain != nil {
		sub := strings.ToLower(strings.TrimSpace(*u.Subdomain))
		if sub != "" && site.ValidSiteSlug(sub) != nil {
			return NodeInfo{}, fmt.Errorf("%w: a subdomain is lowercase letters, digits and hyphens", site.ErrInvalid)
		}
		n.Subdomain = sub
	}
	expose := n.Expose
	if u.Expose != nil {
		expose = *u.Expose
	}
	want := ""
	if expose {
		if s.Exe == nil {
			return NodeInfo{}, fmt.Errorf("%w: exe is not configured on this node", site.ErrInvalid)
		}
		if s.Exe.Domain == "" {
			return NodeInfo{}, fmt.Errorf("%w: exe has no cloudflare.domain, so there is no zone to expose in", site.ErrInvalid)
		}
		sub := n.Subdomain
		if sub == "" {
			sub = slug
		}
		want = sub + "." + s.Exe.Domain
	}
	if want != n.Hostname {
		var warnings []string
		if n.Hostname != "" {
			if err := s.Exe.DeleteRoute(ctx, n.Hostname); err != nil {
				warnings = append(warnings, "remove "+n.Hostname+": "+err.Error())
			}
			s.mu.Lock()
			delete(s.hosts, n.Hostname)
			s.mu.Unlock()
			n.Hostname = ""
		}
		if want != "" {
			w, err := s.Exe.AddRoute(ctx, want, s.Backend)
			if err != nil {
				n.Expose = false
				n.RouteWarnings = warnings
				s.saveNode(n)
				return NodeInfo{}, err
			}
			warnings = append(warnings, w...)
			n.Hostname = want
			s.mu.Lock()
			s.hosts[want] = st.ID
			s.mu.Unlock()
		}
		n.RouteWarnings = warnings
		s.Logf("planet: %s exposed at %q", slug, want)
	}
	n.Expose = expose

	rebuild, publish := false, false
	if u.IPFS != nil && *u.IPFS != n.IPFS {
		if *u.IPFS {
			if s.Kubo == nil {
				return NodeInfo{}, fmt.Errorf("%w: kubo is not configured on this node", site.ErrInvalid)
			}
			ipns, err := s.ensureKey(ctx, st.ID)
			if err != nil {
				return NodeInfo{}, err
			}
			if st.IPNS != ipns {
				if _, err := s.Store.UpdateSite(slug, site.SiteUpdate{IPNS: &ipns}); err != nil {
					return NodeInfo{}, err
				}
				st.IPNS = ipns
				rebuild = true // the pages carry the name
			} else if n.BuiltHash != "" && n.BuiltHash != n.PublishedHash {
				publish = true
			}
			n.PublishError = ""
		}
		n.IPFS = *u.IPFS
	}
	if err := s.saveNode(n); err != nil {
		return NodeInfo{}, err
	}
	if rebuild {
		if _, err := s.Build(slug); err != nil {
			return NodeInfo{}, err
		}
	} else if publish {
		s.startPublish(slug, false)
	}
	return s.info(st), nil
}

// ensureKey makes the site's ed25519 key once, keeps it as PKCS#8 PEM in
// the node's state, lends Kubo a copy under exe-planet-<id>, and answers
// the IPNS name.
func (s *Service) ensureKey(ctx context.Context, id string) (string, error) {
	path := s.keyPath(id)
	var priv ed25519.PrivateKey
	if b, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(b)
		if block == nil {
			return "", fmt.Errorf("%s is not a PEM key", path)
		}
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		var ok bool
		if priv, ok = k.(ed25519.PrivateKey); !ok {
			return "", fmt.Errorf("%s is not an ed25519 key", path)
		}
	} else {
		_, priv, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return "", err
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return "", err
		}
		s.Logf("planet: made the IPNS key for %s", id)
	}
	ipns := kubo.IPNSName(priv.Public().(ed25519.PublicKey))
	keys, err := s.Kubo.Keys(ctx)
	if err != nil {
		return "", err
	}
	for _, k := range keys {
		if k.Name == keyName(id) {
			if k.ID != ipns {
				return "", fmt.Errorf("kubo holds a different key as %s (%s, ours is %s)", k.Name, k.ID, ipns)
			}
			return ipns, nil
		}
	}
	pemBytes, _ := os.ReadFile(path)
	k, err := s.Kubo.ImportKey(ctx, keyName(id), pemBytes)
	if err != nil {
		return "", err
	}
	if k.ID != ipns {
		return "", fmt.Errorf("kubo named the key %s, we derived %s", k.ID, ipns)
	}
	s.Logf("planet: lent kubo the key for %s as %s", id, ipns)
	return ipns, nil
}

// Publish sends the site's build to IPFS now, changed or not.
func (s *Service) Publish(slug string) (NodeInfo, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return NodeInfo{}, err
	}
	n := s.loadNode(st.ID)
	if !n.IPFS {
		return NodeInfo{}, fmt.Errorf("%w: IPFS is off for this site on this node", site.ErrInvalid)
	}
	if _, err := os.Stat(s.PublicDir(st.ID)); err != nil {
		return NodeInfo{}, fmt.Errorf("%w: the site is not built yet", site.ErrNotFound)
	}
	s.startPublish(slug, true)
	return s.info(st), nil
}

// afterBuild records a build's content hash and publishes when the site
// goes to IPFS and the content moved since the last publish.
func (s *Service) afterBuild(slug, id, hash string) {
	s.nodeMu.Lock()
	n := s.loadNode(id)
	n.Slug = slug
	changed := n.BuiltHash != hash
	n.BuiltHash = hash
	if changed {
		s.saveNode(n)
	}
	s.nodeMu.Unlock()
	if n.IPFS && hash != n.PublishedHash {
		s.startPublish(slug, false)
	}
}

// startPublish runs one publish at a time per site; a request during one
// makes it run once more after.
func (s *Service) startPublish(slug string, force bool) {
	s.mu.Lock()
	p := s.publishing[slug]
	if p == nil {
		p = &pubState{}
		s.publishing[slug] = p
	}
	if p.running {
		p.again = true
		p.force = p.force || force
		s.mu.Unlock()
		return
	}
	p.running, p.force = true, force
	s.mu.Unlock()
	go func() {
		for {
			s.mu.Lock()
			force := p.force
			p.force = false
			s.mu.Unlock()
			s.publishOnce(slug, force)
			s.mu.Lock()
			if !p.again {
				p.running = false
				s.mu.Unlock()
				return
			}
			p.again = false
			s.mu.Unlock()
		}
	}()
}

func (s *Service) publishOnce(slug string, force bool) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return
	}
	s.nodeMu.Lock()
	n := s.loadNode(st.ID)
	s.nodeMu.Unlock()
	if !n.IPFS || s.Kubo == nil {
		return
	}
	if !force && n.BuiltHash != "" && n.BuiltHash == n.PublishedHash {
		return
	}
	hash := n.BuiltHash
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fail := func(err error) {
		s.Logf("planet: publish %s: %v", slug, err)
		s.nodeMu.Lock()
		n := s.loadNode(st.ID)
		n.Slug = slug
		n.PublishError = err.Error()
		s.saveNode(n)
		s.nodeMu.Unlock()
	}
	cid, err := s.Kubo.AddDir(ctx, s.PublicDir(st.ID))
	if err != nil {
		fail(err)
		return
	}
	if _, err := s.Kubo.Publish(ctx, keyName(st.ID), cid, ipnsLifetime); err != nil {
		fail(err)
		return
	}
	s.nodeMu.Lock()
	n = s.loadNode(st.ID)
	n.Slug = slug
	old := n.CID
	n.CID, n.Published, n.PublishedHash, n.PublishError = cid, time.Now(), hash, ""
	s.saveNode(n)
	s.nodeMu.Unlock()
	if old != "" && old != cid {
		if err := s.Kubo.Unpin(ctx, old); err != nil {
			s.Logf("planet: unpin %s: %v", old, err)
		}
	}
	s.Logf("planet: published %s: %s -> %s", slug, st.IPNS, cid)
}

// Retire takes a site off this node's routing when it is deleted or
// archived: the exe route goes, the key stays (a key is never dropped
// without being asked), the node file remembers nothing of the route.
func (s *Service) Retire(ctx context.Context, slug, id string) {
	if id == "" {
		return
	}
	s.nodeMu.Lock()
	n := s.loadNode(id)
	s.nodeMu.Unlock()
	if n.Hostname == "" {
		return
	}
	if s.Exe != nil {
		if err := s.Exe.DeleteRoute(ctx, n.Hostname); err != nil {
			s.Logf("planet: retire %s: %v", n.Hostname, err)
		}
	}
	s.mu.Lock()
	delete(s.hosts, n.Hostname)
	s.mu.Unlock()
	s.nodeMu.Lock()
	n = s.loadNode(id)
	n.Slug, n.Expose, n.Hostname, n.RouteWarnings = slug, false, "", nil
	s.saveNode(n)
	s.nodeMu.Unlock()
}

// Export writes a zip of everything the site is: its Workspace folder as
// <slug>/, and beside it the node's key and node file, which is how a
// site takes its publishing to another node.
func (s *Service) Export(slug string, w io.Writer) error {
	st, err := s.Store.Site(slug)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	add := func(name, file string) error {
		fi, err := os.Stat(file)
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		h.Name = name
		h.Method = zip.Deflate
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		out, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, f)
		return err
	}
	err = filepath.WalkDir(st.Dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(st.Dir, p)
		return add(slug+"/"+filepath.ToSlash(rel), p)
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(s.keyPath(st.ID)); err == nil {
		if err := add(slug+".ipns.key", s.keyPath(st.ID)); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.nodePath(st.ID)); err == nil {
		if err := add(slug+".node.json", s.nodePath(st.ID)); err != nil {
			return err
		}
	}
	return zw.Close()
}

// inputHash is what a build is made from: every file of the site folder
// and of its template, path and bytes in order, and the engine's
// version. Two builds from the same inputs compare equal without a
// manifest file, whatever the pages stamp.
func inputHash(siteDir, templateDir, version string) (string, error) {
	h := sha256.New()
	io.WriteString(h, "exe-planet "+version+"\n")
	for _, dir := range []string{siteDir, templateDir} {
		var paths []string
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		sort.Strings(paths)
		for _, p := range paths {
			rel, _ := filepath.Rel(dir, p)
			io.WriteString(h, filepath.ToSlash(rel))
			h.Write([]byte{0})
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return "", err
			}
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
