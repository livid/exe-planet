package build

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/livid/exe-planet/internal/markdown"
	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/stencil"
)

// Planet's own Stencil filters (Planet/Helper/MarkdownUtils.swift),
// rendered with the node's zone as Planet renders with the Mac's.

// Medium date and date-time as macOS formats them in English, with the
// narrow no-break space that precedes AM and PM since macOS 14.
const (
	layoutYMD      = "Jan 2, 2006"
	layoutDateTime = "Jan 2, 2006 at 3:04:05 PM"
	layoutC        = "2006-01-02T15:04:05-07:00"
	layoutMDYYDot  = "1.2.06"
	layoutRFC822   = "Mon, 02 Jan 2006 15:04:05 -0700"
)

func registerPlanetFilters(env *stencil.Env, loc *time.Location) {
	date := func(layout string) stencil.Filter {
		return func(v any, _ []any) (any, error) {
			if t, ok := planet.TimeOf(v); ok {
				return t.In(loc).Format(layout), nil
			}
			return v, nil
		}
	}
	env.RegisterFilter("formatDate", date(layoutDateTime))
	env.RegisterFilter("formatDateC", date(layoutC))
	env.RegisterFilter("ymd", date(layoutYMD))
	env.RegisterFilter("mdyydot", date(layoutMDYYDot))
	env.RegisterFilter("rfc822", date(layoutRFC822))
	env.RegisterFilter("hhmmss", func(v any, _ []any) (any, error) {
		if n, ok := v.(int); ok {
			return fmt.Sprintf("%02d:%02d:%02d", n/3600, (n%3600)/60, n%60), nil
		}
		return "00:00:00", nil
	})
	env.RegisterFilter("md2html", func(v any, _ []any) (any, error) {
		if s, ok := v.(string); ok {
			return markdown.Render(s), nil
		}
		return v, nil
	})
	env.RegisterFilter("absoluteImageURL", func(v any, args []any) (any, error) {
		s, ok := v.(string)
		if !ok || len(args) < 2 {
			return v, nil
		}
		site, _ := args[0].(string)
		id := fmt.Sprint(args[1])
		return soupImageURLs(s, site+"/"+id+"/"), nil
	})
	env.RegisterFilter("escape", func(v any, _ []any) (any, error) {
		if s, ok := v.(string); ok {
			return HTMLEscape(s), nil
		}
		return v, nil
	})
	env.RegisterFilter("escapejs", func(v any, _ []any) (any, error) {
		if s, ok := v.(string); ok {
			return JSEscape(s), nil
		}
		return "", nil
	})
}

// HTMLEscape is Kitura HTMLEntities' htmlEscape() with its defaults: every
// non-ASCII scalar and the five characters &, <, >, " and ' become
// hexadecimal references with upper-case digits; other ASCII passes.
func HTMLEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch c {
			case '&', '<', '>', '"', '\'':
				fmt.Fprintf(&b, "&#x%X;", c)
			default:
				b.WriteByte(c)
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		fmt.Fprintf(&b, "&#x%X;", r)
		i += size
	}
	return b.String()
}

// JSEscape is Planet's escapejs table: Django's set plus the C0 controls,
// each as a \u escape with upper-case hex.
func JSEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\`)
		case r == '\'':
			b.WriteString(`'`)
		case r == '"':
			b.WriteString(`"`)
		case r == '>':
			b.WriteString(`>`)
		case r == '<':
			b.WriteString(`<`)
		case r == '&':
			b.WriteString(`&`)
		case r == '=':
			b.WriteString(`=`)
		case r == '-':
			b.WriteString(`-`)
		case r == ';':
			b.WriteString(`;`)
		case r == ' ':
			b.WriteString(` `)
		case r == ' ':
			b.WriteString(` `)
		case r < 32:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
