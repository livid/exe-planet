// Package watch notices changes under a folder by polling, not inotify:
// no watch limits, no dependency, and a tree of a few thousand files
// stats in milliseconds. A post edited in the desk text editor, by an
// agent, or by a synced peer shows up the same way.
package watch

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

type entry struct {
	size  int64
	mtime int64
	dir   bool
}

// Poller reports a change once it has settled: two consecutive polls
// that agree with each other and differ from what was last reported.
// Hidden entries are skipped.
type Poller struct {
	Root     string
	Every    time.Duration // default one second
	OnChange func(paths []string)

	last       map[string]entry
	pending    map[string]entry
	generation atomic.Uint64
}

// Generation counts reported changes, for a status line.
func (p *Poller) Generation() uint64 { return p.generation.Load() }

// Run polls until the context ends. The first snapshot is the baseline.
func (p *Poller) Run(ctx context.Context) {
	every := p.Every
	if every <= 0 {
		every = time.Second
	}
	p.last = p.snapshot()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.Tick()
		}
	}
}

// Tick is one poll; Run calls it on the ticker and tests call it directly.
func (p *Poller) Tick() {
	if p.last == nil {
		p.last = p.snapshot()
		return
	}
	snap := p.snapshot()
	if equal(snap, p.last) {
		p.pending = nil
		return
	}
	if p.pending == nil || !equal(snap, p.pending) {
		p.pending = snap
		return
	}
	changed := diff(p.last, snap)
	p.last, p.pending = snap, nil
	p.generation.Add(1)
	if p.OnChange != nil {
		p.OnChange(changed)
	}
}

func (p *Poller) snapshot() map[string]entry {
	snap := map[string]entry{}
	filepath.WalkDir(p.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path != p.Root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(p.Root, path)
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		e := entry{dir: d.IsDir()}
		if !e.dir {
			e.size, e.mtime = info.Size(), info.ModTime().UnixNano()
		}
		snap[filepath.ToSlash(rel)] = e
		return nil
	})
	return snap
}

func equal(a, b map[string]entry) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func diff(old, now map[string]entry) []string {
	var out []string
	for k, v := range now {
		if w, ok := old[k]; !ok || w != v {
			out = append(out, k)
		}
	}
	for k := range old {
		if _, ok := now[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Touch is a test helper: it sets a file's mtime forward so a rewrite
// within the same nanosecond still counts.
func Touch(path string, t time.Time) error { return os.Chtimes(path, t, t) }
