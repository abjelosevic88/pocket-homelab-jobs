package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- interviews ----
//
// An interview belongs to a job. With Pocket Calendar connected it is put
// in the calendar by itself (and moved or removed there with it); without,
// the page offers an .ics file any calendar opens. Its questions can go to
// Pocket Learn as flashcards, its preparation to Pocket Notes, and for one
// in person Pocket Weather says what the weather will be.

var kindWords = map[string]string{"phone": "Phone call", "video": "Video call", "onsite": "On-site interview", "technical": "Technical interview",
	"take_home": "Take-home task", "panel": "Panel interview", "final": "Final interview", "other": "Interview"}

// interviewTitle is how an interview is called in a calendar.
func interviewTitle(iv *store.Interview, j *store.Job) string {
	return clip(firstNonEmpty(kindWords[iv.Kind], "Interview")+" — "+j.Label(), 200)
}

// interviewTimes reads when an interview starts and ends (a day = all day).
func interviewTimes(iv *store.Interview) (start, end time.Time, allDay bool, err error) {
	if iv.Clock() == "" {
		start, err = time.ParseInLocation("2006-01-02", iv.Day(), time.Local)
		return start, start.AddDate(0, 0, 1), true, err
	}
	start, err = time.ParseInLocation("2006-01-02T15:04", iv.StartsAt[:16], time.Local)
	mins := iv.Minutes
	if mins <= 0 {
		mins = 60
	}
	return start, start.Add(time.Duration(mins) * time.Minute), false, err
}

type interviewIn struct {
	Kind       *string     `json:"kind"`
	Date       *string     `json:"date"` // YYYY-MM-DD
	Time       *string     `json:"time"` // HH:MM ("" = the whole day)
	StartsAt   *string     `json:"starts_at"`
	Minutes    *int        `json:"minutes"`
	Location   *string     `json:"location"`
	WithPeople *string     `json:"with_people"`
	Prep       *string     `json:"prep"`
	Questions  *[]store.QA `json:"questions"`
	Outcome    *string     `json:"outcome"`
	Feedback   *string     `json:"feedback"`
	ThanksSent *bool       `json:"thanks_sent"`
	Calendar   *bool       `json:"calendar"` // put it in Pocket Calendar now (default: the setting)
}

func (in interviewIn) apply(iv *store.Interview) error {
	if in.Kind != nil {
		k := strings.ToLower(strings.TrimSpace(*in.Kind))
		if !oneOf(k, store.InterviewKinds) {
			return &userErr{"an interview is one of " + strings.Join(store.InterviewKinds, ", ")}
		}
		iv.Kind = k
	}
	if in.StartsAt != nil && strings.TrimSpace(*in.StartsAt) != "" {
		t, err := parseWhen(strings.TrimSpace(*in.StartsAt))
		if err != nil {
			return &userErr{"the time is like 2026-11-12T14:30"}
		}
		if len(strings.TrimSpace(*in.StartsAt)) == 10 {
			iv.StartsAt = t.Format("2006-01-02")
		} else {
			iv.StartsAt = t.In(time.Local).Format("2006-01-02T15:04")
		}
	}
	if in.Date != nil {
		d := strings.TrimSpace(*in.Date)
		if d == "" || !validDate(d) {
			return &userErr{"pick the day of the interview"}
		}
		clock := iv.Clock()
		if in.Time != nil {
			clock = strings.TrimSpace(*in.Time)
		}
		if clock != "" && !reTime.MatchString(clock) {
			return &userErr{"the time is HH:MM"}
		}
		iv.StartsAt = d
		if clock != "" {
			iv.StartsAt += "T" + clock
		}
	} else if in.Time != nil && iv.Day() != "" {
		clock := strings.TrimSpace(*in.Time)
		if clock != "" && !reTime.MatchString(clock) {
			return &userErr{"the time is HH:MM"}
		}
		iv.StartsAt = iv.Day()
		if clock != "" {
			iv.StartsAt += "T" + clock
		}
	}
	if iv.StartsAt == "" {
		return &userErr{"pick the day of the interview"}
	}
	if in.Minutes != nil {
		if *in.Minutes < 5 || *in.Minutes > 24*60 {
			return &userErr{"an interview is 5 minutes to a day long"}
		}
		iv.Minutes = *in.Minutes
	}
	txt := func(p *string, dst *string, n int) {
		if p != nil {
			*dst = clip(strings.TrimSpace(*p), n)
		}
	}
	txt(in.Location, &iv.Location, 500)
	txt(in.WithPeople, &iv.WithPeople, 500)
	txt(in.Prep, &iv.Prep, 30000)
	txt(in.Feedback, &iv.Feedback, 10000)
	if in.Questions != nil {
		iv.Questions = *in.Questions
		for i := range iv.Questions {
			iv.Questions[i].Q, iv.Questions[i].A = clip(iv.Questions[i].Q, 1000), clip(iv.Questions[i].A, 5000)
		}
	}
	if in.Outcome != nil {
		o := strings.ToLower(strings.TrimSpace(*in.Outcome))
		if o != "" && !oneOf(o, []string{"good", "okay", "bad"}) {
			return &userErr{"how it went is good, okay or bad"}
		}
		iv.Outcome = o
	}
	if in.ThanksSent != nil {
		iv.ThanksSent = *in.ThanksSent
	}
	return nil
}

