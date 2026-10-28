// Package redirects reads a site's _redirects file, the format IPFS
// gateways and Netlify share, so the daemon answers an exposed site's
// old addresses the way a gateway answers its CID (PLAN.md, A site's own
// files).
//
// One rule a line: `from to [status]`. from is a path that may hold
// :name segments and end in *; to is a path or a URL that may use :name
// and :splat. The status is 301 unless given: 301, 302, 303, 307 and 308
// redirect, 200 answers with to's file, 404, 410 and 451 answer with
// to's file under that status. # starts a comment. A rule applies only
// to a path the site has no file for, and the first that matches wins.
package redirects

import (
	"strconv"
	"strings"
)

// MaxSize is the most of a _redirects file that is read, as IPFS reads.
const MaxSize = 64 << 10

// Rule is one line of a _redirects file.
type Rule struct {
	From   string
	To     string
	Status int
}

var statuses = map[int]bool{200: true, 301: true, 302: true, 303: true, 307: true, 308: true, 404: true, 410: true, 451: true}

// Redirect says whether a status sends the reader to another address.
func Redirect(status int) bool { return status >= 300 && status < 400 }

// Parse reads a _redirects file. A line it cannot read is skipped, so one
// mistake costs that rule and no other.
func Parse(data []byte) []Rule {
	if len(data) > MaxSize {
		data = data[:MaxSize]
	}
	var rules []Rule
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 || len(f) > 3 || !strings.HasPrefix(f[0], "/") {
			continue
		}
		r := Rule{From: f[0], To: f[1], Status: 301}
		if len(f) == 3 {
			n, err := strconv.Atoi(f[2])
			if err != nil || !statuses[n] {
				continue
			}
			r.Status = n
		}
		// only a redirect may leave the site; the others answer with a file
		if !strings.HasPrefix(r.To, "/") && !(Redirect(r.Status) && (strings.HasPrefix(r.To, "https://") || strings.HasPrefix(r.To, "http://"))) {
			continue
		}
		rules = append(rules, r)
	}
	return rules
}

// Match finds the first rule for path and gives its target with the
// placeholders filled in. A trailing slash is not part of a match.
func Match(rules []Rule, path string) (to string, status int, ok bool) {
	segs, slash := split(path), strings.HasSuffix(path, "/")
	for _, r := range rules {
		vals, ok := match(split(r.From), segs, slash)
		if !ok {
			continue
		}
		to := r.To
		// :splat before the shorter names it could hold
		if v, ok := vals["splat"]; ok {
			to = strings.ReplaceAll(to, ":splat", v)
		}
		for k, v := range vals {
			if k != "splat" {
				to = strings.ReplaceAll(to, ":"+k, v)
			}
		}
		return to, r.Status, true
	}
	return "", 0, false
}

func split(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// match fills a pattern's placeholders from a path's segments; the splat
// keeps the path's trailing slash, so /blog/x/ goes to /x/ in one step.
func match(pat, segs []string, slash bool) (map[string]string, bool) {
	vals := map[string]string{}
	for i, p := range pat {
		if p == "*" && i == len(pat)-1 {
			vals["splat"] = strings.Join(segs[i:], "/")
			if slash && i < len(segs) {
				vals["splat"] += "/"
			}
			return vals, true
		}
		if i >= len(segs) {
			return nil, false
		}
		switch {
		case strings.HasPrefix(p, ":") && len(p) > 1:
			vals[p[1:]] = segs[i]
		case p != segs[i]:
			return nil, false
		}
	}
	return vals, len(segs) == len(pat)
}
