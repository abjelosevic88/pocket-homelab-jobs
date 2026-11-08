package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- Pocket AI on a job search ----
//
// Every help is for one person and sees only their own things: a job (its
// posting, notes and timeline), the CV picked for it (else their main CV),
// the few lines they wrote about themselves, and for an offer the others
// they have. Nothing is saved unless the person keeps it — except the fit
// score, which shows on the job.

const aiSystem = `You help someone look for a job. Be honest, warm and practical, like a sensible friend who hires people for a living. Write plainly and briefly; no jargon, no hype, no emoji. Never invent experience the person does not have. Use the language the job posting is written in, unless the person writes in another.`

// aiReady is the person's AI with the busy-wait, or an error already written.
func (s *Server) aiReady(w http.ResponseWriter, ctx context.Context, uid int64) (*ai.Client, store.Settings, func(), bool) {
	st := s.settingsOf(ctx, uid)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off (Settings → AI)")
		return nil, st, nil, false
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusServiceUnavailable, firstNonEmpty(why, "no AI is set up (Settings → AI)"))
		return nil, st, nil, false
	}
	select {
	case s.aiSem <- struct{}{}:
		return cl, st, func() { <-s.aiSem }, true
	case <-time.After(20 * time.Second):
		writeErr(w, http.StatusServiceUnavailable, "the AI is busy, try again in a moment")
		return nil, st, nil, false
	}
}

// cvFor is the CV to use with a job: the one picked for it, else the main one.
func (s *Server) cvFor(ctx context.Context, j *store.Job) *store.Doc {
	if j != nil && j.CVID > 0 {
		if d, err := s.store.GetDoc(ctx, j.UserID, j.CVID); err == nil && d.Body != "" {
			return d
		}
	}
	uid := int64(0)
	if j != nil {
		uid = j.UserID
	}
	if d, err := s.store.DefaultCV(ctx, uid); err == nil && d.Body != "" {
		return d
	}
	return nil
}

func (s *Server) jobBrief(ctx context.Context, j *store.Job, timeline bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Job: %s\nStage: %s (%s)\n", j.Label(), j.StageName, j.Status)
	if w := joinNonEmpty(" · ", j.Location, modeWords[j.WorkMode], strings.ReplaceAll(j.Employment, "_", " ")); w != "" {
		fmt.Fprintf(&b, "Where/what: %s\n", w)
	}
	if sw := salaryWords(j); sw != "" {
		fmt.Fprintf(&b, "Salary: %s\n", sw)
	}
	if j.ContactName != "" || j.ContactEmail != "" {
		fmt.Fprintf(&b, "Contact: %s\n", joinNonEmpty(" ", j.ContactName, "<"+j.ContactEmail+">"))
	}
	if j.AppliedOn != "" {
		fmt.Fprintf(&b, "Applied: %s\n", j.AppliedOn)
	}
	if j.NextAction != "" {
		fmt.Fprintf(&b, "Next step planned: %s (due %s)\n", j.NextAction, firstNonEmpty(j.NextDue, "no day"))
	}
	fmt.Fprintf(&b, "Today: %s\n", today())
	if j.Notes != "" {
		fmt.Fprintf(&b, "\nMy notes:\n%s\n", clip(j.Notes, 3000))
	}
	if j.Posting != "" {
		fmt.Fprintf(&b, "\nThe posting:\n%s\n", clip(j.Posting, 12000))
	}
	if timeline {
		acts, _ := s.store.ListActivities(ctx, j.UserID, j.ID, 30)
		if len(acts) > 0 {
			b.WriteString("\nTimeline (newest first):\n")
			for _, a := range acts {
				fmt.Fprintf(&b, "- %s [%s] %s", day10(a.At), a.Kind, a.Title)
				if a.Body != "" {
					fmt.Fprintf(&b, ": %s", strings.ReplaceAll(clip(a.Body, 400), "\n", " "))
				}
				b.WriteString("\n")
			}
		}
		ivs, _ := s.store.ListInterviews(ctx, j.UserID, store.InterviewFilter{JobID: j.ID})
		for _, iv := range ivs {
			fmt.Fprintf(&b, "- Interview %s: %s%s\n", whenWords(iv), firstNonEmpty(kindWords[iv.Kind], "interview"), joinNonEmpty("", ""+withPrefix(iv.WithPeople), outcomeSuffix(iv)))
		}
	}
	return b.String()
}

func withPrefix(v string) string {
	if v == "" {
		return ""
	}
	return " with " + v
}

