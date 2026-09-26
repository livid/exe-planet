package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/livid/exe-planet/internal/site"
)

// A site on the exe hub (PLAN.md, Replies from the hub). The site's one
// ed25519 key — the IPNS key — is also its hub author: the site signs
// its own profile and one post for every blog post, and the replies
// those posts get show under the post on the blog. The node's key, an
// admin on the hub, invites the site's key past the hub's token gate
// through exe; the site's key itself never leaves this daemon.

// hubPrefix and hubUpload are what a hub signature covers ahead of the
// envelope and ahead of an upload's time and digest (exe-hub's
// envelope.Prefix and UploadPrefix).
const (
	hubPrefix = "exe-hub:v1\n"
	hubUpload = "exe-hub:v1\nupload\n"
	// the hub's limits on a profile (exe-hub skill.md, profile.set)
	hubMaxName = 64
	hubMaxBio  = 1024
)

// HubOrigin is a hub's address as a node keeps it: http or https, a
// host, nothing after. "" stays "", which is no hub.
func HubOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%w: a hub's address is http(s)://host, nothing after", site.ErrInvalid)
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

var hubThreadPath = regexp.MustCompile(`^/p/([0-9a-f]{64})$`)

// ParseHubThread reads a post's hub: line, https://<hub>/p/<64 hex>, into
// the hub's origin and the post's id.
func ParseHubThread(raw string) (origin, id string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	m := hubThreadPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), m[1], true
}

// hubThreadFor is what the template gets for a post whose hub: line
// reads: the thread page, the replies frame, the hub and its host, the
// post's id.
func hubThreadFor(raw string) map[string]any {
	origin, id, ok := ParseHubThread(raw)
	if !ok {
		return nil
	}
	return map[string]any{
		"link":  origin + "/p/" + id,
		"frame": origin + "/p/" + id + "/replies",
		"hub":   origin,
		"host":  strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://"),
		"id":    id,
	}
}

