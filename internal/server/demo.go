package server

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- demo data (DEMO_DATA=true) ----
//
// A backend developer's job search after ten weeks: a CV in two versions
// and a cover letter, about thirty jobs from saved to hired-or-not, with
// postings, recruiters, interviews (some ahead), two offers to weigh — enough
// to try everything and for the screenshots.

const demoCV = `Mirela Hasić — Backend developer
Sarajevo, Bosnia and Herzegovina · mirela.hasic@example.com · github.com/mhasic

PROFILE
Backend developer with 7 years of Go and PostgreSQL, building payment and logistics systems used by hundreds of thousands of people. I like small teams, clear code and boring, reliable releases.

EXPERIENCE
Senior backend developer — Lumen Pay, Sarajevo (2022–now)
• Led the move of the card-payments service from a monolith to four Go services; checkout errors fell from 1.2% to 0.3%.
• Built the settlement pipeline (Go, PostgreSQL, Kafka) that pays out 40,000 merchants a day.
• Mentor two junior developers; run the weekly architecture review.

Backend developer — Cargoline, Tuzla (2019–2022)
• Wrote the route-planning API (Go, PostGIS) used by 300 drivers; cut planning time from hours to minutes.
• Set up CI/CD with GitHub Actions and Docker; deploys went from monthly to daily.

Junior developer — Webstudio Atlas, Sarajevo (2017–2019)
• PHP and JavaScript websites for local businesses; first Go service for their booking tool.

SKILLS
Go, PostgreSQL, Kafka, Redis, Docker, Kubernetes (basic), gRPC, REST, GitHub Actions, Linux, a little TypeScript and React.

EDUCATION
BSc Computer Science, University of Sarajevo (2017)

LANGUAGES
Bosnian (native), English (fluent, C1), German (basic, A2)`

const demoCVShort = `Mirela Hasić — Go developer (payments)
Sarajevo · mirela.hasic@example.com

7 years of Go. Payments and settlement at Lumen Pay (2022–now): four Go services, 40,000 merchants paid out daily, checkout errors down from 1.2% to 0.3%. Route planning at Cargoline (2019–2022).

Go · PostgreSQL · Kafka · gRPC · Docker · GitHub Actions`

const demoLetter = `Dear hiring team,

For the last three years I have kept money moving at Lumen Pay: I led the split of our card-payments monolith into four Go services and built the pipeline that pays out 40,000 merchants every day. Your posting asks for someone who has run payment systems in production and cares about reliability — that is the work I enjoy most.

I would bring calm releases, careful PostgreSQL design and a habit of writing things down so the next person can follow. I am also glad to mentor; two junior developers on my team now own services of their own.

I would be happy to talk about how I could help.

Kind regards,
Mirela Hasić`

type demoJob struct {
	title, company, location, mode, source string
	min, max                               int64 // euros a year
	contact, email                         string
	excite                                 int
	kind                                   string // where it ends up: saved, applied, active, offer, hired, closed
	end                                    string // closed: why
	daysAgo                                int    // saved this many days ago
	posting                                string
}

