// Package api is the daemon's HTTP API: JSON over loopback, reached by
// the desk through exe's service relay, which checks the token in front.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/livid/exe-planet/internal/importer"
	"github.com/livid/exe-planet/internal/service"
	"github.com/livid/exe-planet/internal/site"
)

// maxUpload bounds one attachment request. The Workspace sync carries
// files up to 10 MB; a bigger attachment stays on this node, which the
// editor says.
const maxUpload = 256 << 20

// Builder is what the daemon's build service offers the API.
type Builder interface {
	Build(slug string) (service.Status, error)
	Status(slug string) (service.Status, error)
	Preview(slug string, post *site.Post) (string, error)
	PublicDir(id string) string
}

// Server serves the API for one store.
type Server struct {
	Store   *site.Store
	Builder Builder
	// Templates is where a Planet template name is resolved on import.
	Templates  string
	Version    string
	Started    time.Time
	Generation func() uint64
}

// Handler mounts the routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.status)
	mux.HandleFunc("GET /v1/sites", s.listSites)
	mux.HandleFunc("POST /v1/sites", s.createSite)
	mux.HandleFunc("GET /v1/sites/{site}", s.getSite)
	mux.HandleFunc("PUT /v1/sites/{site}", s.updateSite)
	mux.HandleFunc("DELETE /v1/sites/{site}", s.deleteSite)
	mux.HandleFunc("POST /v1/sites/{site}/archive", s.archiveSite)
	mux.HandleFunc("POST /v1/sites/{site}/unarchive", s.unarchiveSite)
	mux.HandleFunc("GET /v1/sites/{site}/posts", s.listPosts)
	mux.HandleFunc("POST /v1/sites/{site}/posts", s.createPost)
	mux.HandleFunc("GET /v1/sites/{site}/posts/{post}", s.getPost)
	mux.HandleFunc("PUT /v1/sites/{site}/posts/{post}", s.updatePost)
	mux.HandleFunc("DELETE /v1/sites/{site}/posts/{post}", s.deletePost)
	mux.HandleFunc("POST /v1/sites/{site}/posts/{post}/move", s.movePost)
	mux.HandleFunc("POST /v1/sites/{site}/posts/{post}/attachments", s.addAttachments)
	mux.HandleFunc("GET /v1/sites/{site}/posts/{post}/attachments/{name}", s.getAttachment)
	mux.HandleFunc("DELETE /v1/sites/{site}/posts/{post}/attachments/{name}", s.deleteAttachment)
	mux.HandleFunc("GET /v1/templates", s.listTemplates)
	mux.HandleFunc("GET /v1/import", s.listImport)
	mux.HandleFunc("POST /v1/import", s.runImport)
	mux.HandleFunc("POST /v1/sites/{site}/build", s.buildSite)
	mux.HandleFunc("GET /v1/sites/{site}/status", s.siteStatus)
	mux.HandleFunc("POST /v1/sites/{site}/preview", s.previewPost)
	mux.HandleFunc("GET /sites/{site}/{path...}", s.serveSite)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			writeErr(w, http.StatusNotFound, errors.New("no such route"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "exe-planet %s\n", s.Version)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// fail maps the store's errors to status codes.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, site.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case errors.Is(err, site.ErrExists):
		writeErr(w, http.StatusConflict, err)
	case errors.Is(err, site.ErrInvalid), errors.Is(err, site.ErrInvalidSlug), errors.Is(err, site.ErrReservedSlug):
		writeErr(w, http.StatusBadRequest, err)
	default:
		writeErr(w, http.StatusInternalServerError, err)
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: body: %v", site.ErrInvalid, err)
	}
	return nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	live, _ := s.Store.Sites(false)
	archived, _ := s.Store.Sites(true)
	var gen uint64
	if s.Generation != nil {
		gen = s.Generation()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    s.Version,
		"workspace":  s.Store.Root,
		"sites":      len(live),
		"archived":   len(archived),
		"generation": gen,
		"uptime":     int(time.Since(s.Started).Seconds()),
	})
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	archived := r.URL.Query().Get("archived") == "1"
	sites, err := s.Store.Sites(archived)
	if err != nil {
		fail(w, err)
		return
	}
	if sites == nil {
		sites = []*site.Site{}
	}
	writeJSON(w, http.StatusOK, sites)
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string `json:"title"`
		Slug     string `json:"slug"`
		About    string `json:"about"`
		Template string `json:"template"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = site.Slugify(req.Title)
	}
	if slug == "" {
		writeErr(w, http.StatusBadRequest, errors.New("slug is required"))
		return
	}
	st, err := s.Store.CreateSite(slug, req.Title, req.About, req.Template)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.Site(r.PathValue("site"))
	if err != nil && st == nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request) {
	var u site.SiteUpdate
	if err := decode(r, &u); err != nil {
		fail(w, err)
		return
	}
	st, err := s.Store.UpdateSite(r.PathValue("site"), u)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteSite(r.PathValue("site")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) archiveSite(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.ArchiveSite(r.PathValue("site")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

func (s *Server) unarchiveSite(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.UnarchiveSite(r.PathValue("site")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
}

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := s.Store.Posts(r.PathValue("site"), r.URL.Query().Get("kind"))
	if err != nil {
		fail(w, err)
		return
	}
	if posts == nil {
		posts = []*site.Post{}
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind    string     `json:"kind"`
		Slug    string     `json:"slug"`
		Title   string     `json:"title"`
		Body    string     `json:"body"`
		Tags    []string   `json:"tags"`
		Hero    string     `json:"hero"`
		Link    string     `json:"link"`
		Created *time.Time `json:"created"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.Kind == "" {
		req.Kind = site.KindPost
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = site.Slugify(req.Title)
	}
	if slug == "" {
		writeErr(w, http.StatusBadRequest, errors.New("slug is required"))
		return
	}
	m := site.Meta{Title: strings.TrimSpace(req.Title), Tags: req.Tags, Hero: strings.TrimSpace(req.Hero), Link: strings.TrimSpace(req.Link)}
	if req.Created != nil {
		m.Created = req.Created.Truncate(time.Second)
	}
	p, err := s.Store.CreatePost(r.PathValue("site"), req.Kind, slug, m, req.Body)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.Post(r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind"))
	if err != nil && p == nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	var u site.PostUpdate
	if err := decode(r, &u); err != nil {
		fail(w, err)
		return
	}
	p, err := s.Store.UpdatePost(r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind"), u)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeletePost(r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) movePost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	p, err := s.Store.MovePost(r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind"), req.Kind)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// addAttachments takes multipart parts named file, each saved under its