func outcomeSuffix(iv *store.Interview) string {
	if iv.Outcome == "" {
		return ""
	}
	return " — it " + map[string]string{"good": "went well", "okay": "went okay", "bad": "did not go well"}[iv.Outcome]
}

// meBrief is what the person told about themselves and their CV.
func (s *Server) meBrief(st store.Settings, cv *store.Doc) string {
	var b strings.Builder
	if st.AboutMe != "" {
		fmt.Fprintf(&b, "About me:\n%s\n\n", clip(st.AboutMe, 3000))
	}
	if cv != nil {
		fmt.Fprintf(&b, "My CV (%s):\n%s\n", cv.Name, clip(cv.Body, 14000))
	} else {
		b.WriteString("(No CV text yet.)\n")
	}
	return b.String()
}

// ---- reading a posting ----

// handleReadPosting reads a posting (a link, pasted text, or both) into the
// fields of a new job. The page's JobPosting data fills what it can; the AI
// fills the rest from the text, when there is one.
func (s *Server) handleReadPosting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.URL, in.Text = strings.TrimSpace(in.URL), strings.TrimSpace(in.Text)
	if in.URL == "" && in.Text == "" {
		writeErr(w, http.StatusBadRequest, "paste the link or the text of the posting")
		return
	}
	p := &posting{}
	notes := []string{}
	if in.URL != "" {
		if !strings.Contains(in.URL, "://") {
			in.URL = "https://" + in.URL
		}
		got, err := s.fetchPosting(ctx, in.URL)
		if err != nil {
			if in.Text == "" {
				s.userFail(w, err, "posting")
				return
			}
			notes = append(notes, err.Error())
			p.URL = in.URL
		} else {
			p = got
		}
		if j, err := s.store.JobByURL(ctx, uid, firstNonEmpty(p.URL, in.URL)); err == nil {
			notes = append(notes, fmt.Sprintf("You saved this job before: %s", j.Label()))
		}
	}
	if in.Text != "" {
		p.Text = clip(in.Text, 30000)
	}
	usedAI := false
	st := s.settingsOf(ctx, uid)
	if cl, _, _ := s.aiFor(ctx, uid, st); cl != nil && st.AIHelp && p.Text != "" && (!p.Structured || in.Text != "") {
		select {
		case s.aiSem <- struct{}{}:
			var out struct {
				Title      string  `json:"title"`
				Company    string  `json:"company"`
				Location   string  `json:"location"`
				WorkMode   string  `json:"work_mode"`
				Employment string  `json:"employment"`
				Min        float64 `json:"salary_min"`
				Max        float64 `json:"salary_max"`
				Currency   string  `json:"currency"`
				Period     string  `json:"salary_period"`
				ApplyBy    string  `json:"apply_by"`
				Contact    string  `json:"contact_name"`
				Email      string  `json:"contact_email"`
				Summary    string  `json:"summary"`
			}
			err := cl.JSON(ctx, aiSystem+` Read a job posting and answer as JSON {"title": "", "company": "", "location": "", "work_mode": "onsite|hybrid|remote|", "employment": "full_time|part_time|contract|freelance|internship|", "salary_min": 0, "salary_max": 0, "currency": "EUR", "salary_period": "year|month|hour", "apply_by": "YYYY-MM-DD or empty", "contact_name": "", "contact_email": "", "summary": "three short lines: what the job is, what they want most, anything to watch out for"}. Leave out what the posting does not say (empty string, 0). Never guess a salary.`,
				"Today: "+today()+"\n\nThe page title: "+p.Title+"\n\nThe posting:\n"+clip(p.Text, 14000), &out)
			<-s.aiSem
			if err == nil {
				usedAI = true
				fill := func(dst *string, v string) {
					if *dst == "" {
						*dst = strings.TrimSpace(v)
					}
				}
				if !p.Structured {
					p.Title = "" // the page's <title> is often "Jobs at X | Site"
				}
				fill(&p.Title, out.Title)
				fill(&p.Company, out.Company)
				fill(&p.Location, out.Location)
				if oneOf(out.WorkMode, store.WorkModes) {
					fill(&p.WorkMode, out.WorkMode)
				}
				if oneOf(out.Employment, store.Employments) {
					fill(&p.Employment, out.Employment)
				}
				if p.SalaryMin == 0 && p.SalaryMax == 0 && (out.Min > 0 || out.Max > 0) {
					p.SalaryMin, p.SalaryMax = out.Min, out.Max
					fill(&p.Currency, strings.ToUpper(out.Currency))
					if oneOf(out.Period, store.Periods) {
						fill(&p.Period, out.Period)
					}
				}
				if validDate(out.ApplyBy) && out.ApplyBy >= today() {
					fill(&p.ApplyBy, out.ApplyBy)
				}
				res := map[string]any{"posting": p, "contact_name": strings.TrimSpace(out.Contact), "contact_email": strings.ToLower(strings.TrimSpace(out.Email)),
					"summary": strings.TrimSpace(out.Summary), "ai": true, "notes": notes}
				writeJSON(w, http.StatusOK, res)
				return
			}
			notes = append(notes, "The AI could not read it: "+err.Error())
		case <-time.After(15 * time.Second):
			notes = append(notes, "The AI is busy; filled what the page says")
		}
	}
	if p.Title == "" && !usedAI && p.Text != "" {
		// the first short line of pasted text is usually the role
		for _, l := range strings.Split(p.Text, "\n") {
			if l = strings.TrimSpace(l); l != "" && len([]rune(l)) <= 90 {
				p.Title = l
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"posting": p, "ai": false, "notes": notes})
}

