package server

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- totals, today, companies, reports, search, a day ----

type summary struct {
	Open          int              `json:"open"`
	Saved         int              `json:"saved"`
	Waiting       int              `json:"waiting"` // applied, not heard yet
	Talking       int              `json:"talking"`
	Offers        int              `json:"offers"` // offers still to decide
	Hired         int              `json:"hired"`
	Closed        int              `json:"closed"`
	WeeklyGoal    int              `json:"weekly_goal"`
	AppliedWeek   int              `json:"applied_week"`
	AppliedMonth  int              `json:"applied_month"`
	AppliedTotal  int              `json:"applied_total"`
	ResponseRate  int              `json:"response_rate"`  // % of applications (last 90 days, 7+ days old) that got an answer
	InterviewRate int              `json:"interview_rate"` // % of those that led to an interview
	Upcoming      int              `json:"interviews_upcoming"`
	NextInterview *store.Interview `json:"next_interview"`
	DueToday      int              `json:"due_today"`
	Overdue       int              `json:"overdue"`
	FollowUp      int              `json:"follow_up"`
	ApplySoon     int              `json:"apply_soon"`
	WeekStart     string           `json:"week_start"`
	URL           string           `json:"url"`
}

// weekStart is the first day of the week holding t (the person's week start).
func weekStart(st store.Settings, t time.Time) time.Time {
	first := time.Weekday(st.WeekStart % 7) // 1 = Monday, 7 → 0 = Sunday, 6 = Saturday
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	for d.Weekday() != first {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func (s *Server) summarize(r *http.Request, st store.Settings, all []*store.Job) summary {
	now := time.Now()
	t := now.Format("2006-01-02")
	ws := weekStart(st, now).Format("2006-01-02")
	month := now.Format("2006-01")
	since90, before7 := store.AddDays(t, -90), store.AddDays(t, -7)
	soon := store.AddDays(t, 7)
	out := summary{WeeklyGoal: st.WeeklyGoal, WeekStart: ws, URL: strings.TrimRight(s.cfg.BaseURL, "/") + "/"}
	ctx := r.Context()
	interviewed := map[int64]bool{}
	if ivs, err := s.store.ListInterviews(ctx, actorID(ctx), store.InterviewFilter{}); err == nil {
		for _, iv := range ivs {
			interviewed[iv.JobID] = true
			if !iv.Past {
				out.Upcoming++
				if out.NextInterview == nil {
					out.NextInterview = iv
				}
			}
		}
	}
	base, answered, withInterview := 0, 0, 0
	for _, j := range all {
		if j.AppliedOn != "" {
			out.AppliedTotal++
			if j.AppliedOn >= ws {
				out.AppliedWeek++
			}
			if strings.HasPrefix(j.AppliedOn, month) {
				out.AppliedMonth++
			}
			if j.AppliedOn >= since90 && j.AppliedOn <= before7 {
				base++
				if j.RespondedAt != "" {
					answered++
				}
				if interviewed[j.ID] {
					withInterview++
				}
			}
		}
		switch j.Status {
		case store.StatusHired:
			out.Hired++
			continue
		case store.StatusClosed:
			out.Closed++
			continue
		}
		out.Open++
		switch j.StageKind {
		case store.KindSaved:
			out.Saved++
			if j.ApplyBy != "" && j.ApplyBy <= soon {
				out.ApplySoon++
			}
		case store.KindApplied:
			out.Waiting++
		case store.KindActive:
			out.Talking++
		case store.KindOffer:
			out.Offers++
		}
		switch {
		case j.NextDue != "" && j.NextDue < t:
			out.Overdue++
		case j.NextDue == t:
			out.DueToday++
		}
		if j.Stale {
			out.FollowUp++
		}
	}
	if base > 0 {
		out.ResponseRate = answered * 100 / base
		out.InterviewRate = withInterview * 100 / base
	}
	return out
}

func closedLocal(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

func (s *Server) allDecorated(r *http.Request) (store.Settings, []*store.Job, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	all, err := s.store.AllJobs(ctx, uid)
	if err != nil {
		return st, nil, err
	}
	decorate(st, all...)
	for _, j := range all {
		j.Posting, j.CoverLetter = "", ""
	}
	return st, all, nil
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	st, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	writeJSON(w, http.StatusOK, s.summarize(r, st, all))
}

// handleToday is the Today page: interviews coming up, what to do now, the
// jobs to follow up and to apply for, offers to decide, and the week's goal.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "today")
		return
	}
	t := today()
	week := store.AddDays(t, 7)
	out := map[string][]*store.Job{"overdue": {}, "today": {}, "week": {}, "follow_up": {}, "apply": {}, "no_step": {}, "recent": {}}
	monthAgo := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339Nano)
	for _, j := range all {
		if j.Status != store.StatusOpen {
			if j.Status == store.StatusHired && j.ClosedAt >= monthAgo {
				out["recent"] = append(out["recent"], j)
			}
			continue
		}
		switch {
		case j.NextDue != "" && j.NextDue < t:
			out["overdue"] = append(out["overdue"], j)
		case j.NextDue == t || (j.NextAction != "" && j.NextDue == ""):
			out["today"] = append(out["today"], j)
		case j.NextDue != "" && j.NextDue <= week:
			out["week"] = append(out["week"], j)
		}
		if j.Stale {
			out["follow_up"] = append(out["follow_up"], j)
		}
		if j.StageKind == store.KindSaved && (j.ApplyBy != "" && j.ApplyBy <= store.AddDays(t, 14) || j.Excitement >= 4) {
			out["apply"] = append(out["apply"], j)
		}
		if j.NextAction == "" && !j.Stale && j.StageKind != store.KindSaved && j.NextInterview == "" {
			out["no_step"] = append(out["no_step"], j)
		}
	}
	byDue := func(l []*store.Job) {
		sort.SliceStable(l, func(a, b int) bool {
			if l[a].NextDue != l[b].NextDue {
				return l[a].NextDue < l[b].NextDue
			}
			return l[a].Excitement > l[b].Excitement
		})
	}
	byDue(out["overdue"])
	byDue(out["today"])
	byDue(out["week"])
	sort.SliceStable(out["follow_up"], func(a, b int) bool { return out["follow_up"][a].AppliedOn < out["follow_up"][b].AppliedOn })
	sort.SliceStable(out["apply"], func(a, b int) bool {
		x, y := out["apply"][a], out["apply"][b]
		if (x.ApplyBy == "") != (y.ApplyBy == "") {
			return x.ApplyBy != ""
		}
		if x.ApplyBy != y.ApplyBy {
			return x.ApplyBy < y.ApplyBy
		}
		return x.Excitement > y.Excitement
	})
	if len(out["apply"]) > 8 {
		out["apply"] = out["apply"][:8]
	}
	ivs, _ := s.store.ListInterviews(ctx, uid, store.InterviewFilter{When: "upcoming", To: store.AddDays(t, 15), Limit: 20})
	offers, _ := s.store.ListOffers(ctx, uid)
	open := []*store.Offer{}
	for _, o := range offers {
		if o.Status == "open" || o.Status == "negotiating" {
			open = append(open, o)
		}
	}
	// the week, day by day: applications sent
	ws := weekStart(st, time.Now())
	days := make([]map[string]any, 7)
	for i := range days {
		d := ws.AddDate(0, 0, i).Format("2006-01-02")
		n := 0
		for _, j := range all {
			if j.AppliedOn == d {
				n++
			}
		}
		days[i] = map[string]any{"date": d, "applied": n}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": out, "interviews": ivs, "offers": open, "week": days,
		"summary": s.summarize(r, st, all), "date": t})
}

