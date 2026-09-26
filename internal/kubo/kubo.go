// Package kubo is the slice of Kubo's RPC API exe-planet publishes with:
// keys, a directory add, an IPNS publish and a pin removal. Kubo 0.43
// answers key/import but not key/export, which is why the daemon makes
// and keeps every key itself and only lends Kubo a copy.
package kubo

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Client talks to one Kubo RPC endpoint, such as http://127.0.0.1:5001.
type Client struct {
	URL  string
	HTTP *http.Client
}

// Key is a keystore entry: the name given at import and the IPNS name.
type Key struct {
	Name string `json:"Name"`
	ID   string `json:"Id"`
}

// Published is what name/publish answers.
type Published struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// New makes a client; an empty URL means no Kubo on this node.
func New(rawURL string) *Client {
	if rawURL == "" {
		return nil
	}
	return &Client{URL: strings.TrimRight(rawURL, "/"), HTTP: &http.Client{}}
}

// Version answers Kubo's version string, or an error when it is not there.
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"Version"`
	}
	if err := c.call(ctx, "version", nil, nil, "", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// Keys lists the keystore.
func (c *Client) Keys(ctx context.Context) ([]Key, error) {
	var v struct {
		Keys []Key `json:"Keys"`
	}
	if err := c.call(ctx, "key/list", url.Values{"ipns-base": {"base36"}}, nil, "", &v); err != nil {
		return nil, err
	}
	return v.Keys, nil
}

// ImportKey lends Kubo a PKCS#8 PEM private key under name and answers
// the key's IPNS name.
func (c *Client) ImportKey(ctx context.Context, name string, pemBytes []byte) (Key, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("key", "ipns.key")
	if err != nil {
		return Key{}, err
	}
	fw.Write(pemBytes)
	mw.Close()
	var k Key
	q := url.Values{"arg": {name}, "format": {"pem-pkcs8-cleartext"}, "ipns-base": {"base36"}}
	if err := c.call(ctx, "key/import", q, &buf, mw.FormDataContentType(), &k); err != nil {
		return Key{}, err
	}
	return k, nil
}

// RemoveKey drops a key from the keystore.
func (c *Client) RemoveKey(ctx context.Context, name string) error {
	return c.call(ctx, "key/rm", url.Values{"arg": {name}}, nil, "", nil)
}

// AddDir adds a folder as a pinned CIDv1 directory and answers its CID:
// one multipart part per directory (application/x-directory) and per
// file, named by their path under the folder's own name, as Kubo's own
// client sends them. The answer is one JSON line per entry, the root last.
func (c *Client) AddDir(ctx context.Context, dir string) (string, error) {
	root := filepath.Base(dir)
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != dir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "httpd.conf") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var werr error
		for _, p := range paths {
			rel, _ := filepath.Rel(dir, p)
			name := root
			if rel != "." {
				name = root + "/" + filepath.ToSlash(rel)
			}
			fi, err := os.Stat(p)
			if err != nil {
				werr = err
				break
			}
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", `form-data; name="file"; filename="`+url.PathEscape(name)+`"`)
			if fi.IsDir() {
				h.Set("Content-Type", "application/x-directory")
				if _, err := mw.CreatePart(h); err != nil {
					werr = err
					break
				}
				continue
			}
			h.Set("Content-Type", "application/octet-stream")
			part, err := mw.CreatePart(h)
			if err != nil {
				werr = err
				break
			}
			f, err := os.Open(p)
			if err != nil {
				werr = err
				break
			}
			_, err = io.Copy(part, f)
			f.Close()
			if err != nil {
				werr = err
				break
			}
		}
		if werr == nil {
			werr = mw.Close()
		}
		pw.CloseWithError(werr)
	}()
	q := url.Values{"recursive": {"true"}, "cid-version": {"1"}, "pin": {"true"}, "quieter": {"true"}}
	resp, err := c.do(ctx, "add", q, pr, mw.FormDataContentType())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var cid string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e struct {
			Name string `json:"Name"`
			Hash string `json:"Hash"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Name == root {
			cid = e.Hash
		}
	}
	if cid == "" {
		return "", errors.New("kubo add answered no root entry")
	}
	return cid, nil
}

// Publish points the key's IPNS name at a CID for the lifetime given
// (Planet uses 7200h), even with no peers to hand the record to.
func (c *Client) Publish(ctx context.Context, key, cid, lifetime string) (Published, error) {
	var p Published
	q := url.Values{"arg": {"/ipfs/" + cid}, "key": {key}, "lifetime": {lifetime}, "allow-offline": {"true"}, "ipns-base": {"base36"}}
	if err := c.call(ctx, "name/publish", q, nil, "", &p); err != nil {
		return Published{}, err
	}
	return p, nil
}

// Unpin lets a CID go; a CID that is not pinned is not an error.
func (c *Client) Unpin(ctx context.Context, cid string) error {
	err := c.call(ctx, "pin/rm", url.Values{"arg": {cid}}, nil, "", nil)
	if err != nil && strings.Contains(err.Error(), "not pinned") {
		return nil
	}
	return err
}

func (c *Client) call(ctx context.Context, path string, q url.Values, body io.Reader, ctype string, out any) error {
	resp, err := c.do(ctx, path, q, body, ctype)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) do(ctx context.Context, path string, q url.Values, body io.Reader, ctype string) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("kubo is not configured on this node")
	}
	u := c.URL + "/api/v0/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", u, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kubo %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		var e struct {
			Message string `json:"Message"`
		}
		if json.Unmarshal(b, &e) == nil && e.Message != "" {
			return nil, fmt.Errorf("kubo %s: %s", path, e.Message)
		}
		return nil, fmt.Errorf("kubo %s: %s: %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

// IPNSName is the IPNS name of an ed25519 public key, as Kubo prints it:
// the libp2p protobuf PublicKey (type 1, the 32 bytes) under an identity
// multihash, as a CIDv1 of codec libp2p-key, in base36. Kubo's answer for
// an imported key matched this on 2026-09-25.
func IPNSName(pub ed25519.PublicKey) string {
	pb := append([]byte{0x08, 0x01, 0x12, 0x20}, pub...)
	mh := append([]byte{0x00, byte(len(pb))}, pb...)
	cid := append([]byte{0x01, 0x72}, mh...)
	return "k" + new(big.Int).SetBytes(cid).Text(36)
}