// ---- helps on one job ----

func (s *Server) handleAIJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	j, ok := s.ownJob(w, r)
	if !ok {
		return
	}
	var in struct {
		Want        string `json:"want"` // fit | letter | tailor | prep | email | next | summary | research
		Hint        string `json:"hint"`
		Kind        string `json:"kind"` // email: follow_up | thank_you | accept | decline | withdraw | ask
		InterviewID int64  `json:"interview_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cl, st, done, ok := s.aiReady(w, ctx, j.UserID)
	if !ok {
		return
	}
	defer done()
	cv := s.cvFor(ctx, j)
	me := s.meBrief(st, cv)
	hint := ""
	if h := strings.TrimSpace(in.Hint); h != "" {
		hint = "\n\nAlso keep this in mind: " + clip(h, 1500)
	}
	name := ""
	if u := userFrom(ctx); u != nil {
		name = u.Name
	}
	fail := func(err error) { writeErr(w, http.StatusBadGateway, err.Error()) }
	switch in.Want {
	case "fit":
		if cv == nil && st.AboutMe == "" {
			writeErr(w, http.StatusBadRequest, "add your CV first (CVs & letters), so the AI has something to compare")
			return
		}
		if j.Posting == "" && j.Notes == "" {
			writeErr(w, http.StatusBadRequest, "add the posting's text first, so the AI knows what they ask for")
			return
		}
		var out struct {
			Score     int      `json:"score"`
			Verdict   string   `json:"verdict"`
			Strengths []string `json:"strengths"`
			Gaps      []string `json:"gaps"`
			Advice    string   `json:"advice"`
		}
		err := cl.JSON(ctx, aiSystem+` Compare the person with the job and answer as JSON {"score": 0-100, "verdict": "one sentence", "strengths": ["what in the CV matches what they ask, up to 5, short"], "gaps": ["what they ask that the CV does not show, up to 5, short"], "advice": "two or three sentences: whether to apply and what to stress"}. Be realistic: 80+ means a strong match on the must-haves.`,
			s.jobBrief(ctx, j, false)+"\n\n"+me+hint, &out)
		if err != nil {
			fail(err)
			return
		}
		out.Score = max(0, min(100, out.Score))
		note := strings.TrimSpace(out.Verdict)
		if len(out.Strengths) > 0 {
			note += "\n\nMatches: " + strings.Join(out.Strengths, "; ")
		}
		if len(out.Gaps) > 0 {
			note += "\n\nMissing: " + strings.Join(out.Gaps, "; ")
		}
		if out.Advice != "" {
			note += "\n\n" + strings.TrimSpace(out.Advice)
		}
		_ = s.store.SetFit(ctx, j.UserID, j.ID, out.Score, clip(note, 4000))
		writeJSON(w, http.StatusOK, map[string]any{"score": out.Score, "verdict": out.Verdict, "strengths": out.Strengths, "gaps": out.Gaps, "advice": out.Advice, "cv": docName(cv)})
	case "letter":
		prompt := s.jobBrief(ctx, j, false) + "\n\n" + me + hint + "\n\nWrite a cover letter for this job: at most 250 words, three or four short paragraphs, specific to what they ask and what the CV shows, no clichés (no \"I am writing to apply\"), no made-up facts."
		if name != "" {
			prompt += " Sign it " + name + "."
		}
		if j.ContactName != "" {
			prompt += " Address it to " + j.ContactName + "."
		}
		txt, err := cl.Complete(ctx, aiSystem, prompt, false)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt), "cv": docName(cv)})
	case "tailor":
		if cv == nil {
			writeErr(w, http.StatusBadRequest, "add your CV first (CVs & letters)")
			return
		}
		txt, err := cl.Complete(ctx, aiSystem, s.jobBrief(ctx, j, false)+"\n\n"+me+hint+
			"\n\nSuggest how to change the CV for this job, in Markdown: 1) a two-line profile at the top written for this job; 2) up to six bullet points from the CV to rewrite, each as \"Now: … → Better: …\", using the posting's words where they are true; 3) what to move up or leave out. Do not add experience the CV does not show.", false)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt), "cv": docName(cv)})
	case "prep":
		kind := "interview"
		var iv *store.Interview
		if in.InterviewID > 0 {
			if x, err := s.store.GetInterview(ctx, j.UserID, in.InterviewID); err == nil && x.JobID == j.ID {
				iv = x
				kind = firstNonEmpty(kindWords[x.Kind], "interview")
			}
		}
		var out struct {
			Prep      string     `json:"prep"`
			Questions []store.QA `json:"questions"`
			Ask       []string   `json:"ask"`
		}
		extra := ""
		if iv != nil {
			extra = "\nThe interview: " + kind + " on " + whenWords(iv) + withPrefix(iv.WithPeople) + "."
			if iv.Feedback != "" {
				extra += " Notes so far: " + clip(iv.Feedback, 800)
			}
		}
		err := cl.JSON(ctx, aiSystem+` Prepare the person for an interview. Answer as JSON {"prep": "Markdown: what to know about the role and the company from the posting, the three things to get across, and what to check before (short)", "questions": [{"q": "a likely question", "a": "notes for an answer from the CV, as short bullet points, honest"}], "ask": ["good questions to ask them"]}. Give 8 to 10 questions: some about the role's skills, some about experience (answer with STAR: situation, task, action, result), one or two awkward ones (gaps, salary, why leaving). 3 to 5 questions to ask.`,
			s.jobBrief(ctx, j, true)+extra+"\n\n"+me+hint, &out)
		if err != nil {
			fail(err)
			return
		}
		prep := strings.TrimSpace(out.Prep)
		if len(out.Ask) > 0 {
			prep += "\n\n**To ask them:**\n"
			for _, a := range out.Ask {
				prep += "\n- " + strings.TrimSpace(a)
			}
		}
		qs := []store.QA{}
		for _, q := range out.Questions {
			if strings.TrimSpace(q.Q) != "" {
				qs = append(qs, store.QA{Q: clip(strings.TrimSpace(q.Q), 1000), A: clip(strings.TrimSpace(q.A), 5000)})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"prep": prep, "questions": qs})
	case "email":
		kinds := map[string]string{
			"follow_up": "a short, polite follow-up on the application (or the last talk), asking where things stand",
			"thank_you": "a thank-you after the interview: thank them for their time, one specific thing from the talk or the role that made it interesting, and that you look forward to hearing",
			"accept":    "accepting the offer warmly and confirming the start day and the next steps",
			"decline":   "declining the offer kindly, leaving the door open",
			"withdraw":  "withdrawing from the process politely, thanking them",
			"ask":       "asking a question about the role, the process or the timeline",
		}
		what, ok := kinds[in.Kind]
		if !ok {
			in.Kind, what = "follow_up", kinds["follow_up"]
		}
		if in.Kind == "thank_you" && in.InterviewID > 0 {
			if iv, err := s.store.GetInterview(ctx, j.UserID, in.InterviewID); err == nil && iv.JobID == j.ID {
				what += fmt.Sprintf(" (the interview was %s%s", whenWords(iv), withPrefix(iv.WithPeople))
				if iv.Feedback != "" {
					what += "; my notes from it: " + clip(iv.Feedback, 800)
				}
				what += ")"
			}
		}
		var out struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		prompt := s.jobBrief(ctx, j, true) + "\nWrite an email to " + firstNonEmpty(j.ContactName, "the company") + ": " + what + "." + hint
		if name != "" {
			prompt += " Sign it " + name + "."
		}
		err := cl.JSON(ctx, aiSystem+` Answer as JSON {"subject": "...", "body": "..."}. The body is plain text with blank lines between paragraphs, at most 140 words, natural, not stiff.`, prompt, &out)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"subject": clip(strings.TrimSpace(out.Subject), 200), "body": strings.TrimSpace(out.Body), "kind": in.Kind})
	case "next":
		var out struct {
			Step string `json:"step"`
			Due  string `json:"due"`
			Why  string `json:"why"`
		}
		err := cl.JSON(ctx, aiSystem+` Answer as JSON {"step": "...", "due": "YYYY-MM-DD", "why": "..."}. step is ONE concrete next step for this job (at most 12 words, starting with a verb). due is a sensible day for it, not in the past. why is one short sentence.`,
			s.jobBrief(ctx, j, true)+hint, &out)
		if err != nil {
			fail(err)
			return
		}
		if !validDate(out.Due) || out.Due < today() {
			out.Due = store.AddDays(today(), max(st.ActionDays, 1))
		}
		writeJSON(w, http.StatusOK, map[string]any{"step": clip(strings.TrimSpace(out.Step), 300), "due": out.Due, "why": clip(out.Why, 400)})
	case "summary":
		txt, err := cl.Complete(ctx, aiSystem, "Sum up where this job stands in three to five short lines: what it is, what happened, what is open, and what to do about it.\n\n"+s.jobBrief(ctx, j, true), false)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
	case "research":
		txt, err := cl.Complete(ctx, aiSystem, s.jobBrief(ctx, j, false)+hint+
			"\n\nMake a short checklist in Markdown of what to find out about this company before applying or interviewing (from the posting: what is unclear, what to look up, what to ask people who work there) and red flags in the posting, if any. Do not state facts about the company you are not sure of.", false)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
	default:
		writeErr(w, http.StatusBadRequest, "ask for fit, letter, tailor, prep, email, next, summary or research")
	}
}