// own base name, and answers with the post's attachment list.
func (s *Server) addAttachments(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("multipart body required: %w", err))
		return
	}
	siteSlug, postSlug, kind := r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind")
	var saved []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			continue
		}
		name, err := s.Store.AddAttachment(siteSlug, postSlug, kind, part.FileName(), part)
		if err != nil {
			fail(w, err)
			return
		}
		saved = append(saved, name)
	}
	if len(saved) == 0 {
		writeErr(w, http.StatusBadRequest, errors.New("no file parts"))
		return
	}
	p, err := s.Store.Post(siteSlug, postSlug, kind)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": saved, "attachments": p.Attachments})
}

func (s *Server) getAttachment(w http.ResponseWriter, r *http.Request) {
	fp, err := s.Store.AttachmentPath(r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind"), r.PathValue("name"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, fp)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteAttachment(r.PathValue("site"), r.PathValue("post"), r.URL.Query().Get("kind"), r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := s.Store.Templates()
	if err != nil {
		fail(w, err)
		return
	}
	if ts == nil {
		ts = []*site.Template{}
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) buildSite(w http.ResponseWriter, r *http.Request) {
	if s.Builder == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("no builder"))
		return
	}
	st, err := s.Builder.Build(r.PathValue("site"))
	if err != nil {
		if st.Site != "" {
			writeJSON(w, http.StatusInternalServerError, st)
			return
		}
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) siteStatus(w http.ResponseWriter, r *http.Request) {
	if s.Builder == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("no builder"))
		return
	}
	st, err := s.Builder.Status(r.PathValue("site"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// previewPost renders a body through the site's template without saving
// it and answers with the page. The app puts it in a frame with a base
// of the site's served folder so the template's assets resolve.
func (s *Server) previewPost(w http.ResponseWriter, r *http.Request) {
	if s.Builder == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("no builder"))
		return
	}
	var req struct {
		Kind    string     `json:"kind"`
		Slug    string     `json:"slug"`
		ID      string     `json:"id"`
		Title   string     `json:"title"`
		Body    string     `json:"body"`
		Tags    []string   `json:"tags"`
		Hero    string     `json:"hero"`
		Link    string     `json:"link"`
		Created *time.Time `json:"created"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	post := &site.Post{Kind: req.Kind, Slug: req.Slug, Body: req.Body}
	post.ID, post.Title, post.Tags, post.Hero, post.Link = req.ID, req.Title, req.Tags, req.Hero, req.Link
	if req.Created != nil {
		post.Created = *req.Created
	}
	if post.Slug == "" {
		post.Slug = site.Slugify(post.Title)
	}
	if post.Slug != "" {
		if p, err := s.Store.Post(r.PathValue("site"), post.Slug, ""); err == nil {
			// An existing post keeps its id and attachments in the preview.
			if post.ID == "" {
				post.ID = p.ID
			}
			post.Dir, post.Attachments = p.Dir, p.Attachments
			if post.Created.IsZero() {
				post.Created = p.Created
			}
		}
	}
	html, err := s.Builder.Preview(r.PathValue("site"), post)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(html))
}

// serveSite serves a site's built folder, for the preview frame and for
// exe's route to a local backend.
func (s *Server) serveSite(w http.ResponseWriter, r *http.Request) {
	if s.Builder == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("no builder"))
		return
	}
	st, err := s.Store.Site(r.PathValue("site"))
	if err != nil {
		fail(w, err)
		return
	}
	dir := s.Builder.PublicDir(st.ID)
	if _, err := os.Stat(dir); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("site is not built yet"))
		return
	}
	prefix := "/sites/" + r.PathValue("site") + "/"
	http.StripPrefix(prefix, http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
}

// listImport lists the planets in a Planet library folder on this
// machine: GET /v1/import?library=<path>. The path may also be a
// My/<uuid> folder, in which case that one planet is listed.
func (s *Server) listImport(w http.ResponseWriter, r *http.Request) {
	library, id, err := importer.LibraryRoot(r.URL.Query().Get("library"))
	if err != nil {
		fail(w, err)
		return
	}
	entries, err := importer.List(library)
	if err != nil {
		fail(w, err)
		return
	}
	if id != "" {
		var one []importer.Entry
		for _, e := range entries {
			if e.ID == id {
				one = append(one, e)
			}
		}
		entries = one
	}
	if entries == nil {
		entries = []importer.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"library": library, "planets": entries})
}

// runImport imports one planet: POST /v1/import {library, id, slug}. The
// library may be a My/<uuid> folder, and then id may be left out. The
// site is built right after, when a builder is there.
func (s *Server) runImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Library string `json:"library"`
		ID      string `json:"id"`
		Slug    string `json:"slug"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	library, id, err := importer.LibraryRoot(req.Library)
	if err != nil {
		fail(w, err)
		return
	}
	if req.ID != "" {
		id = req.ID
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("id is required with a library folder"))
		return
	}
	res, err := importer.Import(s.Store, s.Templates, library, id, strings.TrimSpace(req.Slug))
	if err != nil {
		fail(w, err)
		return
	}
	if s.Builder != nil {
		if st, err := s.Builder.Build(res.Site); err != nil {
			res.Warnings = append(res.Warnings, "build: "+st.Error)
		}
	}
	writeJSON(w, http.StatusCreated, res)
}
