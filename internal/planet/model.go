package planet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MyPlanet is the part of Planet's My/<uuid>/planet.json a build reads.
// Optional fields are pointers so that absent and empty stay distinct on
// the way to planet.json, where a follower sees exactly what Planet
// writes. Token fields are not read.
type MyPlanet struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	About        string  `json:"about"`
	Domain       *string `json:"domain"`
	AuthorName   *string `json:"authorName"`
	IPNS         string  `json:"ipns"`
	Created      Date    `json:"created"`
	Updated      Date    `json:"updated"`
	TemplateName string  `json:"templateName"`
	Archived     *bool   `json:"archived"`
	DoNotIndex   *bool   `json:"doNotIndex"`

	Tags map[string]string `json:"tags"`

	PlausibleEnabled   *bool   `json:"plausibleEnabled"`
	PlausibleDomain    *string `json:"plausibleDomain"`
	PlausibleAPIServer *string `json:"plausibleAPIServer"`

	JuiceboxEnabled         *bool `json:"juiceboxEnabled"`
	JuiceboxProjectID       *int  `json:"juiceboxProjectID"`
	JuiceboxProjectIDGoerli *int  `json:"juiceboxProjectIDGoerli"`

	AcceptsDonation           *bool   `json:"acceptsDonation"`
	AcceptsDonationMessage    *string `json:"acceptsDonationMessage"`
	AcceptsDonationETHAddress *string `json:"acceptsDonationETHAddress"`

	TwitterUsername  *string `json:"twitterUsername"`
	GithubUsername   *string `json:"githubUsername"`
	TelegramUsername *string `json:"telegramUsername"`
	MastodonUsername *string `json:"mastodonUsername"`
	DiscordLink      *string `json:"discordLink"`

	PodcastCategories map[string][]string `json:"podcastCategories"`
	PodcastLanguage   *string             `json:"podcastLanguage"`
	PodcastExplicit   *bool               `json:"podcastExplicit"`

	CustomCodeHeadEnabled      *bool   `json:"customCodeHeadEnabled"`
	CustomCodeHead             *string `json:"customCodeHead"`
	CustomCodeBodyStartEnabled *bool   `json:"customCodeBodyStartEnabled"`
	CustomCodeBodyStart        *string `json:"customCodeBodyStart"`
	CustomCodeBodyEndEnabled   *bool   `json:"customCodeBodyEndEnabled"`
	CustomCodeBodyEnd          *string `json:"customCodeBodyEnd"`
}

// MyArticle is Planet's My/<uuid>/Articles/<uuid>.json.
type MyArticle struct {
	ID                     string            `json:"id"`
	ArticleType            *int              `json:"articleType"`
	Link                   *string           `json:"link"`
	Slug                   *string           `json:"slug"`
	ArticleNumber          *int              `json:"articleNumber"`
	ArticleReference       *string           `json:"articleReference"`
	ExternalLink           *string           `json:"externalLink"`
	Title                  string            `json:"title"`
	Content                string            `json:"content"`
	ContentRendered        *string           `json:"contentRendered"`
	Summary                *string           `json:"summary"`
	Created                Date              `json:"created"`
	Modified               *Date             `json:"modified"`
	Starred                *Date             `json:"starred"`
	VideoFilename          *string           `json:"videoFilename"`
	AudioFilename          *string           `json:"audioFilename"`
	Attachments            []string          `json:"attachments"`
	CIDs                   map[string]string `json:"cids"`
	Tags                   map[string]string `json:"tags"`
	IsIncludedInNavigation *bool             `json:"isIncludedInNavigation"`
	NavigationWeight       *int              `json:"navigationWeight"`
	HeroImage              *string           `json:"heroImage"`
	HeroImageWidth         *int              `json:"heroImageWidth"`
	HeroImageHeight        *int              `json:"heroImageHeight"`
	OriginalSiteName       *string           `json:"originalSiteName"`
	OriginalSiteDomain     *string           `json:"originalSiteDomain"`
	OriginalPostID         *string           `json:"originalPostID"`
	OriginalPostDate       *Date             `json:"originalPostDate"`
	Pinned                 *Date             `json:"pinned"`
}

// ArticleType values.
const (
	TypeBlog = 0 // in the feed and the index
	TypePage = 1 // in the navigation only
)

// Type is the article type with Planet's default.
func (a *MyArticle) Type() int {
	if a.ArticleType == nil {
		return TypeBlog
	}
	return *a.ArticleType
}

// HasVideo and HasAudio mirror the model's computed properties.
func (a *MyArticle) HasVideo() bool { return a.VideoFilename != nil }
func (a *MyArticle) HasAudio() bool { return a.AudioFilename != nil }

// LinkPath is the article's path: its slug when it has one, else the
// stored link, else its id.
func (a *MyArticle) LinkPath() string {
	if a.Slug != nil && *a.Slug != "" {
		return "/" + *a.Slug + "/"
	}
	if a.Link != nil && *a.Link != "" {
		return *a.Link
	}
	return "/" + a.ID + "/"
}

// SortArticles orders as Planet lists them: pinned first by pin date,
// then newest first.
func SortArticles(articles []*MyArticle) {
	sort.SliceStable(articles, func(i, j int) bool {
		a, b := articles[i], articles[j]
		switch {
		case a.Pinned == nil && b.Pinned == nil:
			return a.Created > b.Created
		case a.Pinned == nil:
			return false
		case b.Pinned == nil:
			return true
		default:
			return *a.Pinned > *b.Pinned
		}
	})
}

// FiltersCache is My/<uuid>/templateSettingsFilters.json: the colour
// filter strings Planet's solver produced, keyed like user_settings.
type FiltersCache struct {
	TemplateName  string            `json:"templateName"`
	SettingsHash  string            `json:"settingsHash"`
	SolverVersion int               `json:"solverVersion"`
	Values        map[string]string `json:"values"`
}

// Library is one planet as read from a Planet library folder.
type Library struct {
	Dir              string
	Planet           *MyPlanet
	Articles         []*MyArticle
	TemplateSettings map[string]string
	Filters          *FiltersCache
	HasAvatar        bool
	HasFavicon       bool
}

// ReadLibrary reads My/<uuid>: planet.json, every article, and the
// template settings and their filter cache when present. Articles come
// back in Planet's order.
func ReadLibrary(dir string) (*Library, error) {
	lib := &Library{Dir: dir}
	b, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil {
		return nil, err
	}
	lib.Planet = &MyPlanet{}
	if err := json.Unmarshal(b, lib.Planet); err != nil {
		return nil, fmt.Errorf("planet.json: %w", err)
	}
	ents, err := os.ReadDir(filepath.Join(dir, "Articles"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "Articles", e.Name()))
		if err != nil {
			return nil, err
		}
		a := &MyArticle{}
		if err := json.Unmarshal(b, a); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		lib.Articles = append(lib.Articles, a)
	}
	SortArticles(lib.Articles)
	if b, err := os.ReadFile(filepath.Join(dir, "templateSettings.json")); err == nil {
		json.Unmarshal(b, &lib.TemplateSettings)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "templateSettingsFilters.json")); err == nil {
		var fc FiltersCache
		if json.Unmarshal(b, &fc) == nil {
			lib.Filters = &fc
		}
	}
	_, err = os.Stat(filepath.Join(dir, "avatar.png"))
	lib.HasAvatar = err == nil
	_, err = os.Stat(filepath.Join(dir, "favicon.ico"))
	lib.HasFavicon = err == nil
	return lib, nil
}
