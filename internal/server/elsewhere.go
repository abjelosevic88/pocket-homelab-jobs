package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- what the other apps know about a job ----
//
// The emails exchanged with the company (Pocket Mail) and what is in your
// calendar about it (Pocket Calendar), read live and never stored. Each app
// is asked on its own: one that is missing or off only greys out its part.

type elseItem struct {
	Kind   string `json:"kind"` // email | meeting
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	At     string `json:"at"`
	URL    string `json:"url,omitempty"`
	State  string `json:"state,omitempty"`
	Amount string `json:"amount,omitempty"`
	Out    bool   `json:"out,omitempty"` // an email you sent
	Ref    string `json:"ref,omitempty"`
}

type elseSection struct {
	App    string     `json:"app"`
	Name   string     `json:"name"`
	State  string     `json:"state"` // ok | off | error | none (nothing to ask with)
	Detail string     `json:"detail,omitempty"`
	Fix    string     `json:"fix,omitempty"`
	Items  []elseItem `json:"items"`
	Open   string     `json:"open_url,omitempty"`
}

func (s *Server) handleElsewhere(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownJob(w, r)
	if !ok {
		return
	}
	c := s.clients(ctx, d.UserID)
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	type job struct {
		a   pocketApp
		run func(context.Context, *siblingConn) ([]elseItem, error)
		sc  string
		why string // empty = there is something to ask with
	}
	jobs := []job{
		{appMail, func(ctx context.Context, sib *siblingConn) ([]elseItem, error) { return s.mailWith(ctx, sib, d) }, scopeMailRead, whyNot(d.ContactEmail == "" && len([]rune(d.Company)) < 3, "add the company or the contact's email to see your emails about the job")},
		{appCalendar, func(ctx context.Context, sib *siblingConn) ([]elseItem, error) { return s.meetingsWith(ctx, sib, d) }, scopeCalendarRead, whyNot(d.ContactName == "" && d.ContactEmail == "" && d.Company == "", "add the company to see what is in your calendar about it")},
	}
	out := make([]elseSection, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		st := c.status(ctx, j.a)
		sec := elseSection{App: j.a.slug, Name: j.a.name, Items: []elseItem{}, Open: st.Open}
		if j.why != "" {
			sec.State, sec.Detail = "none", j.why
			out[i] = sec
			continue
		}
		if st.Via == "" || !st.Can(j.sc) {
			sec.State, sec.Detail, sec.Fix = "off", st.Detail, st.Fix
			out[i] = sec
			continue
		}
		sib, err := c.conn(ctx, j.a, j.sc)
		if err != nil {
			sec.State, sec.Detail = "off", err.Error()
			out[i] = sec
			continue
		}
		wg.Add(1)
		go func(i int, j job, sec elseSection, sib *siblingConn) {
			defer wg.Done()
			items, err := j.run(ctx, sib)
			if err != nil {
				sec.State, sec.Detail = "error", j.a.name+" did not answer: "+err.Error()
			} else {
				sec.State, sec.Items = "ok", items
				if sec.Items == nil {
					sec.Items = []elseItem{}
				}
			}
			out[i] = sec
		}(i, j, sec, sib)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, out)
}

func whyNot(cond bool, why string) string {
	if cond {
		return why
	}
	return ""
}

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

