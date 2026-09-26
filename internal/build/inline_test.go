package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInline(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	page := filepath.Join(root, "post")
	os.MkdirAll(assets, 0o755)
	os.MkdirAll(page, 0o755)
	os.WriteFile(filepath.Join(assets, "style.css"), []byte(`body{background:url("icon.svg") url(https://x/y.png)}`), 0o644)
	os.WriteFile(filepath.Join(assets, "icon.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(assets, "site.js"), []byte("var x=1;"), 0o644)
	os.WriteFile(filepath.Join(page, "photo.jpg"), []byte("jpeg"), 0o644)
	os.WriteFile(filepath.Join(page, "big.jpg"), make([]byte, 100), 0o644)
	os.WriteFile(filepath.Join(root, "avatar.png"), []byte("png"), 0o644)
	in := `<html><head><link rel="preload" href="../planet.json" as="fetch"><link rel="preload" href="https://x/font.woff2" as="font"><link rel="stylesheet" href="../assets/style.css?sha256=abc"><script src="../assets/site.js"></script><style>h1{background:url(../assets/icon.svg)}</style></head>` +
		`<body><img src="photo.jpg"><img src="big.jpg"><img src="../assets/icon.svg"><img src="https://x/z.png"><img src="missing.png"><img src="../avatar.png"><img src="../../etc/passwd"><a href="photo.jpg">a</a></body></html>`

	out := Inline(in, InlineOptions{PageDir: page, RootDir: root, AssetsDir: assets, MaxInline: 80})
	for _, want := range []string{
		`<style>body{background:url("data:image/svg+xml;base64,PHN2Zy8+") url(https://x/y.png)}</style>`,
		`<script>var x=1;</script>`,
		`<style>h1{background:url(data:image/svg+xml;base64,PHN2Zy8+)}</style>`,
		`<img src="data:image/jpeg;base64,anBlZw=="/>`,
		`<img src="big.jpg"/>`,
		`<img src="data:image/svg+xml;base64,PHN2Zy8+"/>`,
		`<img src="https://x/z.png"/>`,
		`<img src="missing.png"/>`,
		`<a href="photo.jpg">a</a>`,
		`<img src="data:image/png;base64,cG5n"/>`,
		`<img src="../../etc/passwd"/>`,
		`<link rel="preload" href="https://x/font.woff2" as="font"/>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	if strings.Contains(out, `planet.json`) || strings.Contains(out, `stylesheet`) || strings.Contains(out, `src="../assets/site.js"`) {
		t.Errorf("a reference survived:\n%s", out)
	}

	out = Inline(in, InlineOptions{PageDir: page, AssetsDir: assets, AttachmentURL: func(name string) string { return "/att/" + name + "?token=T" }})
	if !strings.Contains(out, `<img src="/att/photo.jpg?token=T"/>`) || !strings.Contains(out, `<img src="/att/big.jpg?token=T"/>`) || !strings.Contains(out, `<img src="missing.png"/>`) {
		t.Errorf("attachment route:\n%s", out)
	}
	if !strings.Contains(out, `<img src="data:image/svg+xml;base64,PHN2Zy8+"/>`) {
		t.Errorf("asset still folded with a route:\n%s", out)
	}
}
