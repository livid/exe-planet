package site

import "testing"

func TestValidSlug(t *testing.T) {
	for _, s := range []string{"a", "charts", "hello-world", "2026-09-25-first-chart", "a1", "x-y-z"} {
		if err := ValidSlug(s); err != nil {
			t.Errorf("%q rejected: %v", s, err)
		}
	}
	for _, s := range []string{"", "-a", "a-", "A", "hello world", "héllo", "a..b", "a/b", "a_b", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if err := ValidSlug(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
	if err := ValidSlug("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Errorf("63 characters rejected: %v", err)
	}
}

func TestValidSiteSlugReserved(t *testing.T) {
	for _, s := range []string{"templates", "archived"} {
		if err := ValidSiteSlug(s); err != ErrReservedSlug {
			t.Errorf("%q: %v", s, err)
		}
	}
	if err := ValidSiteSlug("charts"); err != nil {
		t.Errorf("charts: %v", err)
	}
	if err := ValidSlug("templates"); err != nil {
		t.Errorf("a post may be called templates: %v", err)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Charts & Thoughts":      "charts-thoughts",
		"  Hello, World!  ":      "hello-world",
		"CodeLauncher":           "codelauncher",
		"数据图表":                   "",
		"--a--b--":               "a-b",
		"2026-09-25 First chart": "2026-09-25-first-chart",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	long := Slugify("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbb")
	if len(long) > 63 || ValidSlug(long) != nil {
		t.Errorf("long title gave %q", long)
	}
}
