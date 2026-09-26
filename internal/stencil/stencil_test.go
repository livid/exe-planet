package stencil

import (
	"strings"
	"testing"
	"time"
)

func render(t *testing.T, src string, ctx map[string]any) string {
	t.Helper()
	e := NewEnv(nil)
	out, err := e.RenderString(src, ctx)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return out
}

func TestVariablesAndPrinting(t *testing.T) {
	ctx := map[string]any{
		"planet":  map[string]any{"name": "Charts & Thoughts", "tags": map[string]any{"a": "A"}, "n": 3, "f": 1.5, "g": 2.0, "on": true},
		"items":   []any{"x", "y", 3},
		"when":    time.Date(2026, 9, 25, 10, 0, 0, 0, time.FixedZone("PDT", -7*3600)),
		"missing": nil,
	}
	for src, want := range map[string]string{
		"{{ planet.name }}":           "Charts & Thoughts",
		"{{ planet.n }}":              "3",
		"{{ planet.f }}":              "1.5",
		"{{ planet.g }}":              "2.0",
		"{{ planet.on }}":             "true",
		"{{ nope }}":                  "",
		"{{ planet.nope.deeper }}":    "",
		"{{ missing }}":               "",
		"{{ items }}":                 `["x", "y", 3]`,
		"{{ items.count }}":           "3",
		"{{ items.first }}":           "x",
		"{{ items.last }}":            "3",
		"{{ items.1 }}":               "y",
		"{{ planet.name.count }}":     "17",
		"{{ planet.tags.count }}":     "1",
		"{{ planet.tags['a'] }}":      "A",
		"{{ planet.tags.a }}":         "A",
		"{{ when }}":                  "2026-09-25 17:00:00 +0000",
		"{{ 'lit' }}":                 "lit",
		`{{ "lit" }}`:                 "lit",
		"{{ planet.name|uppercase }}": "CHARTS & THOUGHTS",
		"{{ nope|default:'x' }}":      "x",
		"{{ items|join:', ' }}":       "x, y, 3",
		"{{ planet.tags }}":           `["a": "A"]`,
		"a {# comment #} b":           "a  b",
		"{{\n  planet.name\n}}":       "Charts & Thoughts",
	} {
		if got := render(t, src, ctx); got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

func TestSwiftDouble(t *testing.T) {
	for f, want := range map[float64]string{
		1: "1.0", 1.5: "1.5", 2: "2.0", 0.1: "0.1", 100: "100.0", -3.25: "-3.25", 0: "0.0",
		1755923353.044244: "1755923353.044244", 1790355600: "1790355600.0",
		1e15: "1000000000000000.0", 1e16: "1e+16", 1.5e16: "1.5e+16", 0.0001: "0.0001", 0.00001: "1e-05", 123456789.125: "123456789.125", 2.5e-07: "2.5e-07",
	} {
		if got := swiftDouble(f); got != want {
			t.Errorf("swiftDouble(%v) = %q, want %q", f, got, want)
		}
	}
}

func TestGraphemes(t *testing.T) {
	for in, want := range map[string]int{
		"abc": 3, "": 0, "é": 1, "e\u0301": 1, "🇯🇵🇺🇸": 2, "👨‍👩‍👧": 1, "👍🏽": 1, "☺️": 1, "a\r\nb": 3, "数据 图表": 5, "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F": 1,
	} {
		if got := len(graphemes(in)); got != want {
			t.Errorf("graphemes(%q) = %d, want %d", in, got, want)
		}
	}
	if got := render(t, "{{ s.count }}{{ s.first }}{{ s.last }}", map[string]any{"s": "e\u0301👍🏽x"}); got != "3e\u0301x" {
		t.Fatalf("string words on clusters gave %q", got)
	}
}

func TestIfSemantics(t *testing.T) {
	ctx := map[string]any{
		"s": "x", "e": "", "n0": 0, "n1": 1, "neg": -1, "f": false, "tr": true,
		"list": []any{1}, "empty": []any{}, "dict": map[string]any{"k": "v"},
		"a": 2, "b": 2, "c": "2", "title": "Hello",
	}
	for src, want := range map[string]string{
		"{% if s %}y{% else %}n{% endif %}":               "y",
		"{% if e %}y{% else %}n{% endif %}":               "n",
		"{% if n0 %}y{% else %}n{% endif %}":              "n",
		"{% if n1 %}y{% else %}n{% endif %}":              "y",
		"{% if neg %}y{% else %}n{% endif %}":             "n",
		"{% if f %}y{% else %}n{% endif %}":               "n",
		"{% if list %}y{% else %}n{% endif %}":            "y",
		"{% if empty %}y{% else %}n{% endif %}":           "n",
		"{% if dict %}y{% else %}n{% endif %}":            "y",
		"{% if nope %}y{% else %}n{% endif %}":            "n",
		"{% if not e %}y{% endif %}":                      "y",
		"{% if a == b %}y{% endif %}":                     "y",
		"{% if a == c %}y{% else %}n{% endif %}":          "n",
		"{% if c == '2' %}y{% endif %}":                   "y",
		"{% if a != c %}y{% endif %}":                     "y",
		"{% if nope == missing %}y{% endif %}":            "y",
		"{% if a > 1 %}y{% endif %}":                      "y",
		"{% if a >= 2 and a <= 2 %}y{% endif %}":          "y",
		"{% if a < 1 or s %}y{% endif %}":                 "y",
		"{% if title.count > 3 %}y{% endif %}":            "y",
		"{% if empty.count == 0 %}y{% endif %}":           "y",
		"{% if dict['k'] %}y{% endif %}":                  "y",
		"{% if 'k' in dict %}y{% endif %}":                "y",
		"{% if 1 in list %}y{% endif %}":                  "y",
		"{% if not a == b %}y{% else %}n{% endif %}":      "n",
		"{% if a == b and not e %}y{% endif %}":           "y",
		"{% ifnot e %}y{% endif %}":                       "y",
		"{% if e %}a{% elif s %}b{% else %}c{% endif %}":  "b",
		"{% if title|uppercase == 'HELLO' %}y{% endif %}": "y",
	} {
		if got := render(t, src, ctx); got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

func TestForLoop(t *testing.T) {
	ctx := map[string]any{
		"items": []any{"a", "b", "c"},
		"dict":  map[string]any{"b": 2, "a": 1},
		"none":  []any{},
	}
	for src, want := range map[string]string{
		"{% for i in items %}{{ i }}{% endfor %}":                                                                                "abc",
		"{% for i in items %}{{ forloop.counter }}{{ forloop.counter0 }}{% endfor %}":                                            "102132",
		"{% for i in items %}{% if forloop.first %}[{% endif %}{{ i }}{% if forloop.last %}]{% endif %}{% endfor %}":             "[abc]",
		"{% for i in items %}{{ forloop.length }}{% endfor %}":                                                                   "333",
		"{% for i in items where forloop.counter <= 2 %}{{ i }}{% endfor %}":                                                     "",
		"{% for i in items where i != 'b' %}{{ i }}{% endfor %}":                                                                 "ac",
		"{% for k, v in dict %}{{ k }}={{ v }};{% endfor %}":                                                                     "a=1;b=2;",
		"{% for kv in dict %}{{ kv }}{% endfor %}":                                                                               "ab",
		"{% for i in none %}x{% empty %}empty{% endfor %}":                                                                       "empty",
		"{% for i in nope %}x{% endfor %}":                                                                                       "",
		"{% for i in items %}{% for j in items %}{{ forloop.parentloop.counter }}{{ forloop.counter }} {% endfor %}{% endfor %}": "11 12 13 21 22 23 31 32 33 ",
		"{% for i in items %}{% if forloop.counter <= 2 %}{{ i }}{% endif %}{% endfor %}":                                        "ab",
	} {
		if got := render(t, src, ctx); got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

func TestInheritanceAndInclude(t *testing.T) {
	files := map[string]string{
		"base.html":         "<title>{% block title %}Base{% endblock %}</title>\n{% block body %}{% endblock %}\n{% include 'modules/nav.html' %}",
		"child.html":        "ignored text\n{% extends 'base.html' %}\n{% block title %}Child - {{ block.super }}{% endblock %}\n{% block body %}{% include \"./modules/item.html\" article %}{% endblock %}",
		"grand.html":        "{% extends \"child.html\" %}{% block title %}Grand{% endblock %}",
		"modules/nav.html":  "nav:{{ planet.name }}",
		"modules/item.html": "item:{{ title }}",
	}
	e := NewEnv(func(name string) (string, error) {
		s, ok := files[strings.TrimPrefix(name, "./")]
		if !ok {
			return "", &Error{Msg: "no template " + name}
		}
		return s, nil
	})
	ctx := map[string]any{"planet": map[string]any{"name": "P"}, "article": map[string]any{"title": "T"}}
	out, err := e.Render("child.html", ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Text before extends renders, as in Stencil; text after it does not.
	if out != "ignored text\n<title>Child - Base</title>\nitem:T\nnav:P" {
		t.Fatalf("child rendered %q", out)
	}
	out, err = e.Render("grand.html", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ignored text\n<title>Grand</title>\nitem:T\nnav:P" {
		t.Fatalf("grandchild rendered %q", out)
	}
	if _, err := e.Render("nope.html", nil); err == nil {
		t.Fatal("missing template rendered")
	}
}

func TestComponents(t *testing.T) {
	for in, want := range map[string]string{
		`if a == 0, b > 0`:             "if|a|==|0,b|>|0",
		`for a, b in dict`:             "for|a,b|in|dict",
		`include "x y.html" ctx`:       `include|"x y.html"|ctx`,
		`if x | default: "a, b"`:       `if|x|default:"a, b"`,
		`if (a or b) and c`:            "if|(|a|or|b|)|and|c",
		`if user_settings['k'] == 1`:   "if|user_settings['k']|==|1",
		`for item in  site_navigation`: "for|item|in|site_navigation",
	} {
		if got := strings.Join(components(in), "|"); got != want {
			t.Errorf("components(%q) = %q, want %q", in, got, want)
		}
	}
	// Stencil compares "0,b" as a variable, which resolves to nothing, so
	// the comma condition is false rather than an error.
	ctx := map[string]any{"a": map[string]any{"count": 0}, "b": []any{1}}
	if got := render(t, "{% if a.count == 0, b.count > 0 %}y{% else %}n{% endif %}", ctx); got != "n" {
		t.Fatalf("comma condition rendered %q", got)
	}
	if got := render(t, "{% if (a.count == 0 or nope) and b %}y{% endif %}", ctx); got != "y" {
		t.Fatalf("brackets rendered %q", got)
	}
}

func TestTrimAndErrors(t *testing.T) {
	if got := render(t, "a  {%- if 1 -%}  b  {%- endif -%}  c", nil); got != "abc" {
		t.Fatalf("trim gave %q", got)
	}
	e := NewEnv(nil)
	for _, src := range []string{"{% if %}", "{% for x %}", "{% if 1 %}", "{{ a|nope }}", "{% frobnicate %}", "{{ a"} {
		if _, err := e.RenderString(src, map[string]any{"a": 1}); err == nil {
			t.Errorf("%s: no error", src)
		}
	}
	err := &Error{Template: "blog.html", Line: 3, Msg: "x"}
	if err.Error() != "blog.html:3: x" {
		t.Fatalf("%v", err)
	}
}

// testDate is a TimeValue holding a raw double, as a Planet date does.
type testDate float64

func (d testDate) Time() time.Time                     { return time.Unix(0, int64(float64(d)*1e9)) }
func (d testDate) IntervalSince1970() float64          { return float64(d) }
func (d testDate) IntervalSinceReferenceDate() float64 { return float64(d) }
