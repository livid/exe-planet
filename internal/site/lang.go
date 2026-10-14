package site

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A post in more than one language (PLAN.md, A post in more than one
// language). index.md is the post as it was written; a file
// index.<language>.md beside it is the post in that language, its title
// in the front matter and its text under it. Everything else about the
// post — its id, its dates, its tags, its pictures, its thread on the
// hub — is index.md's, and is the same in every language.

// Version is a post in a language other than the one it was written in:
// one index.<language>.md of its folder.
type Version struct {
	// Lang is the language, a BCP 47 tag in its usual letter case
	// (zh-Hans, pt-BR) whatever case the file's name has it in.
	Lang string `json:"lang"`
	// File is the file's name in the post's folder, as it is written.
	File  string `json:"file"`
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	// Error is set when the file cannot be read as a post's text; such a
	// file is listed and not built.
	Error string `json:"error,omitempty"`
}

// langRe is the tags a file's name may carry: a two-letter language, a
// four-letter script, and a region of two letters or three digits. Three
// letters are left out on purpose: index.old.md and index.bak.md are
// files a person keeps, not languages.
var langRe = regexp.MustCompile(`^([A-Za-z]{2})(?:-([A-Za-z]{4}))?(?:-([A-Za-z]{2}|[0-9]{3}))?$`)

// LangTag reads a language tag in any letter case and writes it the way
// BCP 47 does: the language lower, the script with a capital, the region
// in capitals.
func LangTag(s string) (string, bool) {
	m := langRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	tag := strings.ToLower(m[1])
	if m[2] != "" {
		tag += "-" + strings.ToUpper(m[2][:1]) + strings.ToLower(m[2][1:])
	}
	if m[3] != "" {
		tag += "-" + strings.ToUpper(m[3])
	}
	return tag, true
}

// LangFile says which language a file in a post's folder is the post
// in: index.ja.md is "ja", index.zh-hans.md "zh-Hans". index.md, and
// any other file, is none.
func LangFile(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, "index.")
	if !ok {
		return "", false
	}
	tag, ok := strings.CutSuffix(rest, ".md")
	if !ok {
		return "", false
	}
	return LangTag(tag)
}

// LangFileName is the name a language's file is written under: the tag
// in lower case, so a folder synced to a file system that folds case
// holds one file a language.
func LangFileName(tag string) string { return "index." + strings.ToLower(tag) + ".md" }

// readVersions reads a post folder's language files, in the order of
// their tags. Two files whose names differ only in case are one
// language: the one named in lower case is read.
func readVersions(dir string) []Version {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := map[string]string{}
	for _, e := range ents {
		name := e.Name()
		tag, ok := LangFile(name)
		if !ok || !e.Type().IsRegular() {
			continue
		}
		if have, seen := names[tag]; !seen || (name == LangFileName(tag) && have != name) {
			names[tag] = name
		}
	}
	if len(names) == 0 {
		return nil
	}
	tags := make([]string, 0, len(names))
	for tag := range names {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i]) < strings.ToLower(tags[j]) })
	out := make([]Version, 0, len(tags))
	for _, tag := range tags {
		v := Version{Lang: tag, File: names[tag]}
		if data, err := os.ReadFile(filepath.Join(dir, names[tag])); err != nil {
			v.Error = err.Error()
		} else if m, body, err := ParseDoc(data); err != nil {
			v.Error = names[tag] + ": " + err.Error()
		} else {
			v.Title, v.Body = m.Title, body
		}
		out = append(out, v)
	}
	return out
}