var demoJobs = []demoJob{
	{"Senior Go Engineer", "Northwind Payments", "Berlin, Germany", "remote", "linkedin", 75000, 90000, "Julia Brandt", "julia.brandt@northwind.example", 5, "offer", "", 48,
		"We move money for 12,000 online shops. You will own the ledger and settlement services (Go, PostgreSQL, Kafka) with a team of five.\n\nYou have: 5+ years of Go in production, payment or financial systems, strong SQL.\nNice: Kubernetes, on-call experience.\n\nWe offer: fully remote in Europe, 30 days off, a learning budget."},
	{"Backend Developer (Go)", "Tvornica Koda", "Sarajevo", "hybrid", "referral", 42000, 52000, "Emina Kovačević", "emina@tvornicakoda.example", 4, "offer", "", 40,
		"Product company in Sarajevo building fleet-tracking software for 30 countries. Go, gRPC, TimescaleDB. Hybrid, two days a week at the office in Marijin Dvor."},
	{"Platform Engineer", "Helio Energy", "Vienna, Austria", "hybrid", "company_site", 68000, 80000, "Lukas Steiner", "l.steiner@helio.example", 4, "active", "", 30,
		"Help us run the platform behind 200,000 smart meters. Go, Kubernetes, Terraform, AWS. German is a plus, not a must. Relocation support."},
	{"Senior Backend Engineer, Payments", "Krona Bank", "Zagreb, Croatia", "hybrid", "recruiter", 55000, 65000, "Ivan Perić", "ivan.peric@talentbridge.example", 3, "active", "", 26,
		"A digital bank for the Balkans. You will build card and instant-payment services in Go and Java. Hybrid in Zagreb, three days at the office."},
	{"Go Developer", "Sprout Logistics", "Remote (EU)", "remote", "job_board", 60000, 70000, "", "jobs@sprout.example", 4, "active", "", 22,
		"Route optimisation for same-day delivery. Go, PostGIS, Redis. Fully remote, EU time zones. Take-home task then two talks."},
	{"Staff Engineer", "Ledgerly", "Amsterdam, Netherlands", "onsite", "linkedin", 95000, 115000, "Sanne de Vries", "sanne@ledgerly.example", 3, "applied", "", 18,
		"Lead the technical direction of our accounting API. 10+ years, deep experience with distributed systems, on site in Amsterdam."},
	{"Backend Engineer", "Mostar Digital", "Mostar", "onsite", "network", 30000, 36000, "Haris Zubac", "haris@mostardigital.example", 2, "applied", "", 16,
		"Agency building web shops and booking systems. Go and PHP. Office in Mostar."},
	{"Senior Software Engineer (Go)", "Clearline", "Remote (Europe)", "remote", "linkedin", 70000, 85000, "", "careers@clearline.example", 4, "applied", "", 14,
		"Fraud detection for card payments. Go, ClickHouse, Kafka. Fully remote across Europe, quarterly meetups."},
	{"Go Engineer", "Aurora Mobility", "Munich, Germany", "hybrid", "job_board", 72000, 82000, "", "", 3, "applied", "", 12,
		"Charging software for electric buses. Go, MQTT, PostgreSQL. Hybrid in Munich, German B1 needed."},
	{"Backend Developer", "Pixelmint", "Remote", "remote", "job_board", 50000, 60000, "", "", 2, "applied", "", 11,
		"A small games studio's backend team: matchmaking and player accounts in Go."},
	{"Senior Developer", "Balkan Rail Tech", "Belgrade, Serbia", "hybrid", "recruiter", 40000, 50000, "Milica Jovanović", "milica@hirewell.example", 3, "applied", "", 9,
		"Ticketing and timetable systems for regional railways. Go and TypeScript."},
	{"Software Engineer, Infrastructure", "Datafjord", "Oslo, Norway", "remote", "company_site", 80000, 95000, "", "", 4, "applied", "", 6,
		"Keep our data platform fast and boring. Go, Kubernetes, Postgres. Remote within ±2 hours of CET."},
	{"Go Developer", "Kvadrat Software", "Banja Luka", "onsite", "network", 28000, 34000, "Dragan Savić", "dragan@kvadrat.example", 2, "applied", "", 5,
		"Accounting software for local firms. Go and Vue."},
	{"Backend Engineer, Payments", "Tidepool", "Lisbon, Portugal", "hybrid", "linkedin", 58000, 70000, "", "", 4, "saved", "", 4,
		"Payments for a travel marketplace. Go, PostgreSQL, Stripe. Hybrid in Lisbon, relocation help."},
	{"Senior Go Developer", "Greenline Health", "Remote (EU)", "remote", "linkedin", 65000, 78000, "", "", 5, "saved", "", 3,
		"Patient booking for 2,000 clinics. Go, gRPC, PostgreSQL. Fully remote."},
	{"Engineering Manager", "Lumen Pay", "Sarajevo", "hybrid", "network", 0, 0, "", "", 2, "saved", "", 3,
		"Lead two backend teams (10 people). Internal move."},
	{"Go Developer", "Orbital Labs", "Remote", "remote", "job_board", 0, 0, "", "", 3, "saved", "", 2,
		"Satellite data APIs. Go, Rust a plus."},
	{"Backend Engineer", "Vesna Foods", "Ljubljana, Slovenia", "hybrid", "company_site", 45000, 55000, "", "", 3, "saved", "", 1,
		"Ordering and logistics for 400 stores. Go, Kafka."},
	{"Senior Backend Developer", "Cityhop", "Prague, Czechia", "hybrid", "linkedin", 60000, 72000, "Tomáš Novák", "tomas@cityhop.example", 3, "closed", "rejected", 66,
		"Ride-sharing backend. Go, Kubernetes."},
	{"Go Engineer", "Payverse", "Remote", "remote", "linkedin", 65000, 75000, "", "talent@payverse.example", 4, "closed", "rejected", 62,
		"Payment orchestration. Go, PostgreSQL."},
	{"Backend Developer", "Retail Cloud", "Sarajevo", "onsite", "job_board", 30000, 38000, "", "", 2, "closed", "no_answer", 60,
		"Point-of-sale backend. Go and .NET."},
	{"Senior Software Engineer", "Nordbyte", "Stockholm, Sweden", "remote", "company_site", 75000, 88000, "Elin Berg", "elin@nordbyte.example", 4, "closed", "rejected", 55,
		"Billing platform. Go, Postgres, AWS."},
	{"Golang Developer", "QuickShip", "Remote", "remote", "job_board", 40000, 50000, "", "", 2, "closed", "withdrew", 52,
		"Courier tracking API."},
	{"Backend Engineer", "Medisoft", "Tuzla", "onsite", "network", 26000, 32000, "", "", 1, "closed", "no_answer", 50,
		"Hospital software, Go and Java."},
	{"Platform Developer", "Skyport", "Vienna, Austria", "hybrid", "linkedin", 62000, 74000, "", "", 3, "closed", "filled", 45,
		"Airport systems."},
	{"Senior Go Developer", "Moneta", "Remote (EU)", "remote", "recruiter", 70000, 80000, "Ivan Perić", "ivan.peric@talentbridge.example", 4, "closed", "rejected", 42,
		"Crypto custody backend."},
	{"Backend Developer", "Gradska Pijaca", "Sarajevo", "onsite", "network", 24000, 30000, "", "", 1, "closed", "other_job", 70,
		"A grocery delivery start-up."},
	{"Software Engineer", "BlueHarbor", "Split, Croatia", "hybrid", "job_board", 38000, 46000, "", "", 2, "closed", "no_answer", 38,
		"Booking for marinas."},
	{"Go Developer (contract)", "Freelance — Fintech UK", "Remote", "remote", "recruiter", 0, 0, "Oliver Grant", "oliver@hirefirst.example", 2, "closed", "withdrew", 35,
		"Six-month contract, day rate."},
	{"Backend Lead", "Fieldwork", "Remote", "remote", "linkedin", 85000, 95000, "", "", 4, "closed", "rejected", 28,
		"Lead a team of four. Go, Postgres, Temporal."},
}

var demoQuestions = []store.QA{
	{Q: "Tell us about a payment system you built and what went wrong in production.", A: "• Settlement pipeline at Lumen Pay\n• Duplicate payouts during a Kafka rebalance → idempotency keys\n• Result: zero duplicates since; wrote the incident review"},
	{Q: "How do you keep money correct across services?", A: "• Double-entry ledger, every move balanced\n• Idempotency keys on every write\n• Nightly reconciliation against the bank files"},
	{Q: "Why are you leaving Lumen Pay?", A: "• Proud of the work; want a bigger product and a team that runs its own platform\n• Not about the people"},
	{Q: "Describe a disagreement with a colleague and how it ended.", A: "• STAR: monolith split order\n• Wrote down both options with numbers\n• Agreed on payments first; it was the right call"},
	{Q: "What are your salary expectations?", A: "• Range from the posting; aim for the top half\n• Ask about bonus and days off before answering"},
}

// seedDemo fills the owner's account once.
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx) // with AUTH_MODE=none this runs inside the first request
	if n, _, _ := s.store.ListJobs(ctx, uid, store.JobFilter{Limit: 1}); len(n) > 0 {
		return nil
	}
	if err := s.store.EnsureStages(ctx, uid); err != nil {
		return err
	}
	rnd := rand.New(rand.NewSource(42))
	st, _ := s.store.GetSettings(ctx, uid)
	st.AboutMe = "Backend developer in Sarajevo. Looking for a senior Go role in payments or logistics, remote or hybrid, at least 55,000 EUR a year. Open to moving to Vienna or Berlin for the right team."
	st.Home = "Sarajevo"
	st.WeeklyGoal = 5
	_ = s.store.SaveUserSettings(ctx, uid, st)

	cv := &store.Doc{UserID: uid, Kind: "cv", Name: "CV — full (2026)", Body: demoCV, IsDefault: true}
	_ = s.store.CreateDoc(ctx, cv, []byte(demoCV))
	_ = s.store.SetDocFile(ctx, uid, cv.ID, []byte(demoCV), "Mirela-Hasic-CV.txt", "text/plain; charset=utf-8")
	short := &store.Doc{UserID: uid, Kind: "cv", Name: "CV — one page, payments", Body: demoCVShort}
	_ = s.store.CreateDoc(ctx, short, nil)
	letter := &store.Doc{UserID: uid, Kind: "letter", Name: "Letter — payments roles", Body: demoLetter}
	_ = s.store.CreateDoc(ctx, letter, nil)

	stageOf := map[string]int64{}
	stages, _ := s.store.ListStages(ctx, uid)
	for _, sg := range stages {
		if _, ok := stageOf[sg.Kind]; !ok {
			stageOf[sg.Kind] = sg.ID
		}
		if sg.Name == "Interviewing" {
			stageOf["interviewing"] = sg.ID
		}
	}
	now := time.Now()
	at := func(days int, hour int) string {
		d := now.AddDate(0, 0, -days)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, rnd.Intn(60), 0, 0, time.Local).UTC().Format(time.RFC3339Nano)
	}
	day := func(days int) string { return now.AddDate(0, 0, -days).Format("2006-01-02") }
	for i, dj := range demoJobs {
		saved := dj.daysAgo
		j := &store.Job{UserID: uid, Title: dj.title, Company: dj.company, Location: dj.location, WorkMode: dj.mode, Employment: "full_time",
			Source: dj.source, SalaryMin: dj.min * 100, SalaryMax: dj.max * 100, Currency: "EUR", SalaryPeriod: "year", Excitement: dj.excite,
			ContactName: dj.contact, ContactEmail: dj.email, Posting: dj.posting, Tags: []string{}, FitScore: -1,
			URL:       fmt.Sprintf("https://jobs.%s/%d", strings.ToLower(strings.ReplaceAll(strings.Fields(dj.company)[0], "—", "")), 4100+i*37),
			CreatedAt: at(saved, 9+rnd.Intn(9))}
		if strings.Contains(dj.company, "Freelance") {
			j.Employment = "contract"
		}
		if strings.Contains(dj.title, "Payments") || strings.Contains(dj.company, "Pay") || strings.Contains(dj.posting, "payment") {
			j.Tags = []string{"payments"}
		}
		if dj.mode == "remote" {
			j.Tags = append(j.Tags, "remote")
		}
		appliedAgo := max(saved-1-rnd.Intn(3), 0)
		if dj.kind != "saved" {
			j.AppliedOn = day(appliedAgo)
			j.CVID = cv.ID
			if strings.Contains(dj.posting, "ayment") || strings.Contains(dj.title, "Payments") {
				j.CVID = short.ID
			}
			j.Reached = 1
		}
		if dj.kind == "saved" && i%2 == 0 {
			j.ApplyBy = now.AddDate(0, 0, 3+i%9).Format("2006-01-02")
		}
		kindStage := dj.kind
		sid := stageOf[kindStage]
		if dj.kind == "active" && i%2 == 0 {
			sid = stageOf["interviewing"]
		}
		j.StageID = sid
		if err := s.store.CreateJob(ctx, j, &store.Activity{Kind: "created", Title: "Saved the job", At: j.CreatedAt}); err != nil {
			return err
		}
		if j.AppliedOn != "" {
			_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "applied", Title: "Applied", Body: "Sent the CV" + map[bool]string{true: " and a cover letter", false: ""}[i%3 == 0], At: at(appliedAgo, 11)})
		}
		answered := ""
		heardAgo := max(appliedAgo-3-rnd.Intn(6), 0)
		switch dj.kind {
		case "active", "offer", "hired":
			answered = at(heardAgo, 10)
			j.Reached = map[string]int{"active": 2, "offer": 3, "hired": 4}[dj.kind]
			_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "email", Title: "They wrote back", Body: "Invited to a first call with " + firstNonEmpty(dj.contact, "the recruiter") + ".", At: answered})
		case "closed":
			if dj.end == "rejected" {
				answered = at(heardAgo, 15)
			}
		}
		// interviews
		switch dj.kind {
		case "active", "offer":
			n := 1 + rnd.Intn(2)
			if dj.kind == "offer" {
				n = 3
			}
			for k := 0; k < n; k++ {
				ago := heardAgo - 2 - k*5
				kind := []string{"phone", "video", "technical", "onsite", "final"}[min(k+rnd.Intn(2), 4)]
				iv := &store.Interview{UserID: uid, JobID: j.ID, Kind: kind, Minutes: []int{30, 45, 60, 90}[rnd.Intn(4)],
					WithPeople: firstNonEmpty(dj.contact, "the hiring team"), Questions: []store.QA{}}
				if kind == "phone" {
					iv.Minutes = 30
				}
				d := now.AddDate(0, 0, -ago)
				iv.StartsAt = time.Date(d.Year(), d.Month(), d.Day(), 10+rnd.Intn(6), []int{0, 30}[rnd.Intn(2)], 0, 0, time.Local).Format("2006-01-02T15:04")
				if kind == "video" || kind == "phone" || kind == "technical" {
					iv.Location = "https://meet.example.com/" + strings.ToLower(strings.Fields(dj.company)[0]) + fmt.Sprint(k+1)
				} else {
					iv.Location = dj.location
				}
				if ago <= 0 { // still ahead: give it questions to prepare
					iv.Questions = demoQuestions[:3+rnd.Intn(3)]
					iv.Prep = "**Get across:** payments at scale, calm releases, mentoring.\n\n**Check before:** their engineering blog, the product's pricing page, who the interviewers are."
				} else {
					iv.Outcome = []string{"good", "good", "okay", "bad"}[rnd.Intn(4)]
					iv.Feedback = []string{"Friendly, lots about the ledger. They liked the idempotency story.", "Hard system-design question about payouts; did fine.", "Short call, mostly about salary and notice period.", "Live coding went slower than I wanted."}[rnd.Intn(4)]
					iv.ThanksSent = rnd.Intn(3) > 0
				}
				_ = s.store.CreateInterview(ctx, iv)
				_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "interview", Title: "Interview planned: " + kindWords[kind], Body: whenWords(iv), At: at(max(ago+3, heardAgo), 12)})
			}
		}
		// an offer
		if dj.kind == "offer" {
			o := &store.Offer{UserID: uid, JobID: j.ID, Currency: "EUR", Period: "year", Status: "open"}
			if i == 0 {
				o.Base, o.Bonus, o.PTODays, o.Equity = 84000*100, 6000*100, 30, "0.05% over four years"
				o.Benefits = "Fully remote, 1,500 EUR learning budget, new laptop, health insurance top-up."
				o.StartDate, o.DecideBy = now.AddDate(0, 1, 7).Format("2006-01-02"), now.AddDate(0, 0, 4).Format("2006-01-02")
				o.Notes = "Team of five. On-call one week in six."
			} else {
				o.Base, o.Period, o.PTODays = 4100*100, "month", 25
				o.Benefits = "Hybrid in Sarajevo, private health insurance, lunch at the office."
				o.StartDate, o.DecideBy, o.Status = now.AddDate(0, 0, 30).Format("2006-01-02"), now.AddDate(0, 0, 6).Format("2006-01-02"), "negotiating"
				o.Notes = "Asked for 4,500 a month; they will answer by Friday."
			}
			_ = s.store.SaveOffer(ctx, o)
			_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "offer", Title: "Offer: " + offerWords(o), At: at(2+i, 16)})
		}
		// closed ones
		if dj.kind == "closed" {
			j.EndReason = dj.end
			_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "closed", Title: endWords[dj.end], At: at(max(heardAgo-1, 0), 14)})
		}
		// next steps on the open ones
		if j.Status == store.StatusOpen || dj.kind != "closed" {
			switch dj.kind {
			case "applied":
				if i%3 == 0 {
					j.NextAction, j.NextDue = "Follow up with "+firstNonEmpty(dj.contact, "the recruiter"), now.AddDate(0, 0, -1+rnd.Intn(4)).Format("2006-01-02")
				}
			case "active":
				j.NextAction, j.NextDue = []string{"Send the take-home task", "Prepare the system-design talk", "Ask about the salary range"}[i%3], now.AddDate(0, 0, rnd.Intn(3)).Format("2006-01-02")
			case "offer":
				j.NextAction, j.NextDue = "Decide on the offer", now.AddDate(0, 0, 3).Format("2006-01-02")
			case "saved":
				if i%2 == 1 {
					j.NextAction, j.NextDue = "Tailor the CV and apply", now.AddDate(0, 0, rnd.Intn(5)).Format("2006-01-02")
				}
			}
		}
		if dj.kind != "saved" && i < 14 {
			j.FitScore = []int{88, 74, 81, 69, 85, 62, 55, 79, 66, 58, 64, 77, 49, 83}[i]
			j.FitNote = "Strong on Go and payments; the posting also asks for " + []string{"Kubernetes in production", "Java", "German", "Terraform", "on-call experience"}[i%5] + "."
		}
		if dj.contact == "" && dj.kind == "applied" && i%2 == 0 {
			j.Notes = "Found through a friend's post; the team lead is active on GitHub."
		}
		j.RespondedAt = answered
		closed := ""
		if dj.kind == "closed" {
			closed = at(max(heardAgo-1, 0), 14)
		}
		_ = s.store.UpdateJob(ctx, j)
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE jobs SET responded_at = ?, reached = ?, closed_at = CASE WHEN ? <> '' THEN ? ELSE closed_at END,
			stage_changed_at = ? WHERE user_id = ? AND id = ?`, answered, j.Reached, closed, closed, at(max(heardAgo, 0), 10), uid, j.ID)
	}
	// interviews ahead: later today and in a few days, on jobs that have none coming
	list, _, _ := s.store.ListJobs(ctx, uid, store.JobFilter{Kind: store.KindActive, Limit: 10})
	added := 0
	for _, j := range list {
		if added == 2 || j.NextInterview != "" {
			continue
		}
		d := now.AddDate(0, 0, added*3)
		h := min(max(now.Hour()+2, 10), 17)
		if added == 1 {
			h = 11
		}
		if h <= now.Hour() {
			d = d.AddDate(0, 0, 1) // late in the day: tomorrow
		}
		iv := &store.Interview{UserID: uid, JobID: j.ID, Kind: []string{"technical", "onsite"}[added], Minutes: 60, WithPeople: firstNonEmpty(j.ContactName, "two engineers"),
			StartsAt: time.Date(d.Year(), d.Month(), d.Day(), h, 0, 0, 0, time.Local).Format("2006-01-02T15:04"), Questions: demoQuestions[:4],
			Prep: "**Get across:** payments at scale, calm releases, mentoring.\n\n**Check before:** the product, the interviewers, the commute."}
		if added == 0 {
			iv.Location = "https://meet.example.com/" + strings.ToLower(strings.Fields(j.Company)[0])
		} else {
			iv.Location = j.Location
		}
		_ = s.store.CreateInterview(ctx, iv)
		added++
	}
	s.log.Info("demo data added", "jobs", len(demoJobs))
	return nil
}
