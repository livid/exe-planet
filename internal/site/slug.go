// Package site is the file model: sites and posts as folders under the
// Workspace's Planet directory, with site.json and index.md as the only
// records. There is no database; the folder tree is the truth.
package site

import (
	"errors"
	"regexp"
	"strings"
)

// slugRe is a DNS label: a site slug doubles as the default subdomain.
var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// reservedSiteSlugs are folders under Planet/ that are not sites.
var reservedSiteSlugs = map[string]bool{"templates": true, "archived": true}

// ErrInvalidSlug is returned for a slug that is not lower-case letters,
// digits and hyphens, or that starts or ends with a hyphen, or that is
// longer than 63 characters.
var ErrInvalidSlug = errors.New("slug must be lowercase letters, digits and hyphens, not starting or ending with a hyphen, at most 63 characters")

// ErrReservedSlug names a site slug that is a reserved folder.
var ErrReservedSlug = errors.New("slug is reserved")

// ValidSlug checks a post or template slug.
func ValidSlug(s string) error {
	if !slugRe.MatchString(s) {
		return ErrInvalidSlug
	}
	return nil
}

// ValidSiteSlug checks a site slug: a valid slug that is not reserved.
func ValidSiteSlug(s string) error {
	if err := ValidSlug(s); err != nil {
		return err
	}
	if reservedSiteSlugs[s] {
		return ErrReservedSlug
	}
	return nil
}

// Slugify proposes a slug for a title: lower-cased, every run of
// characters outside a-z and 0-9 folded to one hyphen, trimmed, cut to
// 63. It can be empty (a title in another script), and then the caller
// asks for a slug instead of inventing one.
func Slugify(title string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			hyphen = false
		default:
			if b.Len() > 0 && !hyphen {
				b.WriteByte('-')
				hyphen = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-")
	}
	return s
}