// company is one company across a person's jobs.
type company struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	CompanyRef string   `json:"company_ref"`
	Jobs       int      `json:"jobs"`
	Open       int      `json:"open"`
	Hired      int      `json:"hired"`
	Closed     int      `json:"closed"`
	Interviews int      `json:"interviews"`
	LastAt     string   `json:"last_at"`
	People     []string `json:"people"`
	Emails     []string `json:"emails"`
	Roles      []string `json:"roles"`
	JobIDs     []int64  `json:"job_ids"`
}

// handleCompanies groups jobs by company: where you applied, who you talked to.
func (s *Server) handleCompanies(w http.ResponseWriter, r *http.Request) {
	_, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "companies")
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	by := map[string]*company{}
	var order []string
	for _, j := range all {
		name := strings.TrimSpace(j.Company)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		c := by[key]
		if c == nil {
			c = &company{Key: key, Name: name, People: []string{}, Emails: []string{}, Roles: []string{}, JobIDs: []int64{}}
			by[key] = c
			order = append(order, key)
		}
		c.CompanyRef = firstNonEmpty(c.CompanyRef, j.CompanyRef)
		if j.ContactName != "" && !oneOf(j.ContactName, c.People) {
			c.People = append(c.People, j.ContactName)
		}
		if j.ContactEmail != "" && !oneOf(j.ContactEmail, c.Emails) {
			c.Emails = append(c.Emails, j.ContactEmail)
		}
		if j.Title != "" && !oneOf(j.Title, c.Roles) {
			c.Roles = append(c.Roles, j.Title)
		}
		c.Jobs++
		c.Interviews += j.Interviews
		c.JobIDs = append(c.JobIDs, j.ID)
		switch j.Status {
		case store.StatusOpen:
			c.Open++
		case store.StatusHired:
			c.Hired++
		case store.StatusClosed:
			c.Closed++
		}
		if j.LastActivityAt > c.LastAt {
			c.LastAt = j.LastActivityAt
		}
	}
	out := []*company{}
	for _, k := range order {
		c := by[k]
		if q != "" && !strings.Contains(strings.ToLower(c.Name+" "+strings.Join(c.People, " ")+" "+strings.Join(c.Roles, " ")), q) {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].LastAt > out[b].LastAt })
	writeJSON(w, http.StatusOK, out)
}

// handleLookup is a contact's page in another app: the jobs with this email
// (or company).
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	comp := strings.TrimSpace(r.URL.Query().Get("company"))
	if email == "" && comp == "" {
		writeErr(w, http.StatusBadRequest, "ask with ?email= or ?company=")
		return
	}
	st := s.settingsOf(ctx, uid)
	seen := map[int64]bool{}
	out := []*store.Job{}
	for _, f := range []store.JobFilter{{Email: email}, {Company: comp}} {
		if f.Email == "" && f.Company == "" {
			continue
		}
		f.Limit = 100
		list, _, err := s.store.ListJobs(ctx, uid, f)
		if err != nil {
			s.fail(w, err, "lookup")
			return
		}
		for _, j := range list {
			if !seen[j.ID] {
				seen[j.ID] = true
				out = append(out, j)
			}
		}
	}
	decorate(st, out...)
	for _, j := range out {
		j.Posting, j.CoverLetter, j.Notes = "", "", clip(j.Notes, 300)
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out, "url": strings.TrimRight(s.cfg.BaseURL, "/")})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"jobs": []any{}, "total": 0})
		return
	}
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	list, total, err := s.store.ListJobs(ctx, uid, store.JobFilter{Search: q, Limit: limit})
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	decorate(s.settingsOf(ctx, uid), list...)
	type hit struct {
		*store.Job
		Label string `json:"label"`
		Link  string `json:"link"`
	}
	out := []hit{}
	for _, j := range list {
		j.Posting, j.CoverLetter, j.Notes = "", "", clip(j.Notes, 300)
		out = append(out, hit{j, j.Label(), s.jobLink(j.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out, "total": total})
}

// handleDay is one day for a journal (Pocket Day): applications sent,
// interviews had, offers, steps done, and what was due that day.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chiParam(r, "date")
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "day is YYYY-MM-DD")
		return
	}
	from := day.UTC().Format(time.RFC3339Nano)
	to := day.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	acts, err := s.store.ActivitiesBetween(ctx, uid, from, to, 300)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	labels := map[int64]string{}
	type line struct {
		*store.Activity
		Job    string `json:"job"`
		JobURL string `json:"job_url"`
	}
	lines := []line{}
	counts := map[string]int{}
	for _, a := range acts {
		if _, ok := labels[a.JobID]; !ok {
			if j, err := s.store.GetJob(ctx, uid, a.JobID); err == nil {
				labels[a.JobID] = j.Label()
			}
		}
		counts[a.Kind]++
		lines = append(lines, line{a, labels[a.JobID], s.jobLink(a.JobID)})
	}
	ivs, _ := s.store.ListInterviews(ctx, uid, store.InterviewFilter{From: date, To: store.AddDays(date, 1)})
	due, _, _ := s.store.ListJobs(ctx, uid, store.JobFilter{Status: store.StatusOpen, Due: "today", Today: date, Sort: "due", Limit: 50})
	dueOn := []map[string]any{}
	for _, j := range due {
		if j.NextDue == date {
			dueOn = append(dueOn, map[string]any{"id": j.ID, "label": j.Label(), "next_action": j.NextAction, "url": s.jobLink(j.ID)})
		}
	}
	all, _ := s.store.AllJobs(ctx, uid)
	sent := []map[string]any{}
	for _, j := range all {
		if j.AppliedOn == date {
			sent = append(sent, map[string]any{"id": j.ID, "label": j.Label(), "url": s.jobLink(j.ID)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "activities": lines, "counts": counts, "interviews": ivs, "due": dueOn, "applied": sent,
		"hired": counts["hired"], "offers": counts["offer"], "url": strings.TrimRight(s.cfg.BaseURL, "/") + "/"})
}

// ---- reports ----

type weekRow struct {
	Week       string `json:"week"` // the first day
	Applied    int    `json:"applied"`
	Interviews int    `json:"interviews"`
	Answers    int    `json:"answers"`
}

type countRow struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Answered   int    `json:"answered"`
	Interviews int    `json:"interviews"`
	Offers     int    `json:"offers"`
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "reports")
		return
	}
	weeks := queryInt(r, "weeks")
	if weeks <= 0 || weeks > 104 {
		weeks = 12
	}
	first := weekStart(st, time.Now()).AddDate(0, 0, -7*(weeks-1))
	rowsOut := make([]*weekRow, weeks)
	for i := range rowsOut {
		rowsOut[i] = &weekRow{Week: first.AddDate(0, 0, 7*i).Format("2006-01-02")}
	}
	weekOf := func(day string) *weekRow {
		d, err := time.ParseInLocation("2006-01-02", day, time.Local)
		if err != nil || d.Before(first) {
			return nil
		}
		i := int(d.Sub(first).Hours()/24) / 7
		if i < 0 || i >= weeks {
			return nil
		}
		return rowsOut[i]
	}
	ivs, _ := s.store.ListInterviews(ctx, actorID(ctx), store.InterviewFilter{})
	interviewed := map[int64]bool{}
	for _, iv := range ivs {
		interviewed[iv.JobID] = true
		if row := weekOf(iv.Day()); row != nil {
			row.Interviews++
		}
	}
	funnel := []countRow{{Name: "Applied"}, {Name: "Heard back"}, {Name: "Interviewed"}, {Name: "Offer"}, {Name: "Hired"}}
	sources, modes, ends := map[string]*countRow{}, map[string]*countRow{}, map[string]*countRow{}
	var answerDays []float64
	bump := func(m map[string]*countRow, k string) *countRow {
		if m[k] == nil {
			m[k] = &countRow{Name: k}
		}
		m[k].Count++
		return m[k]
	}
	for _, j := range all {
		if j.AppliedOn == "" {
			continue
		}
		if row := weekOf(j.AppliedOn); row != nil {
			row.Applied++
		}
		if j.RespondedAt != "" {
			if row := weekOf(closedLocal(j.RespondedAt).Format("2006-01-02")); row != nil {
				row.Answers++
			}
			if a, err := time.ParseInLocation("2006-01-02", j.AppliedOn, time.Local); err == nil {
				if b := closedLocal(j.RespondedAt); b.After(a) {
					answerDays = append(answerDays, b.Sub(a).Hours()/24)
				}
			}
		}
		had := interviewed[j.ID]
		offered := j.Reached >= 3 || j.HasOffer
		funnel[0].Count++
		if j.RespondedAt != "" {
			funnel[1].Count++
		}
		if had {
			funnel[2].Count++
		}
		if offered {
			funnel[3].Count++
		}
		if j.Status == store.StatusHired {
			funnel[4].Count++
		}
		for _, row := range []*countRow{bump(sources, firstNonEmpty(j.Source, "not said")), bump(modes, firstNonEmpty(j.WorkMode, "not said"))} {
			if j.RespondedAt != "" {
				row.Answered++
			}
			if had {
				row.Interviews++
			}
			if offered {
				row.Offers++
			}
		}
		if j.Status == store.StatusClosed {
			bump(ends, firstNonEmpty(j.EndReason, "other"))
		}
	}
	list := func(m map[string]*countRow) []*countRow {
		out := []*countRow{}
		for _, v := range m {
			out = append(out, v)
		}
		sort.Slice(out, func(a, b int) bool {
			if out[a].Count != out[b].Count {
				return out[a].Count > out[b].Count
			}
			return out[a].Name < out[b].Name
		})
		return out
	}
	avg := 0
	if len(answerDays) > 0 {
		t := 0.0
		for _, x := range answerDays {
			t += x
		}
		avg = int(t/float64(len(answerDays)) + 0.5)
	}
	writeJSON(w, http.StatusOK, map[string]any{"weeks": rowsOut, "funnel": funnel, "sources": list(sources), "work_modes": list(modes),
		"end_reasons": list(ends), "days_to_answer": avg, "summary": s.summarize(r, st, all)})
}
