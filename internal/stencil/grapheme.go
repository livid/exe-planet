package stencil

import "unicode"

// graphemes splits a string into extended grapheme clusters closely
// enough for Swift's String.count on real text: a base character keeps
// its combining marks, variation selectors, emoji modifiers and anything
// joined to it by a zero-width joiner, regional indicators pair up, and
// CR LF is one cluster. Hangul jamo composition and the rarer rules of
// UAX #29 are not covered; the corpus test will say if that ever shows.
func graphemes(s string) []string {
	var out []string
	runes := []rune(s)
	for i := 0; i < len(runes); {
		start := i
		r := runes[i]
		i++
		switch {
		case r == '\r' && i < len(runes) && runes[i] == '\n':
			i++
		case isRegionalIndicator(r) && i < len(runes) && isRegionalIndicator(runes[i]):
			i++
		}
		for i < len(runes) {
			n := runes[i]
			if extends(n) {
				i++
				continue
			}
			if n == 0x200D && i+1 < len(runes) {
				i += 2 // the joiner and what it joins
				continue
			}
			break
		}
		out = append(out, string(runes[start:i]))
	}
	return out
}

func extends(r rune) bool {
	switch {
	case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Mc, r), unicode.Is(unicode.Me, r):
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0x1F3FB && r <= 0x1F3FF, r >= 0xE0100 && r <= 0xE01EF:
		return true
	case r >= 0xE0020 && r <= 0xE007F: // tag sequences, as in the flag of England
		return true
	}
	return false
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }
