package build

import (
	"reflect"
	"testing"
)

// The rule a reader's language is matched by, which the page script in
// the templates repeats (modules/lang.html).
func TestLangCands(t *testing.T) {
	site := []string{"en", "ja", "ko", "zh-hans", "zh-hant", "pt-br", "pt"}
	for _, c := range []struct {
		want string
		have []string
		out  []string
	}{
		{"ja", site, []string{"ja"}},
		{"ja-JP", site, []string{"ja"}},
		{"en-GB", site, []string{"en"}},
		{"zh-CN", site, []string{"zh-hans", "zh-hant"}},
		{"zh", site, []string{"zh-hans", "zh-hant"}},
		{"zh-Hans-CN", site, []string{"zh-hans", "zh-hant"}},
		{"zh-TW", site, []string{"zh-hant", "zh-hans"}},
		{"zh-HK", site, []string{"zh-hant", "zh-hans"}},
		{"zh-Hant", site, []string{"zh-hant", "zh-hans"}},
		{"zh-hans", site, []string{"zh-hans", "zh-hant"}},
		{"zh-TW", []string{"en", "zh-hans"}, []string{"zh-hans"}},
		{"zh-CN", []string{"zh-TW", "zh-CN"}, []string{"zh-CN", "zh-TW"}},
		{"pt-PT", site, []string{"pt", "pt-br"}},
		{"pt-BR", site, []string{"pt-br", "pt"}},
		{"fr", site, nil},
		{"", site, nil},
		{"en", nil, nil},
	} {
		if got := langCands(c.want, c.have); !reflect.DeepEqual(got, c.out) {
			t.Errorf("langCands(%q, %v) = %v, want %v", c.want, c.have, got, c.out)
		}
	}
	// a template's own options are given back as the template spells them
	paper := []string{"zh-Hans", "zh-Hant", "ja", "en"}
	for want, out := range map[string]string{"zh-hans": "zh-Hans", "zh-TW": "zh-Hant", "EN": "en", "ja-JP": "ja", "ko": "", "zh": "zh-Hans"} {
		if got := pickLang(want, paper); got != out {
			t.Errorf("pickLang(%q) = %q, want %q", want, got, out)
		}
	}
}

func TestLangName(t *testing.T) {
	for tag, want := range map[string]string{
		"en": "English", "zh-Hans": "简体中文", "zh-Hant": "繁體中文", "zh-TW": "繁體中文", "ja": "日本語",
		"pt-BR": "Português", "zh-Hans-CN": "中文", "tlh": "tlh", "xx-YY": "xx-YY",
	} {
		if got := langName(tag); got != want {
			t.Errorf("langName(%q) = %q, want %q", tag, got, want)
		}
	}
}
