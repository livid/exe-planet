package build

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/stencil"
)

func TestEscapes(t *testing.T) {
	if got := HTMLEscape(`a<b>&"c" 'd' — 数`); got != "a&#x3C;b&#x3E;&#x26;&#x22;c&#x22; &#x27;d&#x27; &#x2014; &#x6570;" {
		t.Fatalf("HTMLEscape %q", got)
	}
	if got := JSEscape("a'b\"c<d>e&f=g-h;i\\j\n "); got != `a'b"c<d>e&f=g-h;i\j\u000A ` {
		t.Fatalf("JSEscape %q", got)
	}
}

func TestDateFilters(t *testing.T) {
	la, _ := time.LoadLocation("America/Los_Angeles")
	env := stencil.NewEnv(nil)
	registerPlanetFilters(env, la)
	when := time.Date(2026, 9, 20, 12, 37, 45, 0, time.UTC)
	for src, want := range map[string]string{
		"{{ d|rfc822 }}":                          "Sun, 20 Sep 2026 05:37:45 -0700",
		"{{ d|formatDateC }}":                     "2026-09-20T05:37:45-07:00",
		"{{ d|ymd }}":                             "Sep 20, 2026",
		"{{ d|mdyydot }}":                         "9.20.26",
		"{{ d|formatDate }}":                      "Sep 20, 2026 at 5:37:45 AM",
		"{{ n|hhmmss }}":                          "01:01:05",
		"{{ x|hhmmss }}":                          "00:00:00",
		"{{ s|escape }}":                          "&#x26;",
		"{{ nope|escapejs }}":                     "",
		"{{ md|md2html }}":                        "<p><em>x</em></p>\n",
		`{{ h|absoluteImageURL:"https://s",id }}`: `<p><img src="https://s/ID/a.png" /><img src="https://x/b.png" /></p>`,
	} {
		got, err := env.RenderString(src, map[string]any{"d": when, "n": 3665, "s": "&", "md": "*x*", "h": `<p><img src="a.png"><img src='https://x/b.png'></p>`, "id": "ID"})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

func TestColorFilterSolver(t *testing.T) {
	got := solveColorFilter("#ff3b30", rand.New(rand.NewSource(1)))
	if !strings.HasPrefix(got, "invert(") || !strings.Contains(got, "hue-rotate(") || !strings.HasSuffix(got, "%)") {
		t.Fatalf("filter %q", got)
	}
	s := SettingsWithFilters(map[string]string{"highlightColor": "#ff3b30", "name": "x", "badColor": "red"}, map[string]string{"highlightColorFilter": "cached"})
	if s["highlightColorFilter"] != "cached" || s["name"] != "x" || s["badColorFilter"] != "" || len(s) != 4 {
		t.Fatalf("settings %v", s)
	}
	if !SupportsHexFilter("#ABCDEF") || !SupportsHexFilter("abcdef") || SupportsHexFilter("#abc") || SupportsHexFilter("red") {
		t.Fatal("hex check")
	}
}

// The feed description is the parsed tree serialised as jsoup does:
// entities in text, quoted attributes, boolean attributes bare, void
// elements always closed with a slash, stray structural tags dropped,
// and a stray </p> making the empty paragraph HTML5 says it makes.
func TestSoupImageURLs(t *testing.T) {
	in := `<p>a & b <img src='x.png' alt="q&quot;t"/> <br/> <input disabled="" type="checkbox" /> after </head><body> ` + "`</body>`" + ` c<hr><img src="y.png"></p>`
	want := `<p>a &amp; b <img src="https://s/ID/x.png" alt="q&quot;t" /> <br /> <input disabled type="checkbox" /> after  ` + "``" + ` c</p><hr /><img src="https://s/ID/y.png" /><p></p>`
	if got := soupImageURLs(in, "https://s/ID/"); got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}