func (s *Server) handleListInterviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	list, err := s.store.ListInterviews(ctx, actorID(ctx), store.InterviewFilter{JobID: int64(queryInt(r, "job")), When: q.Get("when"),
		From: q.Get("from"), To: q.Get("to"), Limit: queryInt(r, "limit")})
	if err != nil {
		s.fail(w, err, "interviews")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleJobInterviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if _, err := s.store.GetJob(ctx, uid, idParam(r, "id")); err != nil {
		s.fail(w, err, "job")
		return
	}
	list, err := s.store.ListInterviews(ctx, uid, store.InterviewFilter{JobID: idParam(r, "id")})
	if err != nil {
		s.fail(w, err, "interviews")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetInterview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	iv, err := s.store.GetInterview(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	writeJSON(w, http.StatusOK, iv)
}

func (s *Server) handleCreateInterview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	j, err := s.store.GetJob(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	var in interviewIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := s.settingsOf(ctx, uid)
	iv := &store.Interview{UserID: uid, JobID: j.ID, Kind: "video", Minutes: max(st.InterviewMinutes, 15), Questions: []store.QA{}}
	if err := in.apply(iv); err != nil {
		s.userFail(w, err, "interview")
		return
	}
	if err := s.store.CreateInterview(ctx, iv); err != nil {
		s.fail(w, err, "interview")
		return
	}
	_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "interview", Title: "Interview planned: " + firstNonEmpty(kindWords[iv.Kind], "Interview"),
		Body: joinNonEmpty(" · ", whenWords(iv), iv.WithPeople), SourceApp: linkApp(ctx)})
	// an interview means they answered: move a saved or applied job on
	if store.Rank(j.StageKind) < 2 && j.Status == store.StatusOpen {
		if sg, err := s.store.FirstStage(ctx, uid, store.KindActive); err == nil {
			_, _ = s.moveJob(ctx, uid, j.ID, sg.ID, nil, nil, "", "")
		}
	}
	res := map[string]any{"interview": iv}
	toCal := st.CalendarSync
	if in.Calendar != nil {
		toCal = *in.Calendar
	}
	if toCal && !s.cfg.DemoData {
		if err := s.interviewToCalendar(ctx, uid, iv, j); err != nil && !isNotConnected(err) {
			res["calendar_error"] = plainErr(appCalendar, err)
		} else if err != nil && in.Calendar != nil && *in.Calendar {
			res["calendar_error"] = err.Error()
		}
	}
	iv, _ = s.store.GetInterview(ctx, uid, iv.ID)
	res["interview"] = iv
	payload := s.jobEvent(j)
	payload["interview_id"], payload["kind"], payload["starts_at"], payload["location"] = iv.ID, iv.Kind, iv.StartsAt, iv.Location
	s.publish(ctx, uid, "jobs.interview_planned", fmt.Sprintf("interview:%d", iv.ID), payload)
	writeJSON(w, http.StatusCreated, res)
}

