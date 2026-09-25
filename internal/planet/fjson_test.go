package planet

import (
	"testing"
	"time"
)

// The layout is Foundation's, as read from a real planet.json: sorted
// keys, " : ", empty containers around a blank line, whole-number
// doubles without a point, slashes and non-ASCII untouched.
func TestMarshalFoundationLayout(t *testing.T) {
	got := string(MarshalFoundation(map[string]any{
		"tags":        map[string]string{},
		"attachments": []string{},
		"cids":        map[string]string{"a.png": "Qm1"},
		"created":     Date(675853734),
		"updated":     Date(796032373.386509),
		"n":           63,
		"link":        "/006662F5/",
		"title":       "Charts & Thoughts — 数据 \"q\"\n",
		"list":        []any{1, "x", map[string]any{}},
		"absent":      nil,
		"on":          true,
	}))
	want := `{
  "attachments" : [

  ],
  "cids" : {
    "a.png" : "Qm1"
  },
  "created" : 675853734,
  "link" : "/006662F5/",
  "list" : [
    1,
    "x",
    {

    }
  ],
  "n" : 63,
  "on" : true,
  "tags" : {

  },
  "title" : "Charts & Thoughts — 数据 \"q\"\n",
  "updated" : 796032373.386509
}`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDates(t *testing.T) {
	d := Date(811600665)
	if got := d.Time().Format(time.RFC3339); got != "2026-09-20T12:37:45Z" {
		t.Fatalf("Time %s", got)
	}
	if DateOf(d.Time()) != d {
		t.Fatalf("round trip %v", DateOf(d.Time()))
	}
	frac := Date(796032373.386509)
	if DateOf(frac.Time()).String() != "796032373.386509" {
		t.Fatalf("fraction round trip %s", DateOf(frac.Time()))
	}
	if Date(1.5).String() != "1.5" || Date(2).String() != "2" {
		t.Fatal("number form")
	}
}
