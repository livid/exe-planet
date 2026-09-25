package build

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/livid/exe-planet/internal/planet"
)

var imageSuffixes = []string{".avif", ".jpeg", ".jpg", ".png", ".webp", ".gif", ".tiff", ".heic"}

func isImageName(name string) bool {
	l := strings.ToLower(name)
	for _, s := range imageSuffixes {
		if strings.HasSuffix(l, s) {
			return true
		}
	}
	return false
}

// imageSize reads a file's pixel size when Go can decode its header.
func imageSize(path string) (w, h int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// detectHero is Planet's getHeroImage: the stored hero, else the video
// thumbnail, else the PDF preview, else the first image attachment of at
// least 600 by 400, else the first image at all. dir holds the article's
// files; base is the article's browser URL.
func detectHero(a *planet.MyArticle, dir, base string) *planet.Hero {
	name := ""
	switch {
	case a.HeroImage != nil && *a.HeroImage != "":
		name = *a.HeroImage
	case a.HasVideo():
		name = "_videoThumbnail.png"
	case hasPDF(a):
		name = "_preview.png"
	default:
		first := ""
		for _, att := range a.Attachments {
			if !isImageName(att) {
				continue
			}
			if first == "" {
				first = att
			}
			if dir != "" {
				if w, h, ok := imageSize(filepath.Join(dir, att)); ok && w >= 600 && h >= 400 {
					name = att
					break
				}
			}
		}
		if name == "" {
			name = first
		}
	}
	if name == "" {
		return nil
	}
	hero := &planet.Hero{Filename: name, URL: base + name, Width: a.HeroImageWidth, Height: a.HeroImageHeight}
	if dir != "" {
		if w, h, ok := imageSize(filepath.Join(dir, name)); ok {
			hero.Width, hero.Height = &w, &h
		}
	}
	return hero
}

func hasPDF(a *planet.MyArticle) bool {
	for _, att := range a.Attachments {
		if strings.HasSuffix(att, ".pdf") {
			return true
		}
	}
	return false
}