func docName(d *store.Doc) string {
	if d == nil {
		return ""
	}
	return d.Name
}

// handleAIOffers weighs the person's offers side by side, and how to
// negotiate the one they lean to.
func (s *Server) handleAIOffers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Hint string `json:"hint"`
	}
	_ = decode(r, &in)
	offers, err := s.store.ListOffers(ctx, uid)
	if err != nil {
		s.fail(w, err, "offers")
		return
	}
	var open []*store.Offer
	for _, o := range offers {
		if o.Status == "open" || o.Status == "negotiating" {
			open = append(open, o)
		}
	}
	if len(open) == 0 {
		writeErr(w, http.StatusBadRequest, "there is no offer to decide yet")
		return
	}
	cl, st, done, ok := s.aiReady(w, ctx, uid)
	if !ok {
		return
	}
	defer done()
	var b strings.Builder
	for i, o := range open {
		fmt.Fprintf(&b, "Offer %d: %s at %s (%s, %s)\n- Pay: %s; for a year about %s\n", i+1, o.JobTitle, o.Company, firstNonEmpty(o.Location, "place not said"),
			firstNonEmpty(modeWords[o.WorkMode], "work mode not said"), offerWords(o), fmtMoney(o.Yearly, o.Currency))
		if o.Equity != "" {
			fmt.Fprintf(&b, "- Equity: %s\n", o.Equity)
		}
		if o.PTODays > 0 {
			fmt.Fprintf(&b, "- Days off: %d\n", o.PTODays)
		}
		if o.Benefits != "" {
			fmt.Fprintf(&b, "- Benefits: %s\n", clip(o.Benefits, 600))
		}
		if o.StartDate != "" || o.DecideBy != "" {
			fmt.Fprintf(&b, "- Start: %s; decide by: %s\n", firstNonEmpty(o.StartDate, "?"), firstNonEmpty(o.DecideBy, "?"))
		}
		fmt.Fprintf(&b, "- How much I want it: %d of 5\n", o.Excitement)
		if o.Notes != "" {
			fmt.Fprintf(&b, "- My notes: %s\n", clip(o.Notes, 800))
		}
	}
	if st.AboutMe != "" {
		fmt.Fprintf(&b, "\nAbout me: %s\n", clip(st.AboutMe, 1500))
	}
	if h := strings.TrimSpace(in.Hint); h != "" {
		fmt.Fprintf(&b, "\nAlso: %s\n", clip(h, 1000))
	}
	txt, err := cl.Complete(ctx, aiSystem, "Help me decide between these offers (or about this one). In Markdown: a short side-by-side of what really differs (pay for a year, growth, time off, risk), what to ask before deciding, and a short, polite script to negotiate the one that looks best (what to ask for and how to word it). Do not decide for me.\n\n"+b.String(), false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
}