// HubProfileID is a key's id on a hub: 16 hex characters of its sha256
// (exe-hub's identity.Fingerprint).
func HubProfileID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// SolanaAddress is the key as a Solana address: its 32 bytes in base58.
func SolanaAddress(pub ed25519.PublicKey) string {
	const abc = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	n := new(big.Int).SetBytes(pub)
	var out []byte
	for mod, b58 := new(big.Int), big.NewInt(58); n.Sign() > 0; {
		n.DivMod(n, b58, mod)
		out = append(out, abc[mod.Int64()])
	}
	for _, b := range pub {
		if b != 0 {
			break
		}
		out = append(out, '1')
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

// ---- the ledger ----

// hubLedger is the node's record of the site on its hub, beside node.json
// in <state>/<id>/hub.json. The post's hub: line in the Workspace is the
// record a person sees; this is the guard that keeps a post from being
// announced twice when the line is lost — a crash between the hub's
// answer and the file write, or a Writer save that drops it.
type hubLedger struct {
	Posts     map[string]*hubEntry `json:"posts"`                // post id (the UUID, never the slug)
	Invited   string               `json:"invited,omitempty"`    // the hub the node invited the key on
	Profile   string               `json:"profile,omitempty"`    // what the profile last sent said, hashed
	Avatar    string               `json:"avatar,omitempty"`     // the avatar.png last uploaded, hashed
	AvatarCID string               `json:"avatar_cid,omitempty"` // and the CID the hub gave it
}

// A post's state on the hub: there before announcing was switched on
// (never announced by itself), asked for by hand, signed and on its way,
// or announced.
const (
	hubBefore    = "before"
	hubWant      = "want"
	hubPending   = "pending"
	hubAnnounced = "announced"
)

type hubEntry struct {
	State string    `json:"state"`
	Hub   string    `json:"hub,omitempty"` // the thread, once announced
	Env   string    `json:"env,omitempty"` // pending: the envelope exactly as signed
	Sig   []byte    `json:"sig,omitempty"`
	TS    time.Time `json:"ts,omitempty"`
}

func (s *Service) ledgerPath(id string) string { return filepath.Join(s.State, id, "hub.json") }

func (s *Service) loadLedger(id string) *hubLedger {
	l := &hubLedger{}
	if b, err := os.ReadFile(s.ledgerPath(id)); err == nil {
		json.Unmarshal(b, l)
	}
	if l.Posts == nil {
		l.Posts = map[string]*hubEntry{}
	}
	return l
}

func (s *Service) saveLedger(id string, l *hubLedger) error {
	if err := os.MkdirAll(filepath.Dir(s.ledgerPath(id)), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	tmp := s.ledgerPath(id) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.ledgerPath(id))
}

// ledger runs f over the site's ledger under the lock and saves it.
func (s *Service) ledger(id string, f func(l *hubLedger)) (*hubLedger, error) {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	l := s.loadLedger(id)
	f(l)
	return l, s.saveLedger(id, l)
}

// ledgerHubs is every thread the ledger knows, by post id, for a build
// whose front matter lost a line.
func (s *Service) ledgerHubs(id string) map[string]string {
	s.hubMu.Lock()
	l := s.loadLedger(id)
	s.hubMu.Unlock()
	out := map[string]string{}
	for pid, e := range l.Posts {
		if e.State == hubAnnounced && e.Hub != "" {
			out[pid] = e.Hub
		}
	}
	return out
}

// ---- the hub's API, signed with the site's key ----

type hubClient struct {
	base string
	http *http.Client
}

func (s *Service) hub(origin string) hubClient {
	c := s.HubHTTP
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	return hubClient{base: origin, http: c}
}

type hubError struct {
	Status int
	Msg    string
	Retry  time.Duration
}

func (e *hubError) Error() string {
	if e.Msg != "" {
		return "hub: " + e.Msg
	}
	return fmt.Sprintf("hub: HTTP %d", e.Status)
}

func (h hubClient) do(ctx context.Context, req *http.Request, out any) error {
	resp, err := h.http.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("hub: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var ae struct {
			Error string `json:"error"`
		}
		json.Unmarshal(b, &ae)
		e := &hubError{Status: resp.StatusCode, Msg: ae.Error}
		if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && n > 0 {
			e.Retry = time.Duration(n) * time.Second
		}
		return e
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (h hubClient) seq(ctx context.Context, author string) (int64, error) {
	req, _ := http.NewRequest("GET", h.base+"/v1/seq?author="+url.QueryEscape(author), nil)
	var out struct {
		Seq int64 `json:"seq"`
	}
	err := h.do(ctx, req, &out)
	return out.Seq, err
}

// msg sends one signed envelope and answers the post's id: accepted, or
// a duplicate of one the hub already has, which is the same answer.
func (h hubClient) msg(ctx context.Context, env string, sig []byte) (string, error) {
	body, _ := json.Marshal(map[string]any{"envelope": []byte(env), "sig": sig})
	req, _ := http.NewRequest("POST", h.base+"/v1/msg", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		ID string `json:"id"`
	}
	err := h.do(ctx, req, &out)
	return out.ID, err
}

// avatar uploads a picture for the profile; the hub answers the CID of
// its own 128 by 128 PNG, the only kind profile.set takes.
func (h hubClient) avatar(ctx context.Context, priv ed25519.PrivateKey, pic []byte) (string, error) {
	sum := sha256.Sum256(pic)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sig := ed25519.Sign(priv, []byte(hubUpload+ts+"\n"+hex.EncodeToString(sum[:])))
	req, _ := http.NewRequest("POST", h.base+"/v1/avatar", bytes.NewReader(pic))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Hub-Author", base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	req.Header.Set("X-Hub-Ts", ts)
	req.Header.Set("X-Hub-Sig", base64.StdEncoding.EncodeToString(sig))
	var out struct {
		CID string `json:"cid"`
	}
	err := h.do(ctx, req, &out)
	return out.CID, err
}

// signEnv makes an envelope and signs it as a hub client does: the
// prefix, then the envelope's exact bytes.
func signEnv(priv ed25519.PrivateKey, typ string, seq int64, body any) (string, []byte) {
	env, _ := json.Marshal(map[string]any{
		"type": typ, "author": base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)),
		"seq": seq, "ts": time.Now().UnixMilli(), "body": body,
	})
	return string(env), ed25519.Sign(priv, append([]byte(hubPrefix), env...))
}

// send numbers, signs and sends one op now.
func (h hubClient) send(ctx context.Context, priv ed25519.PrivateKey, typ string, body any) (string, error) {
	author := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	seq, err := h.seq(ctx, author)
	if err != nil {
		return "", err
	}
	env, sig := signEnv(priv, typ, seq+1, body)
	return h.msg(ctx, env, sig)
}

// ---- the site's key ----

// siteKey reads the site's key from the node's state; make says whether
// to make one when there is none. It is the IPNS key too, kept as PKCS#8
// PEM in <state>/<id>/ipns.key; making it lends nothing to Kubo (that is
// ensureKey's, when IPFS is switched on).
func (s *Service) siteKey(id string, make bool) (ed25519.PrivateKey, error) {
	priv, err := readKey(s.keyPath(id))
	if err == nil || !make || !errors.Is(err, os.ErrNotExist) {
		return priv, err
	}
	return makeKey(s.keyPath(id), s.Logf, id)
}

// ---- switching announcing on ----

// hubReason is why a site cannot announce on this node, or "".
func (s *Service) hubReason(st *site.Site, n Node) string {
	_, keyErr := os.Stat(s.keyPath(st.ID))
	switch {
	case s.Exe == nil:
		return "exe is not configured on this node, and the invite goes through it"
	case keyErr != nil && st.IPNS != "":
		return "this site's IPNS name is a key this node does not hold (a Planet site brought in without its key); it announces once that key is imported"
	case !n.Expose || n.Hostname == "":
		return "expose the site first: the hub's link card needs a public address"
	case n.Hub == "":
		return "give the hub's address"
	}
	return ""
}

// startHub is SetNode's part when announcing is switched on, or the hub
// changes while it is: the key is made if there is none, the node
// invites it on the hub through exe (its own key is the hub's admin),
// and every post already published is marked as there before, so only
// posts published from now on announce themselves.
func (s *Service) startHub(ctx context.Context, st *site.Site, n Node, baseline bool) error {
	if why := s.hubReason(st, n); why != "" {
		return fmt.Errorf("%w: %s", site.ErrInvalid, why)
	}
	priv, err := s.siteKey(st.ID, true)
	if err != nil {
		return err
	}
	pub := priv.Public().(ed25519.PublicKey)
	note := "Site: " + st.Name + ", " + n.Hostname
	if err := s.Exe.HubPublish(ctx, n.Hub, "invite.set", map[string]string{
		"target": base64.StdEncoding.EncodeToString(pub), "note": trimBytes(note, 512),
	}); err != nil {
		return fmt.Errorf("the invite on %s: %w", n.Hub, err)
	}
	s.Logf("planet: %s invited on %s as %s", st.Slug, n.Hub, HubProfileID(pub))
	var posts []*site.Post
	if baseline {
		if posts, err = s.Store.Posts(st.Slug, "post"); err != nil {
			return err
		}
	}
	_, err = s.ledger(st.ID, func(l *hubLedger) {
		if l.Invited != n.Hub {
			l.Profile = "" // a new hub has not heard the profile
			l.AvatarCID = ""
		}
		l.Invited = n.Hub
		for _, p := range posts {
			if p.ID != "" && p.Hub == "" && l.Posts[p.ID] == nil {
				l.Posts[p.ID] = &hubEntry{State: hubBefore}
			}
		}
	})
	return err
}

// LiftInvite takes the site's invite back, when the site is deleted.
func (s *Service) LiftInvite(ctx context.Context, id string) {
	l := s.loadLedger(id)
	if l.Invited == "" || s.Exe == nil {
		return
	}
	priv, err := s.siteKey(id, false)
	if err != nil {
		return
	}
	pub := priv.Public().(ed25519.PublicKey)
	if err := s.Exe.HubPublish(ctx, l.Invited, "invite.lift", map[string]string{"target": base64.StdEncoding.EncodeToString(pub)}); err != nil {
		s.Logf("planet: lift the invite of %s on %s: %v", id, l.Invited, err)
		return
	}
	s.ledger(id, func(l *hubLedger) { l.Invited = "" })
}

// AnnouncePost asks for one post to be announced, a post published
// before announcing was switched on: the Planet app's Announce on Hub.
func (s *Service) AnnouncePost(slug, post string) error {
	st, err := s.Store.Site(slug)
	if err != nil {
		return err
	}
	if n := s.loadNode(st.ID); !n.Announce {
		return fmt.Errorf("%w: switch Announce on in Publish… first", site.ErrInvalid)
	}
	p, err := s.Store.Post(slug, post, "post")
	if err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("%w: the post has no id", site.ErrInvalid)
	}
	if p.Hub != "" {
		return fmt.Errorf("%w: the post is on the hub already", site.ErrInvalid)
	}
	if _, err := s.ledger(st.ID, func(l *hubLedger) {
		if e := l.Posts[p.ID]; e == nil || e.State == hubBefore {
			l.Posts[p.ID] = &hubEntry{State: hubWant}
		}
	}); err != nil {
		return err
	}
	s.startAnnounce(slug)
	return nil
}

