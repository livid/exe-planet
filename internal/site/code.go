package site

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A site's own code, as Planet's Custom Code sheet has it: written into
// the head of every page, just after the start of its body, or just
// before its end, the way an analytics or a comments script asks to be.
// Each is a file in the site's folder, for people as much as for the
// daemon: head.html, body-start.html, body-end.html. A file there is on
// unless site.json's code_off names it, so a snippet can be switched off
// and kept, as Planet's Enable box does; an empty one is no file at all.

// CodePart is one of the three: its text, and whether it is written into
// the pages.
type CodePart struct {
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}

// CustomCode is a site's three.
type CustomCode struct {
	Head      CodePart `json:"head"`
	BodyStart CodePart `json:"body_start"`
	BodyEnd   CodePart `json:"body_end"`
}

// CodeUpdate carries the parts a PUT changes; nil leaves a part alone.
type CodeUpdate struct {
	Head      *CodePart `json:"head"`
	BodyStart *CodePart `json:"body_start"`
	BodyEnd   *CodePart `json:"body_end"`
}

// CodeFiles names each part as code_off does, and its file.
var CodeFiles = []struct{ Key, File string }{
	{"head", "head.html"},
	{"body_start", "body-start.html"},
	{"body_end", "body-end.html"},
}

// MaxCode is the most a part may hold: room for any script tag and a
// stylesheet, short of a page's worth.
const MaxCode = 64 << 10

func (c *CustomCode) part(key string) *CodePart {
	switch key {
	case "head":
		return &c.Head
	case "body_start":
		return &c.BodyStart
	}
	return &c.BodyEnd
}

func (u CodeUpdate) part(key string) *CodePart {
	switch key {
	case "head":
		return u.Head
	case "body_start":
		return u.BodyStart
	}
	return u.BodyEnd
}

// ReadCustomCode reads a site folder's three, given site.json's code_off.
func ReadCustomCode(dir string, off []string) CustomCode {
	var c CustomCode
	for _, f := range CodeFiles {
		b, err := os.ReadFile(filepath.Join(dir, f.File))
		if err != nil || strings.TrimSpace(string(b)) == "" {
			continue
		}
		*c.part(f.Key) = CodePart{Code: string(b), Enabled: !slices.Contains(off, f.Key)}
	}
	return c
}

// CustomCode is a site's three as they stand.
func (s *Store) CustomCode(slug string) (*CustomCode, error) {
	st, err := s.liveSite(slug)
	if err != nil {
		return nil, err
	}
	c := ReadCustomCode(st.Dir, st.CodeOff)
	return &c, nil
}

// SetCustomCode writes the parts given: a part with text is its file, on
// or named in code_off; a part with none takes its file away. The site
// is built again by the watcher, which sees the folder change.
func (s *Store) SetCustomCode(slug string, u CodeUpdate) (*CustomCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.liveSite(slug)
	if err != nil {
		return nil, err
	}
	for _, f := range CodeFiles {
		if p := u.part(f.Key); p != nil && len(p.Code) > MaxCode {
			return nil, fmt.Errorf("%w: the %s code is %d KB, more than %d KB", ErrInvalid, strings.ReplaceAll(f.Key, "_", " "), len(p.Code)>>10, MaxCode>>10)
		}
	}
	off := slices.Clone(st.CodeOff)
	for _, f := range CodeFiles {
		p := u.part(f.Key)
		if p == nil {
			continue
		}
		path := filepath.Join(st.Dir, f.File)
		off = slices.DeleteFunc(off, func(k string) bool { return k == f.Key })
		if strings.TrimSpace(p.Code) == "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			continue
		}
		code := p.Code
		if !strings.HasSuffix(code, "\n") {
			code += "\n"
		}
		if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
			return nil, err
		}
		if !p.Enabled {
			off = append(off, f.Key)
		}
	}
	slices.Sort(off)
	if !slices.Equal(off, st.CodeOff) {
		if len(off) == 0 {
			off = nil
		}
		st.CodeOff = off
		if err := writeSite(st.Dir, &st.SiteFile); err != nil {
			return nil, err
		}
	}
	c := ReadCustomCode(st.Dir, st.CodeOff)
	return &c, nil
}
