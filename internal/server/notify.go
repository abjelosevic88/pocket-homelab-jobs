package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// pushes runs every minute: once a day, at each person's own hour, the
// morning note (when they want it) and jobs.steps_due for Pocket Flows; and
// an hour before each interview, a reminder.
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil {
			continue
		}
		s.interviewReminders(ctx, uid, st, now)
		if !st.NotifyDigest || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.dailyCheck(ctx, uid, st, now)
	}
}

// interviewReminders says an hour before an interview what it is and where.
func (s *Server) interviewReminders(ctx context.Context, uid int64, st store.Settings, now time.Time) {
	if st.NotifyURL == "" || !st.NotifyDigest {
		return
	}
	t := now.Format("2006-01-02")
	ivs, err := s.store.ListInterviews(ctx, uid, store.InterviewFilter{From: t, To: store.AddDays(t, 1)})
	if err != nil {
		return
	}
	for _, iv := range ivs {
		start, _, allDay, err := interviewTimes(iv)
		if err != nil || allDay {
			continue
		}
		left := start.Sub(now)
		if left < 50*time.Minute || left > 65*time.Minute || !s.once(ctx, fmt.Sprintf("remind:%d:%d:%s", uid, iv.ID, iv.StartsAt)) {
			continue
		}
		body := fmt.Sprintf("%s at %s — %s", firstNonEmpty(kindWords[iv.Kind], "Interview"), iv.Clock(), joinNonEmpty(" at ", iv.JobTitle, iv.Company))
		if iv.Location != "" {
			body += "\n" + iv.Location
		}
		if iv.WithPeople != "" {
			body += "\nWith " + iv.WithPeople
		}
		s.push(ctx, uid, st, "Interview in an hour", body, s.jobLink(iv.JobID))
	}
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// dailyCheck builds and sends one person's morning note: today's
// interviews, the next steps due and late, the applications to follow up,
// the jobs to apply for soon and the offers to decide.
func (s *Server) dailyCheck(ctx context.Context, uid int64, st store.Settings, now time.Time) (string, bool) {
	all, err := s.store.AllJobs(ctx, uid)
	if err != nil {
		return "", false
	}
	decorate(st, all...)
	t := now.Format("2006-01-02")
	var due, late, quiet, apply []*store.Job
	for _, j := range all {
		if j.Status != store.StatusOpen {
			continue
		}
		switch {
		case j.NextDue != "" && j.NextDue < t:
			late = append(late, j)
		case j.NextDue == t:
			due = append(due, j)
		}
		if j.Stale {
			quiet = append(quiet, j)
		}
		if j.StageKind == store.KindSaved && j.ApplyBy != "" && j.ApplyBy >= t && j.ApplyBy <= store.AddDays(t, 3) {
			apply = append(apply, j)
		}
	}
	ivs, _ := s.store.ListInterviews(ctx, uid, store.InterviewFilter{From: t, To: store.AddDays(t, 1)})
	offers, _ := s.store.ListOffers(ctx, uid)
	var decide []*store.Offer
	for _, o := range offers {
		if (o.Status == "open" || o.Status == "negotiating") && o.DecideBy != "" && o.DecideBy <= store.AddDays(t, 2) {
			decide = append(decide, o)
		}
	}
	if len(due)+len(late)+len(quiet)+len(apply)+len(ivs)+len(decide) == 0 {
		return "", false
	}
	var lines []string
	for _, iv := range ivs[:min(len(ivs), 3)] {
		lines = append(lines, fmt.Sprintf("Interview %s: %s.", firstNonEmpty(iv.Clock(), "today"), joinNonEmpty(" at ", iv.JobTitle, iv.Company)))
	}
	if len(due) > 0 {
		var parts []string
		for _, j := range due[:min(len(due), 4)] {
			parts = append(parts, fmt.Sprintf("%s (%s)", firstNonEmpty(j.NextAction, "next step"), j.Label()))
		}
		lines = append(lines, fmt.Sprintf("Today: %s.", strings.Join(parts, "; ")))
	}
	if len(late) > 0 {
		lines = append(lines, fmt.Sprintf("%d %s overdue.", len(late), plural(len(late), "step is", "steps are")))
	}
	if len(quiet) > 0 {
		lines = append(lines, fmt.Sprintf("%d %s with no answer for %d days or more: time to follow up.", len(quiet), plural(len(quiet), "application", "applications"), st.StaleDays))
	}
	for _, j := range apply[:min(len(apply), 2)] {
		lines = append(lines, fmt.Sprintf("Apply by %s: %s.", j.ApplyBy[5:], j.Label()))
	}
	for _, o := range decide {
		lines = append(lines, fmt.Sprintf("Decide by %s: the offer from %s.", o.DecideBy[5:], firstNonEmpty(o.Company, o.JobTitle)))
	}
	body := strings.Join(lines, "\n")
	s.publish(ctx, uid, "jobs.steps_due", "steps:"+t, map[string]any{"date": t, "interviews": len(ivs), "due": len(due), "overdue": len(late),
		"follow_up": len(quiet), "apply_soon": len(apply), "offers_to_decide": len(decide), "text": body})
	link := strings.TrimRight(s.cfg.BaseURL, "/") + "/"
	if !s.push(ctx, uid, st, "Your job search today", body, link) {
		return body, false
	}
	return body, true
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"briefcase"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Jobs", Body: notifyTestBody, Priority: "info", Tags: []string{"briefcase"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. Each morning, if you want it, a note says what is on for your job search today, and an hour before each interview a reminder."
