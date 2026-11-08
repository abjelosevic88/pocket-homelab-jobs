package server

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- CVs, cover letters and other papers ----
//
// A paper is its text (what you paste, what the AI reads and writes) and,
// if you want, its file (the PDF you send). It can come from Pocket
// Documents: its file and the text Documents read from it.

const maxDocFile = 15 << 20

func (s *Server) handleListDocs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListDocs(ctx, actorID(ctx), r.URL.Query().Get("kind"))
	if err != nil {
		s.fail(w, err, "papers")
		return
	}
	if !queryBool(r, "full") {
		for _, d := range list {
			d.Body = clip(d.Body, 300)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, err := s.store.GetDoc(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "paper")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type docIn struct {
	Kind      *string `json:"kind"`
	Name      *string `json:"name"`
	Body      *string `json:"body"`
	IsDefault *bool   `json:"is_default"`
}

func (in docIn) apply(d *store.Doc) error {
	if in.Kind != nil {
		if !oneOf(*in.Kind, store.DocKinds) {
			return &userErr{"a paper is a cv, a letter or other"}
		}
		d.Kind = *in.Kind
	}
	if in.Name != nil {
		d.Name = clip(strings.TrimSpace(*in.Name), 200)
	}
	if in.Body != nil {
		d.Body = clip(strings.TrimSpace(*in.Body), 100000)
	}
	if d.Name == "" {
		d.Name = map[string]string{"cv": "My CV", "letter": "Cover letter"}[d.Kind]
		if d.Name == "" {
			d.Name = "A paper"
		}
	}
	return nil
}

func (s *Server) handleCreateDoc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in docIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d := &store.Doc{UserID: uid, Kind: "cv"}
	if err := in.apply(d); err != nil {
		s.userFail(w, err, "paper")
		return
	}
	if in.IsDefault != nil {
		d.IsDefault = *in.IsDefault
	}
	if _, err := s.store.DefaultCV(ctx, uid); err != nil && d.Kind == "cv" {
		d.IsDefault = true // the first CV is the main one
	}
	if err := s.store.CreateDoc(ctx, d, nil); err != nil {
		s.fail(w, err, "paper")
		return
	}
	d, _ = s.store.GetDoc(ctx, uid, d.ID)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleUpdateDoc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDoc(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "paper")
		return
	}
	var in docIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(d); err != nil {
		s.userFail(w, err, "paper")
		return
	}
	if err := s.store.UpdateDoc(ctx, d); err != nil {
		s.fail(w, err, "paper")
		return
	}
	if in.IsDefault != nil && *in.IsDefault {
		_ = s.store.SetDefaultDoc(ctx, uid, d.ID)
	}
	d, _ = s.store.GetDoc(ctx, uid, d.ID)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteDoc(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "paper")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDocFile takes a file for a paper (the body is the file; its name
// comes in ?name=). Plain text and Markdown files also become its text.
func (s *Server) handleDocFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDoc(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "paper")
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.store.SetDocFile(ctx, uid, d.ID, nil, "", ""); err != nil {
			s.fail(w, err, "paper")
			return
		}
		d, _ = s.store.GetDoc(ctx, uid, d.ID)
		writeJSON(w, http.StatusOK, d)
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxDocFile+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the file")
		return
	}
	if len(b) == 0 {
		writeErr(w, http.StatusBadRequest, "the file is empty")
		return
	}
	if len(b) > maxDocFile {
		writeErr(w, http.StatusRequestEntityTooLarge, "a file is up to 15 MB")
		return
	}
	name := clip(filepath.Base(strings.TrimSpace(r.URL.Query().Get("name"))), 200)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	typ := r.Header.Get("Content-Type")
	if typ == "" || strings.HasPrefix(typ, "application/octet-stream") {
		if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
			typ = t
		}
	}
	if typ == "" {
		typ = http.DetectContentType(b)
	}
	if err := s.store.SetDocFile(ctx, uid, d.ID, b, name, typ); err != nil {
		s.fail(w, err, "paper")
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	if d.Body == "" && (strings.HasPrefix(typ, "text/") || ext == ".md" || ext == ".txt") {
		d.Body = clip(string(b), 100000)
		_ = s.store.UpdateDoc(ctx, d)
	}
	d, _ = s.store.GetDoc(ctx, uid, d.ID)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDocDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, name, typ, err := s.store.DocFile(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "file")
		return
	}
	w.Header().Set("Content-Type", firstNonEmpty(typ, "application/octet-stream"))
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	disp := "attachment"
	if queryBool(r, "inline") {
		disp = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-cache")
	_, _ = w.Write(b)
}

