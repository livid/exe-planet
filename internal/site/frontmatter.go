package site

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Meta is a post's front matter: the lines between the two `---` fences
// at the top of index.md, `key: value` each, no YAML library. Keys the
// daemon does not know are kept in Extra and written back unchanged, so
// a person's own fields survive a round trip through the editor.
type Meta struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Hero     string    `json:"hero,omitempty"`
	Link     string    `json:"link,omitempty"`
	// Hub is the post's thread on the exe hub, the address of the post
	// that announced it (PLAN.md, Replies from the hub). The announcing
	// node writes it; a person may point it elsewhere or take it out.
	Hub   string            `json:"hub,omitempty"`
	Extra map[string]string `json:"extra,omitempty"`
}

// dateLayouts are the forms a person may type; the daemon writes RFC 3339.
var dateLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"}

// ParseDate reads a front-matter date. Forms without a zone are local.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			if layout != time.RFC3339 {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q", s)
}

// BodyOffset is where the body starts in index.md as the file has it:
// after the front matter ParseDoc reads, 0 with none, -1 when it is not
// closed. For a change made in place, where ParseDoc's text, with its
// line endings made one, no longer counts in the file's own bytes.
func BodyOffset(data []byte) int {
	nl := "\n"
	switch {
	case bytes.HasPrefix(data, []byte("---\r\n")):
		nl = "\r\n"
	case !bytes.HasPrefix(data, []byte("---\n")):
		return 0
	}
	start := 3 + len(nl)
	if end := bytes.Index(data[start:], []byte(nl+"---"+nl)); end >= 0 {
		return start + end + 2*len(nl) + 3
	}
	if bytes.HasSuffix(data[start:], []byte(nl+"---")) {
		return len(data)
	}
	return -1
}

// Issue writes an id, a created date or both into index.md as the file
// has it, for a post made by hand that lacks them (Store.AdoptPosts): a
// line each in its front matter, the id first and the date last, or a
// front matter of just those lines before a file that has none. Every
// other byte stays, the line endings the file's own. An "id:" line left
// empty is filled, not doubled. Empty id or created writes none.
func Issue(data []byte, id, created string) []byte {
	text := string(data)
	nl := "\n"
	if strings.HasPrefix(text, "---\r\n") || (!strings.HasPrefix(text, "---\n") && strings.Contains(text, "\r\n")) {
		nl = "\r\n"
	}
	var add []string
	if id != "" {
		add = append(add, "id: "+id)
	}
	if created != "" {
		add = append(add, "created: "+created)
	}
	if len(add) == 0 {
		return data
	}
	open := "---" + nl
	if !strings.HasPrefix(text, open) {
		if text != "" {
			text = nl + text
		}
		return []byte(open + strings.Join(add, nl) + nl + open + text)
	}
	rest := text[len(open):]
	end, tail := strings.Index(rest, nl+"---"+nl), ""
	switch {
	case end >= 0:
		tail = rest[end:]
	case strings.HasSuffix(rest, nl+"---"):
		end = len(rest) - len(nl) - 3
		tail = rest[end:]
	default:
		return data // not closed: it is shown as it is, invalid
	}
	lines := strings.Split(rest[:end], nl)
	if id != "" {
		filled := false
		for i, line := range lines {
			if key, value, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) == "id" && strings.TrimSpace(value) == "" {
				lines[i], filled = "id: "+id, true
				break
			}
		}
		if !filled {
			lines = append([]string{"id: " + id}, lines...)
		}
	}
	if created != "" {
		lines = append(lines, "created: "+created)
	}
	return []byte(open + strings.Join(lines, nl) + tail)
}

// ParseDoc splits index.md into its front matter and body. A file with no
// front matter is all body with an empty Meta.
func ParseDoc(data []byte) (Meta, string, error) {
	var m Meta
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return m, text, nil
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	var head string
	switch {
	case end >= 0:
		head, rest = rest[:end], rest[end+5:]
	case strings.HasSuffix(rest, "\n---"):
		head, rest = rest[:len(rest)-4], ""
	default:
		return m, "", fmt.Errorf("front matter is not closed")
	}
	for n, line := range strings.Split(head, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return m, "", fmt.Errorf("front matter line %d: %q is not key: value", n+1, line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		var err error
		switch key {
		case "id":
			m.ID = value
		case "title":
			m.Title = value
		case "created":
			m.Created, err = ParseDate(value)
		case "modified":
			m.Modified, err = ParseDate(value)
		case "tags":
			m.Tags = splitTags(value)
		case "hero":
			m.Hero = value
		case "link":
			m.Link = value
		case "hub":
			m.Hub = value
		default:
			if m.Extra == nil {
				m.Extra = map[string]string{}
			}
			m.Extra[key] = value
		}
		if err != nil {
			return m, "", fmt.Errorf("front matter %s: %w", key, err)
		}
	}
	rest = strings.TrimPrefix(rest, "\n")
	return m, rest, nil
}

// FormatDoc writes index.md: the known keys in a fixed order, the extra
// ones sorted, one blank line, then the body ending in a newline.
func FormatDoc(m Meta, body string) []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	line("id", m.ID)
	line("title", m.Title)
	if !m.Created.IsZero() {
		line("created", formatDate(m.Created))
	}
	if !m.Modified.IsZero() {
		line("modified", formatDate(m.Modified))
	}
	line("tags", strings.Join(m.Tags, ", "))
	line("hero", m.Hero)
	line("link", m.Link)
	line("hub", m.Hub)
	keys := make([]string, 0, len(m.Extra))
	for k := range m.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		line(k, m.Extra[k])
	}
	b.WriteString("---\n")
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.Bytes()
}

// formatDate writes RFC 3339, keeping fractional seconds only when there
// are any: an imported Planet date has microseconds a follower compares.
func formatDate(t time.Time) string {
	if t.Nanosecond() == 0 {
		return t.Format(time.RFC3339)
	}
	return t.Format("2006-01-02T15:04:05.999999Z07:00")
}

func splitTags(s string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" && !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			tags = append(tags, t)
		}
	}
	return tags
}
