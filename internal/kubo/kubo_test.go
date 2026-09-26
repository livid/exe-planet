package kubo_test

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livid/exe-planet/internal/kubo"
	"github.com/livid/exe-planet/internal/kubo/kubotest"
)

func TestClient(t *testing.T) {
	f := kubotest.New(t)
	c := kubo.New(f.URL())
	ctx := context.Background()
	if v, err := c.Version(ctx); err != nil || v != "0.43.0" {
		t.Fatalf("version %q %v", v, err)
	}
	priv := ed25519.NewKeyFromSeed([]byte("exe-planet kubo client test seed"))
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	want := kubo.IPNSName(priv.Public().(ed25519.PublicKey))
	k, err := c.ImportKey(ctx, "exe-planet-x", pemBytes)
	if err != nil || k.ID != want {
		t.Fatalf("import %+v %v (want %s)", k, err, want)
	}
	if _, err := c.ImportKey(ctx, "exe-planet-x", pemBytes); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second import: %v", err)
	}
	keys, _ := c.Keys(ctx)
	if len(keys) != 2 {
		t.Fatalf("keys %+v", keys)
	}

	dir := filepath.Join(t.TempDir(), "public")
	os.MkdirAll(filepath.Join(dir, "a b"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "a b", "x.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("no"), 0o644)
	cid, err := c.AddDir(ctx, dir)
	if err != nil || cid != "bafyaddx" {
		t.Fatalf("add %q %v", cid, err)
	}
	if _, adds, _, _ := f.Snapshot(); adds[0] != "public public/a b public/a b/x.txt public/index.html" {
		t.Fatalf("add parts: %q", adds[0])
	}
	p, err := c.Publish(ctx, "exe-planet-x", cid, "7200h")
	if err != nil || p.Name != want || p.Value != "/ipfs/bafyaddx" {
		t.Fatalf("publish %+v %v", p, err)
	}
	if _, err := c.Publish(ctx, "nokey", cid, "7200h"); err == nil || !strings.Contains(err.Error(), "no key by the name") {
		t.Fatalf("publish without a key: %v", err)
	}
	if err := c.Unpin(ctx, cid); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if _, _, _, pins := f.Snapshot(); pins[cid] {
		t.Fatal("still pinned")
	}
	if err := c.Unpin(ctx, cid); err != nil {
		t.Fatalf("unpin twice: %v", err)
	}
	if err := c.RemoveKey(ctx, "exe-planet-x"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if keys, _, _, _ := f.Snapshot(); len(keys) != 1 {
		t.Fatalf("keys after rm: %v", keys)
	}
	var none *kubo.Client
	if _, err := none.Version(ctx); err == nil {
		t.Fatal("a nil client answered")
	}
}

func TestIPNSName(t *testing.T) {
	// The shape every ed25519 IPNS name has, and the one Kubo 0.43 gave
	// back for a key the probe imported on 2026-09-25 (its seed kept).
	seed := []byte("exe-planet ipns name test seed!!")
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	name := kubo.IPNSName(pub)
	if !strings.HasPrefix(name, "k51qzi5uqu5d") || len(name) != 62 {
		t.Fatalf("name %q", name)
	}
	if kubo.IPNSName(ed25519.NewKeyFromSeed([]byte("another seed of thirty-two bytes")).Public().(ed25519.PublicKey)) == name {
		t.Fatal("two keys, one name")
	}
}
