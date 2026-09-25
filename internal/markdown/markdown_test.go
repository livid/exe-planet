package markdown

import (
	"strings"
	"testing"
)

func TestRenderMatchesPlanetOptions(t *testing.T) {
	for md, want := range map[string]string{
		"# Hi\n":                              "<h1>Hi</h1>\n",
		"a\nb\n":                              "<p>a<br />\nb</p>\n",
		"<div>raw</div>\n":                    "<div>raw</div>\n",
		"~~x~~":                               "<p><del>x</del></p>\n",
		"- [ ] a\n":                           "<ul>\n<li><input disabled=\"\" type=\"checkbox\" /> a</li>\n</ul>\n",
		"| a |\n|---|\n| b |\n":               "<table>\n<thead>\n<tr>\n<th>a</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>b</td>\n</tr>\n</tbody>\n</table>\n",
		"see https://example.com now":         "<p>see <a href=\"https://example.com\">https://example.com</a> now</p>\n",
		"x[^1]\n\n[^1]: note\n":               "<p>x<sup id=\"fnref:1\"><a href=\"#fn:1\" class=\"footnote-ref\" role=\"doc-noteref\">1</a></sup></p>\n<div class=\"footnotes\" role=\"doc-endnotes\">\n<hr />\n<ol>\n<li id=\"fn:1\">\n<p>note&#160;<a href=\"#fnref:1\" class=\"footnote-backref\" role=\"doc-backlink\">&#x21a9;&#xfe0e;</a></p>\n</li>\n</ol>\n</div>\n",
		"https://www.youtube.com/watch?v=abc": strings.Replace(embed, "$1", "abc", 1),
		"":                                    "",
	} {
		if got := Render(md); got != want {
			t.Errorf("Render(%q):\n got %q\nwant %q", md, got, want)
		}
	}
}
