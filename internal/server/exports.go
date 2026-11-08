package server

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- your job search, out and in ----

// exportDoc is one person's stages, papers and jobs (with their timelines,
// interviews and offers), as JSON.
type exportDoc struct {
	App      string         `json:"app"`
	Version  string         `json:"version"`
	Exported string         `json:"exported_at"`
	Stages   []*store.Stage `json:"stages"`
	Docs     []exportPaper  `json:"docs"`
	Jobs     []exportJob    `json:"jobs"`
}

type exportPaper struct {
	store.Doc
	Key string `json:"key"`
}

type exportJob struct {
	store.Job
	Key        string             `json:"key"`
	CVKey      string             `json:"cv_key,omitempty"`
	Timeline   []*store.Activity  `json:"timeline"`
	Interviews []*store.Interview `json:"interview_list"`
	Offer      *store.Offer       `json:"offer,omitempty"`
	Elsewhere  []*store.Made      `json:"elsewhere"`
	StageLabel string             `json:"stage"`
	StageKind  string             `json:"stage_kind_name"`
}

func docKey(d *store.Doc) string {
	sum := sha1.Sum([]byte(d.Kind + "\x1f" + d.Name + "\x1f" + d.Body))
	return "doc:" + hex.EncodeToString(sum[:])[:16]
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-jobs", Version: Version, Exported: store.Now(), Docs: []exportPaper{}, Jobs: []exportJob{}}
	doc.Stages, _ = s.store.ListStages(ctx, uid)
	docs, _ := s.store.ListDocs(ctx, uid, "")
	keys := map[int64]string{}
	for _, d := range docs {
		keys[d.ID] = docKey(d)
		doc.Docs = append(doc.Docs, exportPaper{Doc: *d, Key: keys[d.ID]})
	}
	all, _ := s.store.AllJobs(ctx, uid)
	for _, j := range all {
		key := firstNonEmpty(j.ClientKey, fmt.Sprintf("jobs:%d", j.ID))
		acts, _ := s.store.ListActivities(ctx, uid, j.ID, 1000)
		made, _ := s.store.ListMade(ctx, uid, j.ID)
		ivs, _ := s.store.ListInterviews(ctx, uid, store.InterviewFilter{JobID: j.ID})
		ej := exportJob{Job: *j, Key: key, CVKey: keys[j.CVID], Timeline: acts, Interviews: ivs, Elsewhere: made, StageLabel: j.StageName, StageKind: j.StageKind}
		if o, err := s.store.OfferFor(ctx, uid, j.ID); err == nil {
			ej.Offer = o
		}
		doc.Jobs = append(doc.Jobs, ej)
	}
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-jobs-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

var csvHead = []string{"Job title", "Company", "Stage", "Status", "Location", "Work mode", "Employment", "URL", "Min salary", "Max salary", "Currency",
	"Salary period", "Source", "Excitement", "Apply by", "Date applied", "Contact", "Email", "Phone", "Next step", "Next step due", "Interviews",
	"Fit score", "Tags", "End reason", "Notes", "Added", "Closed"}

// handleExportCSV is every job in one table (a spreadsheet opens it).
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := s.store.AllJobs(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-jobs.csv"`)
	_, _ = w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHead)
	amt := func(v float64) string {
		if v == 0 {
			return ""
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	for _, j := range all {
		fit := ""
		if j.FitScore >= 0 {
			fit = strconv.Itoa(j.FitScore)
		}
		_ = cw.Write([]string{j.Title, j.Company, j.StageName, j.Status, j.Location, j.WorkMode, j.Employment, j.URL, amt(j.MinAmount), amt(j.MaxAmount),
			j.Currency, j.SalaryPeriod, j.Source, strconv.Itoa(j.Excitement), j.ApplyBy, j.AppliedOn, j.ContactName, j.ContactEmail, j.ContactPhone,
			j.NextAction, j.NextDue, strconv.Itoa(j.Interviews), fit, strings.Join(j.Tags, ", "), j.EndReason, j.Notes, day10(j.CreatedAt), day10(j.ClosedAt)})
	}
	cw.Flush()
}

// handleImport takes a Pocket Jobs export (again and again: nothing is
// doubled), or jobs as CSV from Huntr, Teal, a spreadsheet.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		if data, err = io.ReadAll(f); err != nil {
			writeErr(w, http.StatusBadRequest, "could not read the file")
			return
		}
	} else {
		b, err := s.readBody(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		data = b
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		var doc exportDoc
		if err := json.Unmarshal(data, &doc); err != nil || doc.App != "pocket-jobs" {
			writeErr(w, http.StatusBadRequest, "that is not a Pocket Jobs export")
			return
		}
		writeJSON(w, http.StatusOK, s.importPocket(ctx, uid, doc))
		return
	}
	out, err := s.importCSV(ctx, uid, data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// stageFor finds (or adds) the stage a job goes to on import.
func (s *Server) stageFor(ctx context.Context, uid int64, name, kind string) int64 {
	name = strings.TrimSpace(name)
	if st, err := s.store.StageByName(ctx, uid, name); err == nil && name != "" {
		return st.ID
	}
	if !oneOf(kind, store.StageKinds) {
		kind = kindFromWord(name)
	}
	if kind != "" {
		if st, err := s.store.FirstStage(ctx, uid, kind); err == nil {
			return st.ID
		}
	}
	if name != "" {
		st := &store.Stage{UserID: uid, Name: clip(name, 60), Kind: firstNonEmpty(kind, store.KindActive)}
		if err := s.store.CreateStage(ctx, st); err == nil {
			return st.ID
		}
	}
	st, _ := s.store.FirstStage(ctx, uid, store.KindSaved)
	if st == nil {
		return 0
	}
	return st.ID
}

func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) map[string]any {
	added, skipped, papers := 0, 0, 0
	have := map[string]int64{}
	if mine, err := s.store.ListDocs(ctx, uid, ""); err == nil {
		for _, d := range mine {
			have[docKey(d)] = d.ID
		}
	}
	for _, p := range doc.Docs {
		key := firstNonEmpty(p.Key, docKey(&p.Doc))
		if _, ok := have[key]; ok {
			continue
		}
		d := p.Doc
		d.ID, d.UserID, d.FileName, d.FileType, d.FileSize = 0, uid, "", "", 0
		if !oneOf(d.Kind, store.DocKinds) {
			d.Kind = "other"
		}
		if err := s.store.CreateDoc(ctx, &d, nil); err == nil {
			have[key] = d.ID
			papers++
		}
	}
	for _, ej := range doc.Jobs {
		key := firstNonEmpty(ej.ClientKey, ej.Key)
		if key == "" {
			key = fmt.Sprintf("jobs:%d", ej.ID)
		}
		if _, err := s.store.JobByClientKey(ctx, uid, key); err == nil {
			skipped++
			continue
		}
		j := ej.Job
		j.ID, j.UserID, j.ClientKey, j.SourceRef, j.NextTaskRef = 0, uid, key, "", ""
		j.CVID = have[ej.CVKey]
		j.StageID = s.stageFor(ctx, uid, ej.StageLabel, ej.StageKind)
		if err := s.store.CreateJob(ctx, &j, nil); err != nil {
			skipped++
			continue
		}
		// CreateJob takes the stage's status; keep the days the export had
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE jobs SET closed_at = ?, applied_on = ?, responded_at = ?, reached = MAX(reached, ?) WHERE user_id = ? AND id = ?`,
			ej.ClosedAt, ej.AppliedOn, ej.RespondedAt, ej.Reached, uid, j.ID)
		for i := len(ej.Timeline) - 1; i >= 0; i-- {
			a := *ej.Timeline[i]
			a.ID, a.UserID, a.JobID = 0, uid, j.ID
			_ = s.store.AddActivity(ctx, &a)
		}
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE jobs SET last_activity_at = ? WHERE user_id = ? AND id = ?`, firstNonEmpty(ej.LastActivityAt, j.CreatedAt), uid, j.ID)
		for _, iv := range ej.Interviews {
			x := *iv
			x.ID, x.UserID, x.JobID, x.EventRef, x.EventURL = 0, uid, j.ID, "", ""
			_ = s.store.CreateInterview(ctx, &x)
		}
		if ej.Offer != nil {
			o := *ej.Offer
			o.ID, o.UserID, o.JobID = 0, uid, j.ID
			_ = s.store.SaveOffer(ctx, &o)
		}
		for _, m := range ej.Elsewhere {
			if m.Error != "" {
				continue
			}
			mm := *m
			mm.ID, mm.JobID = 0, j.ID
			_ = s.store.AddMade(ctx, uid, &mm)
		}
		added++
	}
	return map[string]any{"jobs": added, "skipped": skipped, "docs": papers}
}