// whenWords is an interview's day and time in a few words: "Tue 12 Nov, 14:00".
func whenWords(iv *store.Interview) string {
	d, err := time.ParseInLocation("2006-01-02", iv.Day(), time.Local)
	if err != nil {
		return iv.StartsAt
	}
	out := d.Format("Mon 2 Jan")
	if c := iv.Clock(); c != "" {
		out += ", " + c
	}
	return out
}

func (s *Server) handleUpdateInterview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	var in interviewIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	oldWhen, oldMin, oldLoc, oldOutcome := iv.StartsAt, iv.Minutes, iv.Location, iv.Outcome
	if err := in.apply(iv); err != nil {
		s.userFail(w, err, "interview")
		return
	}
	if err := s.store.UpdateInterview(ctx, iv); err != nil {
		s.fail(w, err, "interview")
		return
	}
	if iv.StartsAt != oldWhen {
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: iv.JobID, Kind: "interview", Title: "Interview moved", Body: whenWords(iv)})
	}
	if iv.Outcome != "" && iv.Outcome != oldOutcome {
		words := map[string]string{"good": "went well", "okay": "went okay", "bad": "did not go well"}[iv.Outcome]
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: iv.JobID, Kind: "interview", Title: firstNonEmpty(kindWords[iv.Kind], "Interview") + " " + words, Body: clip(iv.Feedback, 2000)})
	}
	res := map[string]any{}
	if iv.EventRef != "" && (iv.StartsAt != oldWhen || iv.Minutes != oldMin || iv.Location != oldLoc) {
		if j, err := s.store.GetJob(ctx, uid, iv.JobID); err == nil {
			if err := s.updateCalendarEvent(ctx, uid, iv, j); err != nil {
				res["calendar_error"] = plainErr(appCalendar, err)
			}
		}
	} else if in.Calendar != nil && *in.Calendar && iv.EventRef == "" {
		if j, err := s.store.GetJob(ctx, uid, iv.JobID); err == nil {
			if err := s.interviewToCalendar(ctx, uid, iv, j); err != nil {
				res["calendar_error"] = plainErr(appCalendar, err)
			}
		}
	}
	iv, _ = s.store.GetInterview(ctx, uid, iv.ID)
	res["interview"] = iv
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDeleteInterview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	if err := s.store.DeleteInterview(ctx, uid, iv.ID); err != nil {
		s.fail(w, err, "interview")
		return
	}
	if iv.EventRef != "" && queryBool(r, "calendar") {
		c := s.clients(ctx, uid)
		if sib, err := optional(c.conn(ctx, appCalendar, scopeCalendarWrite)); err == nil && sib != nil {
			_ = sib.client.Do(ctx, http.MethodDelete, "/api/v1/events/"+url.PathEscape(iv.EventRef), nil, nil, nil)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Pocket Calendar ----

func (s *Server) calendarBody(ctx context.Context, iv *store.Interview, j *store.Job) (map[string]any, error) {
	start, end, allDay, err := interviewTimes(iv)
	if err != nil {
		return nil, &userErr{"the interview has no day"}
	}
	desc := joinNonEmpty("\n\n", withLine(iv), clip(iv.Prep, 3000), "From Pocket Jobs — "+j.Label()+": "+s.jobLink(j.ID))
	body := map[string]any{"title": interviewTitle(iv, j), "description": desc, "tz": time.Local.String(), "location": clip(iv.Location, 300), "url": s.jobLink(j.ID)}
	if strings.HasPrefix(iv.Location, "http") {
		body["conference"], body["location"] = iv.Location, ""
	}
	if allDay {
		body["start"], body["all_day"] = iv.Day(), true
	} else {
		body["start"], body["end"], body["all_day"] = start.Format("2006-01-02T15:04"), end.Format("2006-01-02T15:04"), false
	}
	return body, nil
}

func withLine(iv *store.Interview) string {
	if iv.WithPeople == "" {
		return ""
	}
	return "With " + iv.WithPeople
}

// interviewToCalendar puts an interview in the person's Pocket Calendar.
func (s *Server) interviewToCalendar(ctx context.Context, uid int64, iv *store.Interview, j *store.Job) error {
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return err
	}
	body, err := s.calendarBody(ctx, iv, j)
	if err != nil {
		return err
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		return err
	}
	iv.EventRef = idOf(res)
	iv.EventURL = linkTo(sib, "/event/"+iv.EventRef)
	if err := s.store.UpdateInterview(ctx, iv); err != nil {
		return err
	}
	_ = s.store.AddMade(ctx, uid, &store.Made{JobID: j.ID, Target: "event", App: appCalendar.slug, Ref: iv.EventRef, Title: interviewTitle(iv, j), URL: iv.EventURL})
	return nil
}

// updateCalendarEvent moves the interview's event with it.
func (s *Server) updateCalendarEvent(ctx context.Context, uid int64, iv *store.Interview, j *store.Job) error {
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return err
	}
	body, err := s.calendarBody(ctx, iv, j)
	if err != nil {
		return err
	}
	var out map[string]any
	err = sib.client.Do(ctx, http.MethodPut, "/api/v1/events/"+url.PathEscape(iv.EventRef), nil, body, &out)
	if err != nil && strings.Contains(err.Error(), "not found") {
		iv.EventRef, iv.EventURL = "", ""
		_ = s.store.UpdateInterview(ctx, iv)
		return s.interviewToCalendar(ctx, uid, iv, j)
	}
	return err
}