func (s *Server) mailWith(ctx context.Context, sib *siblingConn, d *store.Job) ([]elseItem, error) {
	var out []elseItem
	seen := map[string]bool{}
	var qs []string
	if d.ContactEmail != "" {
		qs = append(qs, "from:"+d.ContactEmail, "to:"+d.ContactEmail)
	}
	if len([]rune(d.Company)) >= 3 {
		qs = append(qs, `"`+d.Company+`"`)
	}
	for _, q := range qs {
		v, err := getJSONFrom(ctx, sib, "/api/v1/messages", url.Values{"q": {q}, "limit": {"15"}})
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "messages") {
			id := idOf(m)
			if seen[id] {
				continue
			}
			seen[id] = true
			from := obj(m, "from")
			addr := str(from, "addr", "address", "email")
			mine := d.ContactEmail != "" && !strings.EqualFold(addr, d.ContactEmail)
			if d.ContactEmail == "" {
				mine = m["outgoing"] == true || m["sent"] == true
			}
			out = append(out, elseItem{Kind: "email", Title: firstNonEmpty(str(m, "subject"), "(no subject)"), Body: clip(str(m, "snippet"), 240),
				At: str(m, "date"), URL: linkTo(sib, "/message/"+id), Out: mine, Ref: id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

func (s *Server) meetingsWith(ctx context.Context, sib *siblingConn, d *store.Job) ([]elseItem, error) {
	var out []elseItem
	seen := map[string]bool{}
	// who they are, not the role: a title like "Developer" would match too much
	for _, q := range []string{d.ContactEmail, d.ContactName, d.Company} {
		if len([]rune(strings.TrimSpace(q))) < 3 {
			continue
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}})
		if err != nil {
			return nil, err
		}
		for _, e := range rows(v, "events", "items") {
			id := firstNonEmpty(str(e, "id"), idOf(e))
			if seen[id] {
				continue
			}
			seen[id] = true
			eid := firstNonEmpty(fmt.Sprint(int64(num(e, "event_id"))), id)
			out = append(out, elseItem{Kind: "meeting", Title: str(e, "title"), Body: clip(str(e, "location"), 120), At: str(e, "start"),
				URL: linkTo(sib, "/event/"+eid), Ref: id})
		}
		if len(out) >= 15 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	if len(out) > 15 {
		out = out[:15]
	}
	return out, nil
}

// ---- keeping up with the other apps ----

// syncAll looks, for everyone, at the next steps out in Pocket Tasks.
func (s *Server) syncAll(ctx context.Context) {
	if !s.syncMu.TryLock() {
		return
	}
	defer s.syncMu.Unlock()
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		s.syncUser(ctx, uid)
	}
}

// syncUser checks up to 20 next-step tasks: ticked off in Pocket Tasks?
func (s *Server) syncUser(ctx context.Context, uid int64) {
	list, err := s.store.MadeToCheck(ctx, uid, 20)
	if err != nil || len(list) == 0 {
		return
	}
	c := s.clients(ctx, uid)
	for _, m := range list {
		var (
			a     pocketApp
			scope string
			path  string
		)
		switch m.Target {
		case "task":
			a, scope, path = appTasks, scopeTasksRead, "/api/v1/tasks/"+url.PathEscape(m.Ref)
		default:
			continue
		}
		sib, err := optional(c.conn(ctx, a, scope))
		if err != nil || sib == nil {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		v, err := getJSONFrom(ctx, sib, path, nil)
		if err != nil {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		obj0, _ := v.(map[string]any)
		if t := obj(obj0, "task"); t != nil {
			obj0 = t
		}
		state := strings.ToLower(str(obj0, "status"))
		if state == "" || state == m.State {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		_ = s.store.SetMadeState(ctx, uid, m.ID, state, "")
		s.madeChanged(ctx, uid, m, state)
	}
}

// madeChanged writes what changed elsewhere on the job's timeline.
func (s *Server) madeChanged(ctx context.Context, uid int64, m *store.Made, state string) {
	if m.Target != "task" || (state != "completed" && state != "done" && state != "cancelled") {
		return
	}
	d, err := s.store.GetJob(ctx, uid, m.JobID)
	if err == nil && d.NextTaskRef == m.Ref {
		line := "Done (ticked off in Pocket Tasks): " + firstNonEmpty(d.NextAction, m.Title)
		_ = s.store.SetNextStep(ctx, uid, d.ID, "", "", "")
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: d.ID, Kind: "step", Title: line, App: "tasks", URL: m.URL})
	}
}

// handleSyncNow looks at one job's linked things now (the job page asks
// when it opens, so a ticked task shows without waiting).
func (s *Server) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownJob(w, r)
	if !ok {
		return
	}
	s.syncUser(ctx, d.UserID)
	made, _ := s.store.ListMade(ctx, d.UserID, d.ID)
	writeJSON(w, http.StatusOK, made)
}