// csvColumns are the names other tools give the fields (lower case):
// Huntr, Teal, Simplify, a spreadsheet, in English and in Bosnian/Croatian/Serbian.
var csvColumns = map[string][]string{
	"title":    {"job title", "title", "job position", "position", "role", "job", "job name", "pozicija", "radno mjesto", "naziv"},
	"company":  {"company", "company name", "employer", "organization", "organisation", "firma", "kompanija", "poslodavac"},
	"stage":    {"stage", "list", "status", "job status", "application status", "faza"},
	"location": {"location", "city", "job location", "lokacija", "grad", "mjesto"},
	"mode":     {"work mode", "remote", "workplace type", "work type", "remote/onsite"},
	"type":     {"employment", "employment type", "job type", "type"},
	"url":      {"url", "job url", "link", "posting url", "job link", "job posting url"},
	"min":      {"min salary", "salary min", "minimum salary", "salary from"},
	"max":      {"max salary", "salary max", "maximum salary", "salary to", "salary", "plata", "plaća"},
	"currency": {"currency", "salary currency", "valuta"},
	"period":   {"salary period", "pay period", "salary per"},
	"source":   {"source", "found on", "job board", "board", "izvor"},
	"excite":   {"excitement", "interest", "rating", "priority", "stars"},
	"applyby":  {"deadline", "apply by", "application deadline", "closing date", "rok"},
	"applied":  {"date applied", "applied", "applied on", "applied date", "application date", "datum prijave"},
	"contact":  {"contact", "recruiter", "contact name", "hiring manager", "kontakt"},
	"email":    {"email", "e-mail", "contact email", "recruiter email"},
	"phone":    {"phone", "contact phone", "telefon"},
	"step":     {"next step", "next action", "follow up", "sljedeći korak"},
	"due":      {"next step due", "follow up date", "follow-up date"},
	"tags":     {"tags", "labels", "label"},
	"notes":    {"notes", "note", "comments", "description", "bilješke", "opis"},
	"added":    {"date saved", "created", "created at", "date added", "added", "saved"},
	"id":       {"id", "job id", "record id"},
}

