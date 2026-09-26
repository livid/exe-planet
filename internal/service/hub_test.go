package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// fakeHub is enough of an exe hub for a site's key: it checks every
// signature as the hub does (the prefix, then the envelope's bytes; an
// upload's time and digest), keeps seqs per author, answers a post's id
// as the hash of its envelope and a duplicate with the same id, and can
// hold posts to a cooldown.
type fakeHub struct {
	mu       sync.Mutex
	seqs     map[string]int64
	msgs     map[string]string // id -> envelope
	posts    []map[string]any  // post.create bodies, in order
	profiles []map[string]any
	avatars  int
	cooling  time.Time // until then a post.create gets a 429, as the hub's cooldown does
	bad      []string
}

func newFakeHub(t *testing.T) (*fakeHub, *httptest.Server) {
	h := &fakeHub{seqs: map[string]int64{}, msgs: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/seq", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]int64{"seq": h.seqs[r.URL.Query().Get("author")]})
	})
	mux.HandleFunc("POST /v1/msg", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		var in struct{ Envelope, Sig []byte }
		json.NewDecoder(r.Body).Decode(&in)
		var env struct {
			Type, Author string
			Seq          int64
			Body         map[string]any
		}
		if err := json.Unmarshal(in.Envelope, &env); err != nil {
			http.Error(w, `{"error":"bad envelope"}`, 400)
			return
		}
		pub, _ := base64.StdEncoding.DecodeString(env.Author)
		if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, append([]byte(hubPrefix), in.Envelope...), in.Sig) {
			h.bad = append(h.bad, "sig")
			http.Error(w, `{"error":"bad signature"}`, 401)
			return
		}
		sum := sha256.Sum256(in.Envelope)
		id := hex.EncodeToString(sum[:])
		if _, dup := h.msgs[id]; dup {
			json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "duplicate"})
			return
		}
		if env.Type == "post.create" && time.Now().Before(h.cooling) {
			w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(h.cooling)/time.Second)+1))
			http.Error(w, `{"error":"posting cooldown: try again in 1s"}`, 429)
			return
		}
		if env.Seq <= h.seqs[env.Author] {
			http.Error(w, `{"error":"stale seq"}`, 409)
			return
		}
		h.seqs[env.Author] = env.Seq
		h.msgs[id] = string(in.Envelope)
		switch env.Type {
		case "post.create":
			h.posts = append(h.posts, env.Body)
		case "profile.set":
			h.profiles = append(h.profiles, env.Body)
		}
		json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "accepted"})
	})
	mux.HandleFunc("POST /v1/avatar", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		pub, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Hub-Author"))
		sig, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Hub-Sig"))
		sum := sha256.Sum256(body)
		if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, []byte(hubUpload+r.Header.Get("X-Hub-Ts")+"\n"+hex.EncodeToString(sum[:])), sig) {
			h.bad = append(h.bad, "upload")
			http.Error(w, `{"error":"bad upload signature"}`, 401)
			return
		}
		h.avatars++
		json.NewEncoder(w).Encode(map[string]string{"cid": "bafkavatar" + strconv.Itoa(h.avatars)})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return h, srv
}

func (h *fakeHub) snapshot() (posts, profiles []map[string]any, bad []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]map[string]any(nil), h.posts...), append([]map[string]any(nil), h.profiles...), append([]string(nil), h.bad...)
}

// waitFor polls until ok or fails the test.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func postHub(t *testing.T, store *site.Store, slug string) string {
	t.Helper()
	p, err := store.Post("exe", slug, site.KindPost)
	if err != nil {
		t.Fatal(err)
	}
	return p.Hub
}

