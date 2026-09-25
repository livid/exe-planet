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
		"{% for kv in dict %}{{ kv }}{% endfor %}":                                                                               `["a", 1]["b", 2]`,
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