// ---- from Pocket Documents ----

// handleDocumentsSearch lists papers in Pocket Documents to bring in.
func (s *Server) handleDocumentsSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	st := c.status(ctx, appDocuments)
	if st.Via == "" || !st.Can(scopeDocumentsRead) {
		writeJSON(w, http.StatusOK, map[string]any{"state": st.State, "detail": st.Detail, "fix": st.Fix, "items": []any{}})
		return
	}
	sib, err := c.conn(ctx, appDocuments, scopeDocumentsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "off", "detail": err.Error(), "items": []any{}})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = "cv"
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/documents", url.Values{"q": {q}, "limit": {"25"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "error", "detail": "Pocket Documents did not answer: " + err.Error(), "items": []any{}})
		return
	}
	items := []map[string]any{}
	for _, d := range rows(v, "results", "documents") {
		items = append(items, map[string]any{"id": idOf(d), "title": str(d, "title"), "created": day10(str(d, "created")), "mime": str(d, "mime"),
			"size": num(d, "size"), "name": str(d, "original_name"), "url": linkTo(sib, "/documents/"+idOf(d))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok", "items": items, "open_url": st.Open})
}

// handleDocumentsImport brings one paper from Pocket Documents: its file
// and the text Documents read from it.
func (s *Server) handleDocumentsImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.ID) == "" {
		writeErr(w, http.StatusBadRequest, "pick a paper from Pocket Documents")
		return
	}
	if !oneOf(in.Kind, store.DocKinds) {
		in.Kind = "cv"
	}
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appDocuments, scopeDocumentsRead)
	if err != nil {
		s.makeFail(w, appDocuments, err)
		return
	}
	id := url.PathEscape(strings.TrimSpace(in.ID))
	v, err := getJSONFrom(ctx, sib, "/api/v1/documents/"+id, nil)
	if err != nil {
		s.makeFail(w, appDocuments, err)
		return
	}
	meta, _ := v.(map[string]any)
	text, _, err := sib.client.Bytes(ctx, "/api/v1/documents/"+id+"/text", 2<<20)
	if err != nil {
		s.makeFail(w, appDocuments, err)
		return
	}
	file, typ, err := sib.client.Bytes(ctx, "/api/v1/documents/"+id+"/download", maxDocFile)
	if err != nil {
		file = nil // the text alone is enough
	}
	name := firstNonEmpty(strings.TrimSpace(in.Name), str(meta, "title"), "My CV")
	d := &store.Doc{UserID: uid, Kind: in.Kind, Name: clip(name, 200), Body: clip(strings.TrimSpace(string(text)), 100000),
		FileName: firstNonEmpty(str(meta, "original_name"), name), FileType: firstNonEmpty(str(meta, "mime"), typ),
		DocRef: strings.TrimSpace(in.ID), DocURL: linkTo(sib, "/documents/"+id)}
	if _, err := s.store.DefaultCV(ctx, uid); err != nil && d.Kind == "cv" {
		d.IsDefault = true
	}
	if err := s.store.CreateDoc(ctx, d, file); err != nil {
		s.fail(w, err, "paper")
		return
	}
	d, _ = s.store.GetDoc(ctx, uid, d.ID)
	if d.Body == "" {
		writeJSON(w, http.StatusCreated, map[string]any{"doc": d, "warning": fmt.Sprintf("Pocket Documents has no text for “%s” yet: paste it here so the AI can read it", name)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"doc": d})
}