func (s *Server) handleInterviewCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	j, err := s.store.GetJob(ctx, uid, iv.JobID)
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	if iv.EventRef != "" {
		err = s.updateCalendarEvent(ctx, uid, iv, j)
	} else {
		err = s.interviewToCalendar(ctx, uid, iv, j)
	}
	if err != nil {
		s.makeFail(w, appCalendar, err)
		return
	}
	iv, _ = s.store.GetInterview(ctx, uid, iv.ID)
	writeJSON(w, http.StatusOK, iv)
}

// handleInterviewClashes lists what else is in the person's calendar at the
// interview's time (their own event for it left out).
func (s *Server) handleInterviewClashes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	c := s.clients(ctx, uid)
	st := c.status(ctx, appCalendar)
	if st.Via == "" || !st.Can(scopeCalendarRead) {
		writeJSON(w, http.StatusOK, map[string]any{"state": "off", "detail": st.Detail, "fix": st.Fix, "items": []any{}})
		return
	}
	start, end, _, err := interviewTimes(iv)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "none", "items": []any{}})
		return
	}
	sib, err := c.conn(ctx, appCalendar, scopeCalendarRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "off", "detail": err.Error(), "items": []any{}})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/events", url.Values{"from": {start.Format(time.RFC3339)}, "to": {end.Format(time.RFC3339)}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "error", "detail": "Pocket Calendar did not answer: " + err.Error(), "items": []any{}})
		return
	}
	items := []map[string]any{}
	for _, e := range rows(v, "events") {
		id := firstNonEmpty(fmt.Sprint(int64(num(e, "event_id"))), str(e, "id"))
		if base, _, _ := strings.Cut(str(e, "id"), ":"); base == iv.EventRef || id == iv.EventRef {
			continue
		}
		if b, ok := e["busy"].(bool); ok && !b {
			continue
		}
		items = append(items, map[string]any{"title": str(e, "title"), "start": str(e, "start"), "end": str(e, "end"), "all_day": e["all_day"],
			"url": linkTo(sib, "/event/"+firstNonEmpty(strings.Split(str(e, "id"), ":")[0], id))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok", "items": items})
}

// handleInterviewICS is an interview as a calendar file, for when Pocket
// Calendar is not connected: any calendar app opens it.
func (s *Server) handleInterviewICS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	j, err := s.store.GetJob(ctx, uid, iv.JobID)
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	start, end, allDay, err := interviewTimes(iv)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the interview has no day")
		return
	}
	esc := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Pocket Jobs//EN\r\nBEGIN:VEVENT\r\n")
	fmt.Fprintf(&b, "UID:jobs-interview-%d@pocket-jobs\r\nDTSTAMP:%s\r\n", iv.ID, time.Now().UTC().Format("20060102T150405Z"))
	if allDay {
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\nDTEND;VALUE=DATE:%s\r\n", start.Format("20060102"), end.Format("20060102"))
	} else {
		fmt.Fprintf(&b, "DTSTART:%s\r\nDTEND:%s\r\n", start.UTC().Format("20060102T150405Z"), end.UTC().Format("20060102T150405Z"))
	}
	fmt.Fprintf(&b, "SUMMARY:%s\r\nDESCRIPTION:%s\r\n", esc(interviewTitle(iv, j)), esc(joinNonEmpty("\n\n", withLine(iv), clip(iv.Prep, 2000), s.jobLink(j.ID))))
	if iv.Location != "" {
		fmt.Fprintf(&b, "LOCATION:%s\r\n", esc(iv.Location))
	}
	b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Interview\r\nTRIGGER:-PT1H\r\nEND:VALARM\r\n")
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="interview-%d.ics"`, iv.ID))
	_, _ = w.Write([]byte(b.String()))
}

// ---- Pocket Learn: questions as flashcards ----

func (s *Server) handleInterviewCards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	j, err := s.store.GetJob(ctx, uid, iv.JobID)
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	if len(iv.Questions) == 0 {
		writeErr(w, http.StatusBadRequest, "add some questions first (or ask the AI to suggest them)")
		return
	}
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appLearn, scopeLearnWrite)
	if err != nil {
		s.makeFail(w, appLearn, err)
		return
	}
	deck := strings.TrimSpace(c.st.LearnDeck)
	if deck == "" {
		deck = clip("Interview — "+j.Label(), 120)
	}
	made, failed := 0, ""
	for i, q := range iv.Questions {
		back := firstNonEmpty(q.A, "(say your answer out loud, then check your notes)")
		_, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"deck": deck, "kind": "basic", "front": q.Q, "back": back,
			"tags": "interview jobs", "source_url": s.jobLink(j.ID), "external_id": fmt.Sprintf("jobs:%d:%d:%d", j.ID, iv.ID, i)})
		if err != nil {
			if strings.Contains(err.Error(), "409") || strings.Contains(strings.ToLower(err.Error()), "already") {
				continue
			}
			failed = err.Error()
			break
		}
		made++
	}
	if made == 0 && failed != "" {
		writeErr(w, http.StatusBadGateway, "Pocket Learn: "+failed)
		return
	}
	m := &store.Made{JobID: j.ID, Target: "cards", App: appLearn.slug, Title: fmt.Sprintf("%d flashcards in “%s”", made, deck), URL: linkTo(sib, "/")}
	_ = s.store.AddMade(ctx, uid, m)
	_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "made", Title: m.Title, App: appLearn.slug, URL: m.URL})
	writeJSON(w, http.StatusOK, map[string]any{"made": made, "deck": deck, "url": m.URL, "error": failed})
}

// handleInterviewNote saves the preparation for an interview to Pocket Notes.
func (s *Server) handleInterviewNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	j, err := s.store.GetJob(ctx, uid, iv.JobID)
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		s.makeFail(w, appNotes, err)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**When:** %s\n\n", whenWords(iv))
	if iv.Location != "" {
		fmt.Fprintf(&b, "**Where:** %s\n\n", iv.Location)
	}
	if iv.WithPeople != "" {
		fmt.Fprintf(&b, "**With:** %s\n\n", iv.WithPeople)
	}
	if iv.Prep != "" {
		fmt.Fprintf(&b, "%s\n\n", iv.Prep)
	}
	if len(iv.Questions) > 0 {
		b.WriteString("### Questions\n\n")
		for _, q := range iv.Questions {
			fmt.Fprintf(&b, "**%s**\n\n%s\n\n", q.Q, firstNonEmpty(q.A, "—"))
		}
	}
	b.WriteString("— " + s.origin(j))
	title := clip(firstNonEmpty(kindWords[iv.Kind], "Interview")+" prep — "+j.Label(), 200)
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": title, "content": b.String()})
	if err != nil {
		s.makeFail(w, appNotes, err)
		return
	}
	m := &store.Made{JobID: j.ID, Target: "note", App: appNotes.slug, Ref: idOf(res), Title: title, URL: linkTo(sib, "/note/"+idOf(res))}
	_ = s.store.AddMade(ctx, uid, m)
	writeJSON(w, http.StatusCreated, m)
}

// ---- Pocket Weather ----

// handleInterviewWeather is the forecast at an in-person interview's place
// and hour (when it is within the next ten days).
func (s *Server) handleInterviewWeather(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	iv, err := s.store.GetInterview(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "interview")
		return
	}
	none := func(state, detail string) {
		writeJSON(w, http.StatusOK, map[string]any{"state": state, "detail": detail})
	}
	place := strings.TrimSpace(iv.Location)
	if j, err := s.store.GetJob(ctx, uid, iv.JobID); err == nil && (place == "" || strings.HasPrefix(place, "http")) && iv.Kind == "onsite" {
		place = j.Location
	}
	if place == "" || strings.HasPrefix(place, "http") || oneOf(iv.Kind, []string{"phone", "video", "take_home"}) {
		none("none", "not in person")
		return
	}
	start, _, allDay, err := interviewTimes(iv)
	if err != nil || start.Before(time.Now().Add(-2*time.Hour)) || start.After(time.Now().AddDate(0, 0, 10)) {
		none("none", "the forecast covers the next ten days")
		return
	}
	c := s.clients(ctx, uid)
	st := c.status(ctx, appWeather)
	if st.Via == "" || !st.Can(scopeWeatherRead) {
		writeJSON(w, http.StatusOK, map[string]any{"state": "off", "detail": st.Detail, "fix": st.Fix})
		return
	}
	sib, err := c.conn(ctx, appWeather, scopeWeatherRead)
	if err != nil {
		none("off", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	// the place: the last part of an address is usually the town
	var lat, lon float64
	var name string
	for _, q := range placeQueries(place) {
		v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}})
		if err != nil {
			none("error", "Pocket Weather did not answer: "+err.Error())
			return
		}
		if hits := rows(v, "results", "items", "places"); len(hits) > 0 {
			lat, lon, name = num(hits[0], "lat"), num(hits[0], "lon"), firstNonEmpty(str(hits[0], "name"), q)
			break
		}
	}
	if name == "" {
		none("none", "Pocket Weather does not know that place")
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/forecast", url.Values{"lat": {fmt.Sprint(lat)}, "lon": {fmt.Sprint(lon)}, "hours": {"240"}})
	if err != nil {
		none("error", "Pocket Weather did not answer: "+err.Error())
		return
	}
	out := map[string]any{"state": "ok", "place": name, "open_url": st.Open}
	f, _ := v.(map[string]any)
	if !allDay {
		want := start.Format("2006-01-02T15")
		for _, h := range rows(f, "hours") {
			if strings.HasPrefix(str(h, "time", "at"), want) {
				out["hour"] = h
				break
			}
		}
	}
	for _, d := range rows(f, "days") {
		if strings.HasPrefix(str(d, "date", "day"), iv.Day()) {
			out["day"] = d
			break
		}
	}
	if out["hour"] == nil && out["day"] == nil {
		none("none", "no forecast for that day yet")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// placeQueries are the ways to look a place up: whole, then its last parts.
func placeQueries(p string) []string {
	out := []string{p}
	parts := strings.Split(p, ",")
	for i := len(parts) - 1; i >= 0 && len(out) < 3; i-- {
		t := strings.TrimSpace(strings.TrimLeft(parts[i], "0123456789 "))
		if len([]rune(t)) >= 3 && !oneOf(t, out) {
			out = append(out, t)
		}
	}
	return out
}
