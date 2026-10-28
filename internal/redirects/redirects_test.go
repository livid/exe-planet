package redirects

import "testing"

func TestParseAndMatch(t *testing.T) {
	rules := Parse([]byte(`# old Zola addresses
/blog/            /archive.html
/blog/*           /:splat           301
/news/:year/:slug /:year-:slug      302
/old              https://example.com/new
/away             https://example.com/  200
/pretty           /index.html       200
/gone             /404.html         410
/weird            /x                999
not-a-rule
/a /b /c /d
/*                /404.html         404
`))
	if len(rules) != 7 {
		t.Fatalf("rules %+v", rules)
	}
	for _, c := range []struct {
		path, to string
		status   int
	}{
		{"/blog/", "/archive.html", 301},
		{"/blog", "/archive.html", 301},
		{"/blog/20220102/", "/20220102/", 301},
		{"/blog/20220102", "/20220102", 301},
		{"/blog/a/b/", "/a/b/", 301},
		{"/news/2022/hello", "/2022-hello", 302},
		{"/old", "https://example.com/new", 301},
		{"/pretty", "/index.html", 200},
		{"/gone/", "/404.html", 410},
		{"/away", "/404.html", 404}, // a 200 to a URL is not a rule
		{"/nothing/here", "/404.html", 404},
		{"/", "/404.html", 404},
	} {
		to, status, ok := Match(rules, c.path)
		if !ok || to != c.to || status != c.status {
			t.Errorf("%s: %q %d %v, want %q %d", c.path, to, status, ok, c.to, c.status)
		}
	}
	if _, _, ok := Match(Parse([]byte("/blog/* /:splat")), "/other"); ok {
		t.Error("a path outside the rule matched")
	}
	if _, _, ok := Match(Parse([]byte("/news/:slug /x")), "/news/a/b"); ok {
		t.Error("a placeholder took two segments")
	}
}
