package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPollerSettlesThenReports(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "charts", "posts"), 0o755)
	os.WriteFile(filepath.Join(root, "charts", "site.json"), []byte("{}"), 0o644)
	os.MkdirAll(filepath.Join(root, ".Trash"), 0o755)
	os.WriteFile(filepath.Join(root, ".Trash", "x"), []byte("x"), 0o644)
	var got [][]string
	p := &Poller{Root: root, OnChange: func(paths []string) { got = append(got, paths) }}
	p.Tick() // baseline
	p.Tick()
	if len(got) != 0 || p.Generation() != 0 {
		t.Fatalf("quiet tree reported %v", got)
	}

	os.WriteFile(filepath.Join(root, "charts", "posts", "hello", "index.md"), nil, 0o644) // fails: no folder
	os.MkdirAll(filepath.Join(root, "charts", "posts", "hello"), 0o755)
	os.WriteFile(filepath.Join(root, "charts", "posts", "hello", "index.md"), []byte("---\n---\n"), 0o644)
	p.Tick() // seen, not settled
	if len(got) != 0 {
		t.Fatalf("reported before settling: %v", got)
	}
	os.WriteFile(filepath.Join(root, "charts", "posts", "hello", "index.md"), []byte("---\ntitle: x\n---\n"), 0o644)
	p.Tick() // still moving
	if len(got) != 0 {
		t.Fatalf("reported while moving: %v", got)
	}
	p.Tick() // settled
	if len(got) != 1 || strings.Join(got[0], ",") != "charts/posts/hello,charts/posts/hello/index.md" || p.Generation() != 1 {
		t.Fatalf("reported %v gen %d", got, p.Generation())
	}
	p.Tick()
	if len(got) != 1 {
		t.Fatalf("reported again with no change: %v", got)
	}

	os.WriteFile(filepath.Join(root, ".Trash", "y"), []byte("y"), 0o644)
	p.Tick()
	p.Tick()
	if len(got) != 1 {
		t.Fatalf("hidden change reported: %v", got)
	}

	os.RemoveAll(filepath.Join(root, "charts", "posts", "hello"))
	p.Tick()
	p.Tick()
	if len(got) != 2 || strings.Join(got[1], ",") != "charts/posts/hello,charts/posts/hello/index.md" {
		t.Fatalf("removal reported %v", got)
	}

	// A rewrite with the same size in the same second is still a change
	// when the mtime moves.
	os.WriteFile(filepath.Join(root, "charts", "site.json"), []byte("{}"), 0o644)
	Touch(filepath.Join(root, "charts", "site.json"), time.Now().Add(time.Second))
	p.Tick()
	p.Tick()
	if len(got) != 3 || got[2][0] != "charts/site.json" {
		t.Fatalf("touch reported %v", got)
	}
}