// ---- the announcer ----

// startAnnounce runs one announcing pass at a time per site, as
// publishing does; a pass that meets the hub's cooldown comes back when
// it has run out.
func (s *Service) startAnnounce(slug string) {
	s.mu.Lock()
	p := s.announcing[slug]
	if p == nil {
		p = &pubState{}
		s.announcing[slug] = p
	}
	if p.running {
		p.again = true
		s.mu.Unlock()
		return
	}
	p.running = true
	s.mu.Unlock()
	go func() {
		for {
			wait := s.announceOnce(slug)
			s.mu.Lock()
			if wait > 0 {
				if t := s.announceTimers[slug]; t != nil {
					t.Stop()
				}
				s.announceTimers[slug] = time.AfterFunc(wait, func() { s.startAnnounce(slug) })
			}
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

// announceOnce brings the site's profile and posts up to date on its hub
// and answers how long to wait before trying again, 0 for not at all.
func (s *Service) announceOnce(slug string) time.Duration {
	st, err := s.Store.Site(slug)
	if err != nil {
		return 0
	}
	s.nodeMu.Lock()
	n := s.loadNode(st.ID)
	s.nodeMu.Unlock()
	if !n.Announce || s.hubReason(st, n) != "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	wait, err := s.announce(ctx, st, n)
	s.nodeMu.Lock()
	n = s.loadNode(st.ID)
	msg := ""
	var he *hubError
	if errors.As(err, &he) && he.Status == http.StatusTooManyRequests {
		err = nil // the hub's cooldown is a wait, not a failure: the timer comes back
	}
	if err != nil {
		msg = err.Error()
		s.Logf("planet: announce %s: %v", slug, err)
	}
	if n.AnnounceError != msg {
		n.AnnounceError = msg
		s.saveNode(n)
	}
	s.nodeMu.Unlock()
	if err != nil && wait == 0 {
		wait = 5 * time.Minute // the hub may be down; a later build tries sooner
	}
	return wait
}

func (s *Service) announce(ctx context.Context, st *site.Site, n Node) (time.Duration, error) {
	priv, err := s.siteKey(st.ID, false)
	if err != nil {
		return 0, err
	}
	hc := s.hub(n.Hub)
	if l := s.loadLedger(st.ID); l.Invited != n.Hub {
		// the invite is SetNode's; one that failed there is retried here
		if err := s.startHub(ctx, st, n, false); err != nil {
			return 0, err
		}
	}
	if err := s.hubProfile(ctx, hc, priv, st, n); err != nil {
		return retryOf(err), err
	}
	posts, err := s.Store.Posts(st.Slug, "post")
	if err != nil {
		return 0, err
	}
	// oldest first, so a queue of several posts lands in the order written
	for i := len(posts) - 1; i >= 0; i-- {
		p := posts[i]
		if p.ID == "" || p.Error != "" {
			continue
		}
		l := s.loadLedger(st.ID)
		e := l.Posts[p.ID]
		switch {
		case p.Hub != "":
			// the front matter has a thread: the ledger learns it
			if e == nil || e.State != hubAnnounced || e.Hub != p.Hub {
				s.ledger(st.ID, func(l *hubLedger) { l.Posts[p.ID] = &hubEntry{State: hubAnnounced, Hub: p.Hub, TS: time.Now()} })
			}
			continue
		case e != nil && e.State == hubAnnounced:
			// the line was lost (a save over it): written back, not posted again
			if _, err := s.Store.SetPostHub(st.Slug, p.Slug, "post", e.Hub); err != nil {
				return 0, err
			}
			continue
		case e != nil && e.State == hubBefore:
			continue
		}
		thread, err := s.announcePost(ctx, hc, priv, st, n, p)
		if err != nil {
			return retryOf(err), err
		}
		if _, err := s.Store.SetPostHub(st.Slug, p.Slug, "post", thread); err != nil {
			return 0, err
		}
		s.Logf("planet: announced %s/%s at %s", st.Slug, p.Slug, thread)
	}
	return 0, nil
}

// retryOf is how long the hub asked to wait (its cooldown), or 0.
func retryOf(err error) time.Duration {
	var he *hubError
	if errors.As(err, &he) && he.Retry > 0 {
		return he.Retry + time.Second
	}
	return 0
}

// announcePost posts one blog post's announcement and answers its
// thread's address. The envelope is kept in the ledger before it is
// sent, and sent again as it was until the hub has it: a post that
// landed answers as a duplicate with the same id, so a crash anywhere
// in between never posts twice. One the hub refused for its seq never
// landed (a landed one would be a duplicate), so it is signed anew.
func (s *Service) announcePost(ctx context.Context, hc hubClient, priv ed25519.PrivateKey, st *site.Site, n Node, p *site.Post) (string, error) {
	for try := 0; try < 2; try++ {
		e := s.loadLedger(st.ID).Posts[p.ID]
		if e == nil || e.State != hubPending || e.Env == "" {
			text, err := s.announcement(st, n, p)
			if err != nil {
				return "", err
			}
			author := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
			seq, err := hc.seq(ctx, author)
			if err != nil {
				return "", err
			}
			env, sig := signEnv(priv, "post.create", seq+1, map[string]string{"text": text})
			e = &hubEntry{State: hubPending, Env: env, Sig: sig, TS: time.Now()}
			if _, err := s.ledger(st.ID, func(l *hubLedger) { l.Posts[p.ID] = e }); err != nil {
				return "", err
			}
		}
		id, err := hc.msg(ctx, e.Env, e.Sig)
		var he *hubError
		if errors.As(err, &he) && he.Status == http.StatusConflict {
			s.ledger(st.ID, func(l *hubLedger) { l.Posts[p.ID] = &hubEntry{State: hubWant} })
			continue
		}
		if err != nil {
			return "", err
		}
		thread := n.Hub + "/p/" + id
		if _, err := s.ledger(st.ID, func(l *hubLedger) {
			l.Posts[p.ID] = &hubEntry{State: hubAnnounced, Hub: thread, TS: time.Now()}
		}); err != nil {
			return "", err
		}
		return thread, nil
	}
	return "", errors.New("hub: the post's seq was refused twice")
}

// announcement is the hub post for a blog post: its title in bold, its
// summary, and its address last-but-only, the first link, so the hub
// unfurls the page's own card.
func (s *Service) announcement(st *site.Site, n Node, listed *site.Post) (string, error) {
	p, err := s.Store.Post(st.Slug, listed.Slug, "post")
	if err != nil {
		return "", err
	}
	_, arts := FromSite(st, []*site.Post{p})
	var b strings.Builder
	if t := strings.TrimSpace(p.Title); t != "" {
		b.WriteString("**" + strings.ReplaceAll(t, "*", "") + "**\n\n")
	}
	if len(arts) == 1 && arts[0].Summary != nil {
		if sum := strings.TrimSpace(*arts[0].Summary); sum != "" {
			b.WriteString(cutWords(sum, 280) + "\n\n")
		}
	}
	b.WriteString("https://" + n.Hostname + "/" + p.Slug + "/")
	return b.String(), nil
}

// hubProfile sends the site's profile when what it says moved: its name,
// its about and address, its avatar.png as the hub keeps it.
func (s *Service) hubProfile(ctx context.Context, hc hubClient, priv ed25519.PrivateKey, st *site.Site, n Node) error {
	name := strings.TrimSpace(st.Name)
	if name == "" {
		name = st.Slug
	}
	name = trimBytes(name, hubMaxName)
	addr := "https://" + n.Hostname + "/"
	bio := addr
	if about := strings.TrimSpace(st.About); about != "" {
		bio = trimBytes(about, hubMaxBio-len(addr)-2) + "\n\n" + addr
	}
	l := s.loadLedger(st.ID)
	cid := l.AvatarCID
	if pic, err := os.ReadFile(filepath.Join(st.Dir, "avatar.png")); err == nil {
		sum := sha256.Sum256(pic)
		hash := hex.EncodeToString(sum[:])
		if hash != l.Avatar || cid == "" {
			if cid, err = hc.avatar(ctx, priv, pic); err != nil {
				return err
			}
			if _, err := s.ledger(st.ID, func(l *hubLedger) { l.Avatar, l.AvatarCID = hash, cid }); err != nil {
				return err
			}
		}
	} else {
		cid = ""
	}
	sum := sha256.Sum256([]byte(n.Hub + "\x00" + name + "\x00" + bio + "\x00" + cid))
	want := hex.EncodeToString(sum[:])
	if l.Profile == want {
		return nil
	}
	body := map[string]string{"name": name, "bio": bio}
	if cid != "" {
		body["avatar"] = cid
	}
	if _, err := hc.send(ctx, priv, "profile.set", body); err != nil {
		return err
	}
	_, err := s.ledger(st.ID, func(l *hubLedger) { l.Profile = want })
	return err
}

// trimBytes cuts s to at most n bytes on a character boundary.
func trimBytes(s string, n int) string {
	if n < 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// cutWords shortens words to about n characters at a space, with an
// ellipsis.
func cutWords(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}
