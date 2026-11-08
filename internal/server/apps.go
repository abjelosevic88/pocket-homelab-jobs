package server

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- a job in the other Pocket apps ----
//
// From one job, as its owner (their own Core link or direct token): the
// recruiter and the company into Pocket Contacts (or linked to the cards
// there), the next step as a task in Pocket Tasks, the job as a note in
// Pocket Notes, an email from Pocket Mail. Interviews go to Pocket Calendar
// and Pocket Learn from interviews_api.go. Each thing made is remembered
// with a link to it; a failed try says why in plain words.
//
// Nothing here is needed for the job itself: when an app is not on the box,
// switched off, or too old, the job keeps the person, the step and the
// email text on its own, and the page says what to switch on.

// origin is the line that says where a thing came from.
func (s *Server) origin(j *store.Job) string {
	return fmt.Sprintf("From Pocket Jobs — “%s”: %s", j.Label(), s.jobLink(j.ID))
}

// salaryWords is a job's salary range in a few words ("" = not said).
func salaryWords(j *store.Job) string {
	per := map[string]string{"year": "a year", "month": "a month", "hour": "an hour"}[j.SalaryPeriod]
	switch {
	case j.SalaryMin > 0 && j.SalaryMax > 0 && j.SalaryMin != j.SalaryMax:
		return fmtMoney(j.SalaryMin, "") + "–" + fmtMoney(j.SalaryMax, j.Currency) + " " + per
	case j.SalaryMax > 0:
		return fmtMoney(j.SalaryMax, j.Currency) + " " + per
	case j.SalaryMin > 0:
		return "from " + fmtMoney(j.SalaryMin, j.Currency) + " " + per
	}
	return ""
}

