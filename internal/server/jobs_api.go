package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- jobs, the stages of a search and the timeline ----

var (
	reHex   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reCur   = regexp.MustCompile(`^[A-Z]{3}$`)
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reTime  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// userErr is a reason the person can act on (400, not 502).
type userErr struct{ msg string }

func (e *userErr) Error() string { return e.msg }

// today is the local day for the person (the box's zone).
func today() string { return time.Now().Format("2006-01-02") }

// decorate fills what lists and pages show besides the stored fields.
func decorate(st store.Settings, js ...*store.Job) {
	t := today()
	quietBefore := store.AddDays(t, -max(st.StaleDays, 1))
	for _, j := range js {
		j.ClientKey = ""
		if j.Status != store.StatusOpen {
			continue
		}
		j.Overdue = j.NextDue != "" && j.NextDue < t
		// applied, nothing heard, nothing planned: time to follow up
		j.Stale = st.StaleDays > 0 && j.StageKind == store.KindApplied && j.AppliedOn != "" && j.AppliedOn <= quietBefore &&
			day10(j.LastActivityAt) <= quietBefore && j.NextAction == ""
		if at, err := time.Parse(time.RFC3339Nano, j.StageChangedAt); err == nil {
			j.DaysInStage = int(time.Since(at).Hours() / 24)
		}
	}
}

func (s *Server) settingsOf(ctx context.Context, uid int64) store.Settings {
	st, _ := s.store.GetSettings(ctx, uid)
	return st
}

// ---- stages ----

func (s *Server) handleListStages(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListStages(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "stages")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type stageIn struct {
	Name     *string `json:"name"`
	Kind     *string `json:"kind"`
	Color    *string `json:"color"`
	Position int     `json:"position"`
}

func (in stageIn) apply(st *store.Stage) error {
	if in.Name != nil {
		st.Name = clip(strings.TrimSpace(*in.Name), 60)
	}
	if st.Name == "" {
		return &userErr{"give the stage a name"}
	}
	if in.Kind != nil {
		if !oneOf(*in.Kind, store.StageKinds) {
			return &userErr{"a stage is saved, applied, active, offer, hired or closed"}
		}
		st.Kind = *in.Kind
	}
	if in.Color != nil {
		if *in.Color != "" && !reHex.MatchString(*in.Color) {
			return &userErr{"the colour is like #3d5a98"}
		}
		st.Color = *in.Color
	}
	return nil
}

func (s *Server) handleCreateStage(w http.ResponseWriter, r *http.Request) {
	var in stageIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := &store.Stage{UserID: actorID(r.Context()), Kind: store.KindActive, Position: in.Position}
	if err := in.apply(st); err != nil {
		s.userFail(w, err, "stage")
		return
	}
	if err := s.store.CreateStage(r.Context(), st); err != nil {
		s.fail(w, err, "create stage")
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.store.GetStage(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "stage")
		return
	}
	var in stageIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(st); err != nil {
		s.userFail(w, err, "stage")
		return
	}
	if err := s.store.UpdateStage(ctx, st); err != nil {
		s.fail(w, err, "save stage")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDeleteStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := s.store.DeleteStage(ctx, actorID(ctx), idParam(r, "id"), int64(queryInt(r, "move_to")))
	if errors.Is(err, store.ErrStageInUse) {
		writeErr(w, http.StatusConflict, "jobs are still in this stage: pick where they go")
		return
	}
	if err != nil {
		s.fail(w, err, "delete stage")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOrderStages(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := r.Context()
	if err := s.store.ReorderStages(ctx, actorID(ctx), in.IDs); err != nil {
		s.fail(w, err, "order stages")
		return
	}
	s.handleListStages(w, r)
}

// ---- jobs ----

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	f := store.JobFilter{Status: q.Get("status"), StageID: int64(queryInt(r, "stage")), Kind: q.Get("kind"), Search: q.Get("q"), Company: q.Get("company"),
		Email: q.Get("email"), Due: q.Get("due"), Starred: queryBool(r, "starred"), Tag: q.Get("tag"), Source: q.Get("source"), WorkMode: q.Get("work_mode"),
		Sort: q.Get("sort"), Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}
	list, total, err := s.store.ListJobs(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "jobs")
		return
	}
	decorate(s.settingsOf(ctx, uid), list...)
	if !queryBool(r, "full") {
		for _, j := range list {
			j.Posting, j.CoverLetter = clip(j.Posting, 400), ""
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": total})
}

// jobIn is a job as sent: every field optional (PUT changes what is sent).
type jobIn struct {
	Title        *string   `json:"title"`
	Company      *string   `json:"company"`
	CompanyRef   *string   `json:"company_ref"`
	StageID      *int64    `json:"stage_id"`
	Stage        *string   `json:"stage"` // by name or kind (links, imports)
	Location     *string   `json:"location"`
	WorkMode     *string   `json:"work_mode"`
	Employment   *string   `json:"employment"`
	URL          *string   `json:"url"`
	Posting      *string   `json:"posting"`
	SalaryMin    *float64  `json:"min_amount"`
	SalaryMax    *float64  `json:"max_amount"`
	Currency     *string   `json:"currency"`
	SalaryPeriod *string   `json:"salary_period"`
	Source       *string   `json:"source"`
	Excitement   *int      `json:"excitement"`
	ApplyBy      *string   `json:"apply_by"`
	AppliedOn    *string   `json:"applied_on"`
	ContactName  *string   `json:"contact_name"`
	ContactEmail *string   `json:"contact_email"`
	ContactPhone *string   `json:"contact_phone"`
	ContactRef   *string   `json:"contact_ref"`
	Notes        *string   `json:"notes"`
	CoverLetter  *string   `json:"cover_letter"`
	CVID         *int64    `json:"cv_id"`
	NextAction   *string   `json:"next_action"`
	NextDue      *string   `json:"next_due"`
	Tags         *[]string `json:"tags"`
	Starred      *bool     `json:"starred"`
	EndReason    *string   `json:"end_reason"`
	OutcomeNote  *string   `json:"outcome_note"`
	SourceRef    string    `json:"source_ref"` // links: what it came from, so it is never added twice
	Note         string    `json:"note"`       // a first line for the timeline
}

// cents reads an amount in a currency's units as cents.
func cents(v float64) (int64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e13 {
		return 0, &userErr{"an amount is a number, 0 or more"}
	}
	return int64(math.Round(v * 100)), nil
}

// normCurrency reads "km" as BAM and checks the code.
func normCurrency(v string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(v))
	if c == "KM" {
		c = "BAM"
	}
	if c == "€" {
		c = "EUR"
	}
	if c == "$" {
		c = "USD"
	}
	if !reCur.MatchString(c) {
		return "", &userErr{"the currency is a three-letter code like EUR"}
	}
	return c, nil
}

func (in jobIn) apply(j *store.Job) error {
	txt := func(p *string, dst *string, n int) {
		if p != nil {
			*dst = clip(strings.TrimSpace(*p), n)
		}
	}
	txt(in.Title, &j.Title, 200)
	txt(in.Company, &j.Company, 200)
	txt(in.CompanyRef, &j.CompanyRef, 60)
	txt(in.Location, &j.Location, 200)
	txt(in.URL, &j.URL, 2000)
	txt(in.Posting, &j.Posting, 60000)
	txt(in.ContactName, &j.ContactName, 200)
	txt(in.ContactEmail, &j.ContactEmail, 200)
	txt(in.ContactPhone, &j.ContactPhone, 60)
	txt(in.ContactRef, &j.ContactRef, 60)
	txt(in.Notes, &j.Notes, 30000)
	txt(in.CoverLetter, &j.CoverLetter, 20000)
	txt(in.NextAction, &j.NextAction, 300)
	txt(in.NextDue, &j.NextDue, 10)
	txt(in.ApplyBy, &j.ApplyBy, 10)
	txt(in.AppliedOn, &j.AppliedOn, 10)
	txt(in.OutcomeNote, &j.OutcomeNote, 5000)
	j.ContactEmail = strings.ToLower(j.ContactEmail)
	if j.ContactEmail != "" && !reEmail.MatchString(j.ContactEmail) {
		return &userErr{"that email address does not look right"}
	}
	if j.URL != "" {
		if u, err := url.Parse(j.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			if !strings.Contains(j.URL, "://") && strings.Contains(j.URL, ".") {
				j.URL = "https://" + j.URL
			} else {
				return &userErr{"the posting's link starts with https://"}
			}
		}
	}
	if !validDate(j.NextDue) || !validDate(j.ApplyBy) || !validDate(j.AppliedOn) {
		return &userErr{"days are YYYY-MM-DD"}
	}
	var err error
	if in.SalaryMin != nil {
		if j.SalaryMin, err = cents(*in.SalaryMin); err != nil {
			return err
		}
	}
	if in.SalaryMax != nil {
		if j.SalaryMax, err = cents(*in.SalaryMax); err != nil {
			return err
		}
	}
	if j.SalaryMin > 0 && j.SalaryMax > 0 && j.SalaryMax < j.SalaryMin {
		j.SalaryMin, j.SalaryMax = j.SalaryMax, j.SalaryMin
	}
	if in.Currency != nil {
		if j.Currency, err = normCurrency(*in.Currency); err != nil {
			return err
		}
	}
	pick := func(p *string, dst *string, list []string, what string) error {
		if p == nil {
			return nil
		}
		v := strings.ToLower(strings.TrimSpace(*p))
		v = strings.NewReplacer("-", "_", " ", "_").Replace(v)
		if v != "" && !oneOf(v, list) {
			return &userErr{what + " is one of " + strings.Join(list, ", ")}
		}
		*dst = v
		return nil
	}
	if err := pick(in.WorkMode, &j.WorkMode, store.WorkModes, "where you work"); err != nil {
		return err
	}
	if err := pick(in.Employment, &j.Employment, store.Employments, "the kind of job"); err != nil {
		return err
	}
	if err := pick(in.SalaryPeriod, &j.SalaryPeriod, store.Periods, "the salary is per"); err != nil {
		return err
	}
	if j.SalaryPeriod == "" {
		j.SalaryPeriod = "year"
	}
	if err := pick(in.EndReason, &j.EndReason, store.EndReasons, "why it ended"); err != nil {
		return err
	}
	if in.Source != nil {
		src := strings.NewReplacer("-", "_", " ", "_").Replace(strings.TrimSpace(strings.ToLower(*in.Source)))
		if src != "" && !oneOf(src, store.Sources) {
			src = "other"
		}
		j.Source = src
	}
	if in.Excitement != nil {
		if *in.Excitement < 0 || *in.Excitement > 5 {
			return &userErr{"how much you want it is 0 to 5"}
		}
		j.Excitement = *in.Excitement
	}
	if in.CVID != nil {
		j.CVID = *in.CVID
	}
	if in.Tags != nil {
		j.Tags = *in.Tags
	}
	if in.Starred != nil {
		j.Starred = *in.Starred
	}
	if j.Title == "" && j.Company == "" {
		return &userErr{"say what the job is (the role or the company)"}
	}
	return nil
}

func (s *Server) userFail(w http.ResponseWriter, err error, what string) {
	var ue *userErr
	if errors.As(err, &ue) {
		writeErr(w, http.StatusBadRequest, ue.msg)
		return
	}
	s.fail(w, err, what)
}

// resolveStage turns stage_id / stage (a name or a kind) into one of the person's stages.
func (s *Server) resolveStage(ctx context.Context, uid int64, in jobIn) (*store.Stage, error) {
	switch {
	case in.StageID != nil && *in.StageID > 0:
		st, err := s.store.GetStage(ctx, uid, *in.StageID)
		if err != nil {
			return nil, &userErr{"there is no such stage"}
		}
		return st, nil
	case in.Stage != nil && strings.TrimSpace(*in.Stage) != "":
		name := strings.TrimSpace(*in.Stage)
		if st, err := s.store.StageByName(ctx, uid, name); err == nil {
			return st, nil
		}
		kind := kindFromWord(name)
		if kind != "" {
			if st, err := s.store.FirstStage(ctx, uid, kind); err == nil {
				return st, nil
			}
		}
		return nil, &userErr{"there is no stage called " + name}
	}
	return nil, nil
}

// kindFromWord reads the stage words other trackers use (Huntr, Teal, a sheet).
func kindFromWord(v string) string {
	w := strings.ToLower(strings.TrimSpace(v))
	switch {
	case oneOf(w, store.StageKinds):
		return w
	case strings.Contains(w, "wish") || strings.Contains(w, "bookmark") || strings.Contains(w, "save") || strings.Contains(w, "interest") || w == "to apply":
		return store.KindSaved
	case strings.Contains(w, "appl"):
		return store.KindApplied
	case strings.Contains(w, "interview") || strings.Contains(w, "screen") || strings.Contains(w, "phone") || strings.Contains(w, "talk"):
		return store.KindActive
	case strings.Contains(w, "offer") || strings.Contains(w, "negotiat"):
		return store.KindOffer
	case strings.Contains(w, "accept") || strings.Contains(w, "hired") || strings.Contains(w, "signed"):
		return store.KindHired
	case strings.Contains(w, "reject") || strings.Contains(w, "declin") || strings.Contains(w, "withdr") || strings.Contains(w, "closed") || strings.Contains(w, "archiv") || strings.Contains(w, "ghost"):
		return store.KindClosed
	}
	return ""
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in jobIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	app := linkApp(ctx)
	ref := clip(strings.TrimSpace(in.SourceRef), 200)
	if ref != "" {
		if app != "" && !strings.HasPrefix(ref, app+":") {
			ref = app + ":" + ref
		}
		if j, err := s.store.JobBySource(ctx, uid, ref); err == nil {
			decorate(s.settingsOf(ctx, uid), j)
			writeJSON(w, http.StatusOK, j)
			return
		}
	}
	if in.URL != nil && strings.TrimSpace(*in.URL) != "" {
		if j, err := s.store.JobByURL(ctx, uid, strings.TrimSpace(*in.URL)); err == nil && (app != "" || queryBool(r, "once")) {
			decorate(s.settingsOf(ctx, uid), j)
			writeJSON(w, http.StatusOK, j)
			return
		}
	}
	j, err := s.createJob(ctx, uid, in, app, ref)
	if err != nil {
		s.userFail(w, err, "create job")
		return
	}
	writeJSON(w, http.StatusCreated, j)
}

// createJob stores a new job with its first timeline line and tells the box.
func (s *Server) createJob(ctx context.Context, uid int64, in jobIn, app, ref string) (*store.Job, error) {
	st := s.settingsOf(ctx, uid)
	j := &store.Job{UserID: uid, Currency: st.Currency, SalaryPeriod: st.Period, SourceApp: app, SourceRef: ref, Tags: []string{}, FitScore: -1}
	if err := in.apply(j); err != nil {
		return nil, err
	}
	if j.CVID > 0 {
		if _, err := s.store.GetDoc(ctx, uid, j.CVID); err != nil {
			j.CVID = 0
		}
	}
	if stage, err := s.resolveStage(ctx, uid, in); err != nil {
		return nil, err
	} else if stage != nil {
		j.StageID = stage.ID
	}
	first := &store.Activity{Kind: "created", Title: "Saved the job", Body: strings.TrimSpace(in.Note), SourceApp: app}
	if a, ok := appBySlug(app); ok {
		first.Title, first.App = "Added from "+a.name, app
	} else if app != "" {
		first.Title = "Added from Pocket " + strings.ToUpper(app[:1]) + app[1:]
	}
	if err := s.store.CreateJob(ctx, j, first); err != nil {
		return nil, err
	}
	j, err := s.store.GetJob(ctx, uid, j.ID)
	if err != nil {
		return nil, err
	}
	decorate(st, j)
	s.publish(ctx, uid, "jobs.job_added", fmt.Sprintf("job:%d", j.ID), s.jobEvent(j))
	if j.AppliedOn != "" {
		s.publish(ctx, uid, "jobs.applied", fmt.Sprintf("job:%d", j.ID), s.jobEvent(j))
	}
	if j.NextAction != "" && st.TaskMirror {
		go s.mirrorStep(context.WithoutCancel(ctx), uid, j.ID)
	}
	return j, nil
}

// jobEvent is a job as events carry it.
func (s *Server) jobEvent(j *store.Job) map[string]any {
	return map[string]any{"job_id": j.ID, "title": j.Title, "company": j.Company, "label": j.Label(), "stage": j.StageName, "stage_kind": j.StageKind,
		"status": j.Status, "location": j.Location, "work_mode": j.WorkMode, "applied_on": j.AppliedOn, "contact_name": j.ContactName,
		"contact_email": j.ContactEmail, "next_action": j.NextAction, "next_due": j.NextDue, "url": s.jobLink(j.ID), "posting_url": j.URL}
}

// jobLink is the owner's page of a job.
func (s *Server) jobLink(id int64) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + fmt.Sprintf("/jobs/%d", id)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	j, err := s.store.GetJob(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	decorate(s.settingsOf(ctx, uid), j)
	acts, _ := s.store.ListActivities(ctx, uid, j.ID, 300)
	made, _ := s.store.ListMade(ctx, uid, j.ID)
	ivs, _ := s.store.ListInterviews(ctx, uid, store.InterviewFilter{JobID: j.ID})
	out := map[string]any{"job": j, "activities": acts, "made": made, "interviews": ivs, "url": s.jobLink(j.ID), "offer": nil}
	if o, err := s.store.OfferFor(ctx, uid, j.ID); err == nil {
		out["offer"] = o
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	j, err := s.store.GetJob(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	if app := linkApp(ctx); app != "" && j.SourceApp != app {
		writeErr(w, http.StatusForbidden, "this job was not added by "+app)
		return
	}
	var in jobIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	oldStep, oldDue, oldApplied := j.NextAction, j.NextDue, j.AppliedOn
	if err := in.apply(j); err != nil {
		s.userFail(w, err, "job")
		return
	}
	if j.NextAction == "" {
		j.NextDue = ""
	}
	if j.CVID > 0 {
		if _, err := s.store.GetDoc(ctx, uid, j.CVID); err != nil {
			s.userFail(w, &userErr{"there is no such CV"}, "job")
			return
		}
	}
	if j.AppliedOn != "" && j.Reached < 1 {
		j.Reached = 1
	}
	if err := s.store.UpdateJob(ctx, j); err != nil {
		s.fail(w, err, "save job")
		return
	}
	if stage, err := s.resolveStage(ctx, uid, in); err != nil {
		s.userFail(w, err, "job")
		return
	} else if stage != nil && stage.ID != j.StageID {
		if _, err := s.moveJob(ctx, uid, j.ID, stage.ID, nil, nil, "", ""); err != nil {
			s.userFail(w, err, "move job")
			return
		}
	}
	if j.NextAction != oldStep || j.NextDue != oldDue {
		go s.mirrorStep(context.WithoutCancel(ctx), uid, j.ID)
	}
	j, _ = s.store.GetJob(ctx, uid, j.ID)
	if oldApplied == "" && j.AppliedOn != "" {
		s.publish(ctx, uid, "jobs.applied", fmt.Sprintf("job:%d", j.ID), s.jobEvent(j))
	}
	decorate(s.settingsOf(ctx, uid), j)
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteJob(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete job")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// endWords is how the timeline says why a job ended.
var endWords = map[string]string{"rejected": "They said no", "withdrew": "I withdrew", "no_answer": "No answer",
	"filled": "The job was filled or closed", "other_job": "I took another job", "other": "Ended"}

// moveJob puts a job in a stage and writes it on the timeline; applying,
// an offer, being hired and an end are told to the box.
func (s *Server) moveJob(ctx context.Context, uid, id, stageID int64, before, after *float64, reason, note string) (*store.Job, error) {
	prev, err := s.store.GetJob(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	st, err := s.store.GetStage(ctx, uid, stageID)
	if err != nil {
		return nil, &userErr{"there is no such stage"}
	}
	if st.Kind == store.KindClosed && reason != "" {
		if !oneOf(reason, store.EndReasons) {
			return nil, &userErr{"why it ended is one of " + strings.Join(store.EndReasons, ", ")}
		}
		prev.EndReason = reason
		_ = s.store.UpdateJob(ctx, prev)
	}
	old, err := s.store.MoveJob(ctx, uid, id, stageID, before, after)
	if err != nil {
		return nil, err
	}
	j, err := s.store.GetJob(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if old == nil || old.ID == st.ID {
		return j, nil
	}
	if st.Kind == store.KindClosed && (reason != "" || note != "") {
		j.EndReason, j.OutcomeNote = reason, clip(note, 5000)
		_ = s.store.UpdateJob(ctx, j)
	} else if st.Kind == store.KindHired && note != "" {
		j.OutcomeNote = clip(note, 5000)
		_ = s.store.UpdateJob(ctx, j)
	}
	a := &store.Activity{UserID: uid, JobID: id, Kind: "stage", Title: fmt.Sprintf("Moved from %s to %s", old.Name, st.Name), SourceApp: linkApp(ctx)}
	topic := "jobs.stage_changed"
	switch {
	case st.Kind == store.KindHired && prev.Status != store.StatusHired:
		a.Kind, a.Title, a.Body, topic = "hired", "Hired 🎉", note, "jobs.hired"
	case st.Kind == store.KindClosed && prev.Status != store.StatusClosed:
		a.Kind, a.Title, a.Body, topic = "closed", firstNonEmpty(endWords[reason], "Closed"), note, "jobs.closed"
	case st.Kind == store.KindOffer && prev.Reached < 3:
		a.Kind, a.Title, topic = "offer", "Got an offer", "jobs.offer_received"
	case st.Kind == store.KindApplied && prev.AppliedOn == "":
		a.Kind, a.Title, topic = "applied", "Applied", "jobs.applied"
	case store.StatusOf(st.Kind) == store.StatusOpen && prev.Status != store.StatusOpen:
		a.Title = "Opened again, in " + st.Name
	}
	_ = s.store.AddActivity(ctx, a)
	j, _ = s.store.GetJob(ctx, uid, id)
	payload := s.jobEvent(j)
	payload["from_stage"] = old.Name
	s.publish(ctx, uid, topic, fmt.Sprintf("job:%d", id), payload)
	if topic != "jobs.stage_changed" && topic != "jobs.applied" && j.NextTaskRef != "" && j.Status != store.StatusOpen {
		ref := j.NextTaskRef
		_ = s.store.SetNextStep(ctx, uid, id, "", "", "")
		go s.completeTask(context.WithoutCancel(ctx), uid, ref)
	}
	return j, nil
}

func (s *Server) handleMoveJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		StageID int64    `json:"stage_id"`
		Before  *float64 `json:"before"` // the position of the job above
		After   *float64 `json:"after"`  // and below
		Reason  string   `json:"end_reason"`
		Note    string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	j, err := s.moveJob(ctx, uid, idParam(r, "id"), in.StageID, in.Before, in.After, strings.TrimSpace(in.Reason), strings.TrimSpace(in.Note))
	if err != nil {
		s.userFail(w, err, "move job")
		return
	}
	decorate(s.settingsOf(ctx, uid), j)
	writeJSON(w, http.StatusOK, j)
}

// handleStepDone ticks off the next step (it goes on the timeline, and is
// ticked in Pocket Tasks too) and sets the one after it.
func (s *Server) handleStepDone(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	j, err := s.store.GetJob(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	var in struct {
		Kind       string `json:"kind"` // what it was: step | call | email
		Note       string `json:"note"`
		NextAction string `json:"next_action"`
		NextDue    string `json:"next_due"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !validDate(in.NextDue) {
		writeErr(w, http.StatusBadRequest, "days are YYYY-MM-DD")
		return
	}
	if !oneOf(in.Kind, store.ActivityKinds) {
		in.Kind = "step"
	}
	if j.NextAction != "" || strings.TrimSpace(in.Note) != "" {
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: in.Kind, Title: firstNonEmpty(j.NextAction, "Done"), Body: clip(strings.TrimSpace(in.Note), 20000)})
	}
	oldTask := j.NextTaskRef
	next := clip(strings.TrimSpace(in.NextAction), 300)
	due := in.NextDue
	if next == "" {
		due = ""
	}
	if err := s.store.SetNextStep(ctx, uid, j.ID, next, due, ""); err != nil {
		s.fail(w, err, "next step")
		return
	}
	go func(ctx context.Context) {
		if oldTask != "" {
			s.completeTask(ctx, uid, oldTask)
		}
		if next != "" {
			s.mirrorStep(ctx, uid, j.ID)
		}
	}(context.WithoutCancel(ctx))
	j, _ = s.store.GetJob(ctx, uid, j.ID)
	decorate(s.settingsOf(ctx, uid), j)
	writeJSON(w, http.StatusOK, j)
}

// ---- the timeline ----

func (s *Server) handleListActivities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if _, err := s.store.GetJob(ctx, uid, idParam(r, "id")); err != nil {
		s.fail(w, err, "job")
		return
	}
	list, err := s.store.ListActivities(ctx, uid, idParam(r, "id"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type activityIn struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
	At    string `json:"at"`
}

func (in *activityIn) clean() error {
	in.Kind = strings.TrimSpace(in.Kind)
	if in.Kind == "" {
		in.Kind = "note"
	}
	if !oneOf(in.Kind, store.ActivityKinds) {
		return &userErr{"a line is a note, call, email or step"}
	}
	in.Title, in.Body = clip(strings.TrimSpace(in.Title), 300), clip(strings.TrimSpace(in.Body), 20000)
	if in.Title == "" && in.Body == "" {
		return &userErr{"write something first"}
	}
	if in.At != "" {
		t, err := parseWhen(in.At)
		if err != nil {
			return &userErr{"the time is like 2026-11-07T14:30"}
		}
		in.At = t.UTC().Format(time.RFC3339Nano)
	}
	return nil
}

// parseWhen reads a time as people and apps send it.
func parseWhen(v string) (time.Time, error) {
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("bad time")
}

func (s *Server) handleAddActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in activityIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.clean(); err != nil {
		s.userFail(w, err, "timeline")
		return
	}
	a := &store.Activity{UserID: uid, JobID: idParam(r, "id"), Kind: in.Kind, Title: in.Title, Body: in.Body, At: in.At, SourceApp: linkApp(ctx)}
	if sa, ok := appBySlug(a.SourceApp); ok {
		a.App = sa.slug
	}
	if err := s.store.AddActivity(ctx, a); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	a, err := s.store.GetActivity(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "timeline")
		return
	}
	if !oneOf(a.Kind, store.ActivityKinds) {
		writeErr(w, http.StatusBadRequest, "this line was written by the app")
		return
	}
	in := activityIn{Kind: a.Kind, Title: a.Title, Body: a.Body}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.clean(); err != nil {
		s.userFail(w, err, "timeline")
		return
	}
	a.Kind, a.Title, a.Body = in.Kind, in.Title, in.Body
	if in.At != "" {
		a.At = in.At
	}
	if err := s.store.UpdateActivity(ctx, a); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteActivity(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteMade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteMade(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "linked item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.Tags(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}