// TestAnnounce (PLAN.md, Replies from the hub): switched on, the site's
// key is invited through exe and what is already published is left
// alone; a post published after announces itself — the profile first,
// then the post, signed by the site's key, its first link the post's
// address — and its hub: line is written; one published before goes
// only when asked; the hub's cooldown is waited out; a lost hub: line is
// written back and a crash between the hub's answer and the file never
// posts twice; a site whose IPNS key is not here cannot switch on; a
// deleted site's invite is lifted.
func TestAnnounce(t *testing.T) {
	svc, store, fe, _ := newNodeService(t)
	fh, hubSrv := newFakeHub(t)
	svc.HubHTTP = hubSrv.Client()
	ctx := context.Background()
	if _, err := store.CreateSite("exe", "exe", "A personal VM cloud.", "platinum"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("exe")
	os.WriteFile(filepath.Join(st.Dir, "avatar.png"), []byte("a png"), 0o644)
	store.CreatePost("exe", site.KindPost, "old", site.Meta{Title: "Old", Created: time.Now().Add(-time.Hour)}, "Written before.\n")

	// not before the site is exposed, nor without a hub
	if _, err := svc.SetNode(ctx, "exe", NodeUpdate{Announce: ptr(true), Hub: ptr(hubSrv.URL)}); err == nil || !strings.Contains(err.Error(), "expose") {
		t.Fatalf("announce on an unexposed site: %v", err)
	}
	if _, err := svc.SetNode(ctx, "exe", NodeUpdate{Expose: ptr(true), Subdomain: ptr("blog")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetNode(ctx, "exe", NodeUpdate{Hub: ptr(hubSrv.URL + "/p/x")}); err == nil {
		t.Error("a hub address with a path was taken")
	}

	n, err := svc.SetNode(ctx, "exe", NodeUpdate{Announce: ptr(true), Hub: ptr(hubSrv.URL + "/")})
	if err != nil {
		t.Fatal(err)
	}
	if !n.Announce || n.Hub != hubSrv.URL || n.HubID == "" || n.Address == "" || n.HubReason != "" {
		t.Fatalf("node after switching on: %+v", n)
	}
	priv, err := svc.siteKey(st.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if n.HubID != HubProfileID(pub) || n.Address != SolanaAddress(pub) {
		t.Errorf("the key's spellings: %s %s", n.HubID, n.Address)
	}
	fe.mu.Lock()
	ops := append([]fakeHubOp(nil), fe.hubOps...)
	fe.mu.Unlock()
	if len(ops) != 1 || ops[0].Type != "invite.set" || ops[0].Hub != hubSrv.URL ||
		ops[0].Body["target"] != base64.StdEncoding.EncodeToString(pub) || !strings.Contains(ops[0].Body["note"], "blog.example.test") {
		t.Fatalf("the invite through exe: %+v", ops)
	}

	// a post published now announces itself once its page is live
	store.CreatePost("exe", site.KindPost, "new", site.Meta{Title: "New *thing*", Created: time.Now()}, "The first words of the new post.\n\nMore after.\n")
	if _, err := svc.Build("exe"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the new post's hub line", func() bool { return postHub(t, store, "new") != "" })
	posts, profiles, bad := fh.snapshot()
	if len(bad) > 0 {
		t.Fatalf("bad signatures: %v", bad)
	}
	if len(profiles) != 1 || profiles[0]["name"] != "exe" || profiles[0]["avatar"] != "bafkavatar1" ||
		!strings.Contains(profiles[0]["bio"].(string), "A personal VM cloud.") || !strings.HasSuffix(profiles[0]["bio"].(string), "https://blog.example.test/") {
		t.Errorf("profile: %+v", profiles)
	}
	if len(posts) != 1 {
		t.Fatalf("posts on the hub: %+v", posts)
	}
	text := posts[0]["text"].(string)
	if !strings.HasPrefix(text, "**New thing**\n\n") || !strings.Contains(text, "The first words of the new post.") ||
		!strings.HasSuffix(text, "https://blog.example.test/new/") {
		t.Errorf("announcement: %q", text)
	}
	thread := postHub(t, store, "new")
	if _, id, ok := ParseHubThread(thread); !ok || !strings.HasPrefix(thread, hubSrv.URL+"/p/") || len(id) != 64 {
		t.Errorf("hub line: %q", thread)
	}
	if postHub(t, store, "old") != "" {
		t.Error("a post from before the switch was announced by itself")
	}

	// the page frames the thread; the old post's does not
	if _, err := svc.Build("exe"); err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "new", "index.html"))
	if !strings.Contains(string(page), `src="`+thread+`/replies"`) || !strings.Contains(string(page), `id="compose"`) {
		t.Error("the new post's page has no Replies window")
	}
	if old, _ := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "old", "index.html")); strings.Contains(string(old), `id="replies"`) {
		t.Error("the old post's page has a Replies window")
	}

	// the old one goes when asked; the hub's cooldown is waited out
	fh.mu.Lock()
	fh.cooling = time.Now().Add(time.Second)
	fh.mu.Unlock()
	asked := time.Now()
	if err := svc.AnnouncePost("exe", "old"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the old post's hub line", func() bool { return postHub(t, store, "old") != "" })
	if waited := time.Since(asked); waited < time.Second {
		t.Errorf("the old post went in %v, inside the hub's cooldown", waited)
	}
	if n, _ := svc.Node("exe"); n.AnnounceError != "" {
		t.Errorf("an error stands after the retry: %q", n.AnnounceError)
	}
	if posts, _, _ := fh.snapshot(); len(posts) != 2 || !strings.HasSuffix(posts[1]["text"].(string), "/old/") {
		t.Errorf("after Announce on Hub: %+v", posts)
	}
	if err := svc.AnnouncePost("exe", "old"); err == nil {
		t.Error("an announced post was announced again")
	}

	// a save that drops the line: written back, not posted again
	p, _ := store.Post("exe", "new", site.KindPost)
	p.Hub = ""
	os.WriteFile(filepath.Join(p.Dir, "index.md"), site.FormatDoc(p.Meta, p.Body), 0o644)
	if _, err := svc.Build("exe"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the line written back", func() bool { return postHub(t, store, "new") == thread })
	if posts, _, _ := fh.snapshot(); len(posts) != 2 {
		t.Errorf("a lost line posted again: %d posts", len(posts))
	}

	// a crash after the hub took the post, before anything was written:
	// the kept envelope goes again and the hub knows it
	store.CreatePost("exe", site.KindPost, "third", site.Meta{Title: "Third", Created: time.Now()}, "Third.\n")
	p3, _ := store.Post("exe", "third", site.KindPost)
	seq, _ := svc.hub(hubSrv.URL).seq(ctx, base64.StdEncoding.EncodeToString(pub))
	env, sig := signEnv(priv, "post.create", seq+1, map[string]string{"text": "landed before the crash"})
	if _, err := svc.hub(hubSrv.URL).msg(ctx, env, sig); err != nil {
		t.Fatal(err)
	}
	svc.ledger(st.ID, func(l *hubLedger) { l.Posts[p3.ID] = &hubEntry{State: hubPending, Env: env, Sig: sig} })
	if _, err := svc.Build("exe"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the third post's hub line", func() bool { return postHub(t, store, "third") != "" })
	if posts, _, _ := fh.snapshot(); len(posts) != 3 {
		t.Errorf("a crash posted twice: %d posts", len(posts))
	}
	sum := sha256.Sum256([]byte(env))
	if postHub(t, store, "third") != hubSrv.URL+"/p/"+hex.EncodeToString(sum[:]) {
		t.Errorf("the third post's line is not the post that landed: %s", postHub(t, store, "third"))
	}

	// the profile goes again only when it moved
	if _, profiles, _ := fh.snapshot(); len(profiles) != 1 {
		t.Errorf("profile sent %d times without a change", len(profiles))
	}
	about := "Now with replies."
	store.UpdateSite("exe", site.SiteUpdate{About: &about})
	svc.Build("exe")
	waitFor(t, "the new profile", func() bool { _, pr, _ := fh.snapshot(); return len(pr) == 2 })

	// a drafted post is never announced
	store.CreatePost("exe", site.KindDraft, "draft", site.Meta{Title: "Draft"}, "Not yet.\n")
	svc.Build("exe")
	time.Sleep(200 * time.Millisecond)
	if posts, _, _ := fh.snapshot(); len(posts) != 3 {
		t.Errorf("a draft was announced: %d posts", len(posts))
	}

	// deleted, the site's invite is lifted
	svc.LiftInvite(ctx, st.ID)
	fe.mu.Lock()
	last := fe.hubOps[len(fe.hubOps)-1]
	fe.mu.Unlock()
	if last.Type != "invite.lift" || last.Body["target"] != base64.StdEncoding.EncodeToString(pub) {
		t.Errorf("lift: %+v", last)
	}
}

// TestAnnounceImportedSite: a site whose IPNS name is a key this node
// does not hold — a Planet site brought in without its key — cannot
// announce: a new key would be a new identity.
func TestAnnounceImportedSite(t *testing.T) {
	svc, store, _, _ := newNodeService(t)
	_, hubSrv := newFakeHub(t)
	ctx := context.Background()
	store.CreateSite("mac", "Mac", "", "platinum")
	ipns := "k51qzi5uqu5dlvj2baxnqndepeb86cbk3ng7n3i46uzyxzyqj2xjonzllnv0v8"
	store.UpdateSite("mac", site.SiteUpdate{IPNS: &ipns})
	svc.SetNode(ctx, "mac", NodeUpdate{Expose: ptr(true)})
	n, _ := svc.Node("mac")
	if !strings.Contains(n.HubReason, "does not hold") {
		t.Errorf("reason: %q", n.HubReason)
	}
	if _, err := svc.SetNode(ctx, "mac", NodeUpdate{Announce: ptr(true), Hub: ptr(hubSrv.URL)}); err == nil {
		t.Error("an imported site without its key switched announcing on")
	}
	st, _ := store.Site("mac")
	if _, err := os.Stat(svc.keyPath(st.ID)); err == nil {
		t.Error("a key was made for an imported site")
	}
}

// TestHubThreadTemplate: the Replies window comes from a valid hub: line
// only, carries the frame, the thread, the hub and the root, and loads
// the template's own script; the ledger stands in for a lost line.
func TestHubThreadTemplate(t *testing.T) {
	svc, store := newService(t)
	store.CreateSite("exe", "exe", "", "platinum")
	st, _ := store.Site("exe")
	id := strings.Repeat("ab", 32)
	store.CreatePost("exe", site.KindPost, "with", site.Meta{Title: "With", Hub: "https://hub.example.test/p/" + id}, "x\n")
	store.CreatePost("exe", site.KindPost, "bad", site.Meta{Title: "Bad", Hub: "https://hub.example.test/p/nope"}, "x\n")
	store.CreatePost("exe", site.KindPost, "lost", site.Meta{Title: "Lost"}, "x\n")
	lost, _ := store.Post("exe", "lost", site.KindPost)
	svc.ledger(st.ID, func(l *hubLedger) {
		l.Posts[lost.ID] = &hubEntry{State: hubAnnounced, Hub: "https://hub.example.test/p/" + strings.Repeat("cd", 32)}
	})
	if _, err := svc.Build("exe"); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	page := read("with/index.html")
	for _, want := range []string{
		`<script defer src="../assets/replies.js"></script>`,
		`id="compose" data-hub="https://hub.example.test" data-root="` + id + `"`,
		`src="https://hub.example.test/p/` + id + `/replies"`,
		`href="https://hub.example.test/p/` + id + `" target="_blank"`,
		`title="Replies on hub.example.test"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("with/index.html lacks %q", want)
		}
	}
	if strings.Contains(read("bad/index.html"), `id="replies"`) {
		t.Error("a malformed hub line drew a Replies window")
	}
	if !strings.Contains(read("lost/index.html"), strings.Repeat("cd", 32)+"/replies") {
		t.Error("the ledger did not stand in for a lost line")
	}
	if strings.Contains(read("index.html"), `id="compose"`) {
		t.Error("the index page has a composer")
	}
	if _, err := os.Stat(filepath.Join(svc.PublicDir(st.ID), "assets", "replies.js")); err != nil {
		t.Error("assets/replies.js is not in the build")
	}
	// article.json keeps Planet's shape
	if strings.Contains(read(lost.ID+"/article.json"), "hub") {
		t.Error("article.json carries the hub")
	}
}