// jobMarkdown is a job as Markdown (a note).
func (s *Server) jobMarkdown(ctx context.Context, j *store.Job, timeline bool) string {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "**%s:** %s\n\n", k, v)
		}
	}
	line("Company", j.Company)
	line("Where", joinNonEmpty(" · ", j.Location, modeWords[j.WorkMode]))
	line("Salary", salaryWords(j))
	line("Posting", j.URL)
	line("Contact", joinNonEmpty(" · ", j.ContactName, j.ContactEmail, j.ContactPhone))
	line("Stage", j.StageName)
	line("Applied", j.AppliedOn)
	if j.Notes != "" {
		fmt.Fprintf(&b, "%s\n\n", j.Notes)
	}
	if j.Posting != "" {
		fmt.Fprintf(&b, "### The posting\n\n%s\n\n", clip(j.Posting, 12000))
	}
	if timeline {
		acts, _ := s.store.ListActivities(ctx, j.UserID, j.ID, 60)
		if len(acts) > 0 {
			b.WriteString("### Timeline\n\n")
			for _, a := range acts {
				l := a.Title
				if a.Body != "" {
					l = joinNonEmpty(": ", a.Title, strings.ReplaceAll(clip(a.Body, 400), "\n", " "))
				}
				fmt.Fprintf(&b, "- %s — %s\n", day10(a.At), l)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

var modeWords = map[string]string{"onsite": "on site", "hybrid": "hybrid", "remote": "remote"}

// fmtMoney is 450000 cents EUR as "4,500 EUR" (cents only when there are some).
func fmtMoney(cents int64, cur string) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole, frac := cents/100, cents%100
	s := fmt.Sprintf("%d", whole)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if frac != 0 {
		s += fmt.Sprintf(".%02d", frac)
	}
	if neg {
		s = "-" + s
	}
	return strings.TrimSpace(s + " " + cur)
}

type makeIn struct {
	Target     string `json:"target"` // contact | task | note | email
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	Date       string `json:"date"`
	Time       string `json:"time"`
	Minutes    int    `json:"minutes"`
	Location   string `json:"location"`
	Due        string `json:"due"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	To         string `json:"to"`
	TemplateID int64  `json:"template_id"`
	TeamID     int64  `json:"team_id"`
	Send       bool   `json:"send"`
	Timeline   bool   `json:"timeline"`
	ContactRef string `json:"contact_ref"` // contact: link this card instead of looking
}

var makeTargets = map[string]pocketApp{"contact": appContacts, "task": appTasks, "note": appNotes, "email": appMail}

// make makes one thing for a job in another app and remembers it.
func (s *Server) make(ctx context.Context, c *clients, d *store.Job, in makeIn) (*store.Made, error) {
	a, ok := makeTargets[in.Target]
	if !ok {
		return nil, &userErr{"make a contact, task, note or email"}
	}
	m := &store.Made{JobID: d.ID, Target: in.Target, App: a.slug}
	var (
		res  map[string]any
		path string
		err  error
	)
	switch in.Target {
	case "contact":
		return s.linkContact(ctx, c, d, in.ContactRef)
	case "task":
		res, path, err = s.makeTask(ctx, c, d, in)
	case "note":
		res, path, err = s.makeNote(ctx, c, d, in)
	case "email":
		err = s.sendEmail(ctx, c, d, in)
		m.Title = firstNonEmpty(in.Subject, "Email")
	}
	if err != nil {
		return nil, err
	}
	if res != nil {
		m.Ref = idOf(res)
		m.Title = firstNonEmpty(str(res, "title", "name", "number", "fn"), in.Title, m.Title)
		m.State = str(res, "status")
		if sib, _ := c.conn(ctx, a, a.scopes[len(a.scopes)-1]); sib != nil && sib.public != "" && m.Ref != "" && path != "" {
			m.URL = strings.TrimRight(sib.public, "/") + path
		}
	}
	s.store.DropMadeErrors(ctx, d.UserID, d.ID, m.Target)
	if in.Target != "email" {
		if err := s.store.AddMade(ctx, d.UserID, m); err != nil {
			return nil, err
		}
	}
	s.madeLine(ctx, d, m, in)
	return m, nil
}

// madeLine writes what was made on the job's timeline.
func (s *Server) madeLine(ctx context.Context, d *store.Job, m *store.Made, in makeIn) {
	a := &store.Activity{UserID: d.UserID, JobID: d.ID, Kind: "made", App: m.App, URL: m.URL}
	switch m.Target {
	case "task":
		a.Title = "Task made: " + m.Title
	case "note":
		a.Title = "Saved as a note"
	case "email":
		a.Kind, a.Title, a.Body = "email", "Email sent: "+firstNonEmpty(in.Subject, "(no subject)"), clip(in.Body, 4000)
	default:
		return
	}
	_ = s.store.AddActivity(ctx, a)
}

// plainErr is a failed try in a sentence.
func plainErr(a pocketApp, err error) string {
	msg := err.Error()
	var ue *userErr
	if isNotConnected(err) || errors.As(err, &ue) {
		return msg
	}
	return a.name + ": " + msg
}

// ---- Pocket Contacts ----

// splitName turns "Ana Marić" into given and family names.
func splitName(n string) (given, family string) {
	parts := strings.Fields(n)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

// findCompany is the company card with this name, if there is one.
func findCompany(ctx context.Context, sib *siblingConn, name string) map[string]any {
	if name == "" {
		return nil
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {name}, "limit": {"20"}})
	if err != nil {
		return nil
	}
	for _, c := range rows(v, "contacts") {
		if str(c, "kind") == "org" && strings.EqualFold(strings.TrimSpace(str(c, "fn", "org")), name) {
			return c
		}
	}
	return nil
}

// firstValue is the first email or phone of a card (strings or {value}).
func firstValue(v any) string {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return ""
	}
	switch x := l[0].(type) {
	case string:
		return x
	case map[string]any:
		return str(x, "value")
	}
	return ""
}

// linkContact links the job to the person's card in Pocket Contacts: the
// one picked, the one with their email, or a new one (and the company's).
func (s *Server) linkContact(ctx context.Context, c *clients, d *store.Job, pick string) (*store.Made, error) {
	if d.ContactName == "" && d.ContactEmail == "" && d.Company == "" && pick == "" {
		return nil, &userErr{"say who to talk to first (a name, an email or the company)"}
	}
	rs, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		return nil, err
	}
	m := &store.Made{JobID: d.ID, Target: "contact", App: appContacts.slug}
	cardURL := func(ref string) string {
		if rs.public == "" || ref == "" {
			return ""
		}
		return strings.TrimRight(rs.public, "/") + "/contacts/" + ref
	}
	link := func(card map[string]any) (*store.Made, error) {
		ref := idOf(card)
		contactRef, companyRef := ref, ""
		if str(card, "kind") == "org" {
			contactRef, companyRef = "", ref
		} else if org := findCompany(ctx, rs, d.Company); org != nil {
			companyRef = idOf(org)
		}
		_ = s.store.SetContactRef(ctx, d.UserID, d.ID, contactRef, companyRef)
		m.Ref, m.Title, m.URL = ref, firstNonEmpty(str(card, "fn", "name", "display_name"), d.ContactName, d.Company), cardURL(ref)
		return m, nil
	}
	if pick != "" {
		v, err := getJSONFrom(ctx, rs, "/api/v1/contacts/"+url.PathEscape(pick), nil)
		if err != nil {
			return nil, err
		}
		card, _ := v.(map[string]any)
		if card == nil {
			return nil, &userErr{"that contact is not there any more"}
		}
		if inner := obj(card, "card"); inner != nil {
			inner["id"] = card["id"]
			if k := str(card, "kind"); k != "" {
				inner["kind"] = k
			}
			card = inner
		}
		// the card's details fill what the job does not know yet
		changed := false
		fill := func(dst *string, v string) {
			if *dst == "" && v != "" {
				*dst, changed = v, true
			}
		}
		if str(card, "kind") == "org" {
			fill(&d.Company, str(card, "fn", "org"))
		} else {
			fill(&d.ContactName, str(card, "fn"))
			fill(&d.Company, str(card, "org"))
			fill(&d.ContactEmail, strings.ToLower(firstValue(card["emails"])))
			fill(&d.ContactPhone, firstValue(card["phones"]))
		}
		if changed {
			_ = s.store.UpdateJob(ctx, d)
		}
		return link(card)
	}
	if d.ContactEmail != "" {
		if v, err := getJSONFrom(ctx, rs, "/api/v1/lookup", url.Values{"email": {d.ContactEmail}}); err == nil {
			if found := rows(v, "contacts"); len(found) > 0 {
				return link(found[0])
			}
		}
	}
	if d.ContactName == "" && d.ContactEmail == "" {
		if org := findCompany(ctx, rs, d.Company); org != nil {
			return link(org)
		}
	}
	ws, err := c.conn(ctx, appContacts, scopeContactsWrite)
	if err != nil {
		return nil, err
	}
	companyRef := ""
	if d.Company != "" {
		if org := findCompany(ctx, rs, d.Company); org != nil {
			companyRef = idOf(org)
		} else if res, err := postJSONTo(ctx, ws, "/api/v1/contacts", map[string]any{"card": map[string]any{"kind": "org", "fn": d.Company, "org": d.Company, "note": s.origin(d)}}); err == nil {
			companyRef = idOf(res)
		}
	}
	if d.ContactName == "" && d.ContactEmail == "" {
		if companyRef == "" {
			return nil, &userErr{"Pocket Contacts did not take the company"}
		}
		_ = s.store.SetContactRef(ctx, d.UserID, d.ID, "", companyRef)
		m.Ref, m.Title, m.URL = companyRef, d.Company+" (added)", cardURL(companyRef)
		return m, nil
	}
	given, family := splitName(d.ContactName)
	card := map[string]any{"kind": "individual", "fn": firstNonEmpty(d.ContactName, d.ContactEmail), "given": given, "family": family, "org": d.Company, "note": s.origin(d)}
	if d.ContactEmail != "" {
		card["emails"] = []map[string]any{{"label": "Work", "value": d.ContactEmail, "pref": true}}
	}
	if d.ContactPhone != "" {
		card["phones"] = []map[string]any{{"label": "Work", "value": d.ContactPhone}}
	}
	res, err := postJSONTo(ctx, ws, "/api/v1/contacts", map[string]any{"card": card})
	if err != nil {
		return nil, err
	}
	ref := idOf(res)
	_ = s.store.SetContactRef(ctx, d.UserID, d.ID, ref, companyRef)
	m.Ref, m.Title, m.URL = ref, firstNonEmpty(str(res, "fn", "name", "display_name"), d.ContactName, d.ContactEmail)+" (added)", cardURL(ref)
	_ = s.store.AddActivity(ctx, &store.Activity{UserID: d.UserID, JobID: d.ID, Kind: "made", Title: "Added to Pocket Contacts: " + firstNonEmpty(d.ContactName, d.ContactEmail), App: "contacts", URL: m.URL})
	return m, nil
}

// handleContactSearch is the person picker on a job: cards from Pocket Contacts.
func (s *Server) handleContactSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}})
		return
	}
	sib, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}, "limit": {"12"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": "Pocket Contacts: " + err.Error()})
		return
	}
	out := []map[string]any{}
	for _, x := range rows(v, "contacts") {
		out = append(out, map[string]any{"id": idOf(x), "kind": str(x, "kind"), "name": str(x, "fn"), "company": str(x, "org"), "title": str(x, "title"),
			"email": firstValue(x["emails"]), "phone": firstValue(x["phones"]), "initials": str(x, "initials"), "color": str(x, "color")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// ---- Pocket Tasks ----

func (s *Server) makeTask(ctx context.Context, c *clients, d *store.Job, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appTasks, scopeTasksWrite)
	if err != nil {
		return nil, "", err
	}
	title := firstNonEmpty(strings.TrimSpace(in.Title), d.NextAction, "Follow up on "+d.Label())
	if !strings.Contains(title, d.Label()) {
		title += " — " + d.Label()
	}
	body := map[string]any{"title": clip(title, 200), "notes": clip(joinNonEmpty("\n\n", in.Notes, s.origin(d)), 20000), "url": s.jobLink(d.ID)}
	if due := firstNonEmpty(in.Due, in.Date); due != "" {
		if !validDate(due) {
			return nil, "", &userErr{"the due day is YYYY-MM-DD"}
		}
		body["due"] = due
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/tasks", body)
	if err != nil {
		return nil, "", err
	}
	return res, "/today?task=" + idOf(res), nil
}

// mirrorStep keeps the job's next step as a task in Pocket Tasks (when
// the person wants that and Tasks is connected): made once, then renamed
// and moved with the step.
func (s *Server) mirrorStep(ctx context.Context, uid, jobID int64) {
	d, err := s.store.GetJob(ctx, uid, jobID)
	if err != nil || d.Status != store.StatusOpen || d.NextAction == "" {
		return
	}
	c := s.clients(ctx, uid)
	if d.NextTaskRef != "" {
		sib, err := optional(c.conn(ctx, appTasks, scopeTasksWrite))
		if err != nil || sib == nil {
			return
		}
		body := map[string]any{"title": clip(d.NextAction+" — "+d.Label(), 200)}
		if d.NextDue != "" {
			body["due"] = d.NextDue
		}
		var out map[string]any
		if err := sib.client.Do(ctx, http.MethodPut, "/api/v1/tasks/"+url.PathEscape(d.NextTaskRef), nil, body, &out); err != nil {
			s.log.Debug("tasks: update step", "job", d.ID, "err", err)
		}
		return
	}
	if !c.st.TaskMirror {
		return
	}
	res, path, err := s.makeTask(ctx, c, d, makeIn{Title: d.NextAction, Due: d.NextDue})
	if err != nil {
		if !isNotConnected(err) {
			s.log.Debug("tasks: mirror step", "job", d.ID, "err", err)
		}
		return
	}
	ref := idOf(res)
	_ = s.store.SetNextStep(ctx, uid, d.ID, d.NextAction, d.NextDue, ref)
	m := &store.Made{JobID: d.ID, Target: "task", App: "tasks", Ref: ref, Title: d.NextAction, State: "open"}
	if sib, _ := c.conn(ctx, appTasks, scopeTasksWrite); sib != nil && sib.public != "" {
		m.URL = strings.TrimRight(sib.public, "/") + path
	}
	_ = s.store.AddMade(ctx, uid, m)
}

// completeTask ticks a task off in Pocket Tasks (the step was done here).
func (s *Server) completeTask(ctx context.Context, uid int64, ref string) {
	c := s.clients(ctx, uid)
	sib, err := optional(c.conn(ctx, appTasks, scopeTasksWrite))
	if err != nil || sib == nil {
		return
	}
	var out map[string]any
	if err := sib.client.Do(ctx, http.MethodPost, "/api/v1/tasks/"+url.PathEscape(ref)+"/complete", nil, map[string]any{}, &out); err != nil {
		s.log.Debug("tasks: complete", "task", ref, "err", err)
	}
}

// ---- Pocket Notes ----

func (s *Server) makeNote(ctx context.Context, c *clients, d *store.Job, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		return nil, "", err
	}
	title := firstNonEmpty(strings.TrimSpace(in.Title), d.Label())
	content := joinNonEmpty("\n\n", in.Notes, s.jobMarkdown(ctx, d, true), "— "+s.origin(d))
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(title, 200), "content": content})
	if err != nil {
		return nil, "", err
	}
	return res, "/note/" + idOf(res), nil
}

// ---- Pocket Mail ----

func mailHTML(body string) string {
	var h strings.Builder
	h.WriteString(`<!doctype html><html><head><meta charset="utf-8"></head><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2433;line-height:1.55">`)
	for _, p := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		h.WriteString(`<p style="margin:0 0 12px">` + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(p)), "\n", "<br>") + `</p>`)
	}
	h.WriteString(`</body></html>`)
	return h.String()
}