// csvDate reads the days other tools write (2026-03-04, 04.03.2026, 3/4/2026, with a time or not).
func csvDate(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) >= 10 {
		if _, err := time.Parse("2006-01-02", v[:10]); err == nil {
			return v[:10]
		}
	}
	for _, f := range []string{"02.01.2006", "2.1.2006", "02.01.2006.", "02/01/2006", "1/2/2006", "01/02/2006", "Jan 2, 2006", "2 Jan 2006", "January 2, 2006"} {
		part := v
		if i := strings.IndexAny(v, " T"); i > 0 && !strings.Contains(f, " ") {
			part = v[:i]
		}
		if t, err := time.Parse(f, part); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func (s *Server) importCSV(ctx context.Context, uid int64, data []byte) (map[string]any, error) {
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	head := data[:min(len(data), 4096)]
	if bytes.Count(head, []byte(";")) > bytes.Count(head, []byte(",")) {
		rd.Comma = ';'
	} else if bytes.Count(head, []byte("\t")) > bytes.Count(head, []byte(",")) {
		rd.Comma = '\t'
	}
	recs, err := rd.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil, errors.New("this does not look like jobs in a CSV file (a header row and at least one job)")
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		lh := strings.ToLower(strings.TrimSpace(strings.Trim(h, "\ufeff\"")))
		for k, names := range csvColumns {
			if _, ok := idx[k]; !ok && oneOf(lh, names) {
				idx[k] = i
			}
		}
	}
	_, hasTitle := idx["title"]
	_, hasCompany := idx["company"]
	if !hasTitle && !hasCompany {
		return nil, errors.New("the first row needs a column for the job (Job title, Position) or the company")
	}
	st := s.settingsOf(ctx, uid)
	added, skipped := 0, 0
	for _, rec := range recs[1:] {
		get := func(k string) string {
			if i, ok := idx[k]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if get("title") == "" && get("company") == "" {
			continue
		}
		sum := sha1.Sum([]byte(strings.Join(rec, "\x1f")))
		key := "csv:" + hex.EncodeToString(sum[:])[:16]
		if id := get("id"); id != "" {
			key = "csv:id:" + id
		}
		if _, err := s.store.JobByClientKey(ctx, uid, key); err == nil {
			skipped++
			continue
		}
		j := &store.Job{UserID: uid, Title: clip(get("title"), 200), Company: clip(get("company"), 200), Location: clip(get("location"), 200),
			Currency: st.Currency, SalaryPeriod: st.Period, Tags: []string{}, ClientKey: key, FitScore: -1,
			ContactName: clip(get("contact"), 200), ContactEmail: strings.ToLower(clip(get("email"), 200)), ContactPhone: clip(get("phone"), 60),
			NextAction: clip(get("step"), 300), NextDue: csvDate(get("due")), ApplyBy: csvDate(get("applyby")), AppliedOn: csvDate(get("applied")),
			Notes: clip(get("notes"), 30000)}
		if u := get("url"); strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			j.URL = clip(u, 2000)
		}
		if j.ContactEmail != "" && !reEmail.MatchString(j.ContactEmail) {
			j.ContactEmail = ""
		}
		minV, maxV := parseAmount(get("min")), parseAmount(get("max"))
		if raw := get("max"); minV == 0 && strings.ContainsAny(raw, "-–") {
			// "50,000 - 60,000"
			parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '-' || r == '–' })
			if len(parts) == 2 {
				minV, maxV = parseAmount(parts[0]), parseAmount(parts[1])
			}
		}
		j.SalaryMin, j.SalaryMax = int64(minV*100+0.5), int64(maxV*100+0.5)
		if c, err := normCurrency(get("currency")); err == nil {
			j.Currency = c
		}
		if p := strings.ToLower(get("period")); p != "" {
			for _, k := range store.Periods {
				if strings.Contains(p, k) {
					j.SalaryPeriod = k
				}
			}
		}
		low := strings.ToLower(get("mode") + " " + get("location"))
		switch {
		case strings.Contains(low, "hybrid"):
			j.WorkMode = "hybrid"
		case strings.Contains(low, "remote"):
			j.WorkMode = "remote"
		case strings.Contains(low, "on-site") || strings.Contains(low, "onsite") || strings.Contains(low, "office"):
			j.WorkMode = "onsite"
		}
		if t := strings.ToLower(get("type")); t != "" {
			for _, k := range store.Employments {
				if strings.Contains(strings.ReplaceAll(t, "-", "_"), strings.ReplaceAll(k, "_", "")) || strings.Contains(strings.ReplaceAll(t, " ", "_"), k) {
					j.Employment = k
				}
			}
		}
		if src := strings.ToLower(get("source")); src != "" {
			j.Source = "other"
			switch {
			case strings.Contains(src, "linkedin"):
				j.Source = "linkedin"
			case strings.Contains(src, "refer"):
				j.Source = "referral"
			case strings.Contains(src, "recruit"):
				j.Source = "recruiter"
			case strings.Contains(src, "company") || strings.Contains(src, "career"):
				j.Source = "company_site"
			case strings.Contains(src, "indeed") || strings.Contains(src, "glassdoor") || strings.Contains(src, "board"):
				j.Source = "job_board"
			}
		}
		if e := parseAmount(get("excite")); e > 0 {
			j.Excitement = min(int(e), 5)
		}
		if t := get("tags"); t != "" {
			j.Tags = strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ';' || r == '|' })
		}
		if a := csvDate(get("added")); a != "" {
			j.CreatedAt = a + "T09:00:00Z"
		}
		j.StageID = s.stageFor(ctx, uid, get("stage"), "")
		if j.Title == "" {
			j.Title = "A job"
		}
		if err := s.store.CreateJob(ctx, j, &store.Activity{Kind: "created", Title: "Brought in from a CSV file"}); err != nil {
			skipped++
			continue
		}
		added++
	}
	return map[string]any{"jobs": added, "skipped": skipped}, nil
}