func (s *Server) sendEmail(ctx context.Context, c *clients, d *store.Job, in makeIn) error {
	to := strings.ToLower(firstNonEmpty(strings.TrimSpace(in.To), d.ContactEmail))
	if to == "" || !reEmail.MatchString(to) {
		return &userErr{"this job has no email address to write to (add the contact's email)"}
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return &userErr{"write the email first"}
	}
	subject := clip(firstNonEmpty(strings.TrimSpace(in.Subject), d.Label()), 200)
	if s.cfg.DemoData {
		s.log.Debug("demo: email not sent", "to", to, "subject", subject)
		return nil
	}
	sib, err := c.conn(ctx, appMail, scopeMailSend)
	if err != nil {
		return err
	}
	msg := map[string]any{"from": c.st.MailFrom, "to": []map[string]string{{"name": d.ContactName, "addr": to}}, "subject": subject,
		"body_text": body, "body_html": mailHTML(body)}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out map[string]any
	return sib.client.Do(ctx, http.MethodPost, "/api/v1/send", nil, msg, &out)
}

// ---- the handlers ----

func (s *Server) ownJob(w http.ResponseWriter, r *http.Request) (*store.Job, bool) {
	ctx := r.Context()
	d, err := s.store.GetJob(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "job")
		return nil, false
	}
	return d, true
}

func (s *Server) makeFail(w http.ResponseWriter, a pocketApp, err error) {
	var ue *userErr
	switch {
	case errors.As(err, &ue), isNotConnected(err):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
	}
}

func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownJob(w, r)
	if !ok {
		return
	}
	var in makeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := s.clients(ctx, d.UserID)
	m, err := s.make(ctx, c, d, in)
	if err != nil {
		s.makeFail(w, makeTargets[in.Target], err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// handleApps says per app whether jobs can reach it, and if not why and
// what to do — the pages grey out what does not work.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}
