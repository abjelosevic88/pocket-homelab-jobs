package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

func (a *app) raw(tok, method, path, ctype string, body []byte) (int, []byte, http.Header) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b, res.Header
}

func stageByKind(a *app, tok, kind string) map[string]any {
	for _, s := range a.list(tok, "/api/v1/stages") {
		if st := s.(map[string]any); st["kind"] == kind {
			return st
		}
	}
	a.t.Fatalf("no %s stage", kind)
	return nil
}

func TestJobsArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Go developer", "company": "Acme", "min_amount": 50000, "max_amount": 60000,
		"contact_name": "Ana Marić", "contact_email": "Ana@Acme.example", "work_mode": "Hybrid", "url": "jobs.acme.example/12"})
	if j["contact_email"] != "ana@acme.example" || num(j, "salary_min") != 5000000 || j["status"] != "open" || j["work_mode"] != "hybrid" ||
		j["url"] != "https://jobs.acme.example/12" || j["stage_kind"] != "saved" || j["applied_on"] != "" {
		t.Fatalf("job as stored: %v", j)
	}
	path := "/api/v1/jobs/" + id(j)
	iv := a.ok(a.alice, "POST", path+"/interviews", map[string]any{"kind": "video", "date": "2099-01-02", "time": "10:30", "calendar": false})["interview"].(map[string]any)
	a.ok(a.alice, "PUT", path+"/offer", map[string]any{"base_amount": 62000, "bonus_amount": 3000})
	doc := a.ok(a.alice, "POST", "/api/v1/docs", map[string]any{"kind": "cv", "name": "My CV", "body": "Go, Postgres"})
	if doc["is_default"] != true {
		t.Fatalf("the first CV is the main one: %v", doc)
	}
	if code, _, _ := a.raw(a.alice, "PUT", "/api/v1/docs/"+id(doc)+"/file?name=cv.pdf", "application/pdf", []byte("%PDF-1.4 hello")); code != 200 {
		t.Fatalf("upload: %d", code)
	}
	// Bob sees none of it, and cannot touch it
	for _, c := range [][2]string{{"GET", path}, {"PUT", path}, {"DELETE", path}, {"POST", path + "/activities"}, {"POST", path + "/move"},
		{"POST", path + "/make"}, {"POST", path + "/interviews"}, {"PUT", path + "/offer"}, {"GET", path + "/elsewhere"},
		{"GET", "/api/v1/interviews/" + id(iv)}, {"PUT", "/api/v1/interviews/" + id(iv)}, {"DELETE", "/api/v1/interviews/" + id(iv)},
		{"GET", "/api/v1/interviews/" + id(iv) + "/interview.ics"}, {"GET", "/api/v1/docs/" + id(doc)}, {"PUT", "/api/v1/docs/" + id(doc)},
		{"GET", "/api/v1/docs/" + id(doc) + "/file"}, {"DELETE", "/api/v1/docs/" + id(doc)}} {
		code, _, _ := a.call(a.bob, c[0], c[1], map[string]any{"title": "mine", "stage_id": 1, "target": "note", "date": "2099-01-01", "name": "x"})
		if code != 404 {
			t.Fatalf("Bob %s %s → %d, want 404", c[0], c[1], code)
		}
	}
	if l := a.ok(a.bob, "GET", "/api/v1/jobs", nil); num(l, "total") != 0 {
		t.Fatalf("Bob sees Alice's jobs: %v", l)
	}
	for _, p := range []string{"/api/v1/interviews", "/api/v1/offers", "/api/v1/docs", "/api/v1/companies"} {
		if l := a.list(a.bob, p); len(l) != 0 {
			t.Fatalf("Bob sees Alice's %s: %v", p, l)
		}
	}
	if l := a.ok(a.bob, "GET", "/api/v1/lookup?email=ana@acme.example", nil); len(l["jobs"].([]any)) != 0 {
		t.Fatalf("Bob's lookup finds Alice's job")
	}
	// Bob's stages and CVs are his own
	first := stageByKind(a, a.alice, "applied")
	a.expect(404, a.bob, "PUT", "/api/v1/stages/"+id(first), map[string]any{"name": "x"})
	a.expect(400, a.bob, "POST", "/api/v1/jobs", map[string]any{"title": "x", "stage_id": num(first, "id")})
	bj := a.ok(a.bob, "POST", "/api/v1/jobs", map[string]any{"title": "x", "cv_id": num(doc, "id")})
	if num(bj, "cv_id") != 0 {
		t.Fatalf("Bob could use Alice's CV: %v", bj)
	}
	a.expect(400, a.bob, "PUT", "/api/v1/jobs/"+id(bj), map[string]any{"cv_id": num(doc, "id")})
	act := a.ok(a.alice, "POST", path+"/activities", map[string]any{"kind": "call", "title": "Called Ana"})
	a.expect(404, a.bob, "PUT", "/api/v1/activities/"+id(act), map[string]any{"title": "x"})
	a.expect(404, a.bob, "DELETE", "/api/v1/activities/"+id(act), nil)
	// Alice sees it all
	got := a.ok(a.alice, "GET", path, nil)
	if len(got["interviews"].([]any)) != 1 || got["offer"] == nil || len(got["activities"].([]any)) < 3 {
		t.Fatalf("Alice's job: %v", got)
	}
	code, b, h := a.raw(a.alice, "GET", "/api/v1/docs/"+id(doc)+"/file", "", nil)
	if code != 200 || string(b) != "%PDF-1.4 hello" || h.Get("Content-Type") != "application/pdf" {
		t.Fatalf("download: %d %q %v", code, b, h)
	}
}

func TestStagesApplyAnswerHireClose(t *testing.T) {
	a := newApp(t, nil)
	stages := a.list(a.alice, "/api/v1/stages")
	if len(stages) != len(store.DefaultStages) {
		t.Fatalf("default stages: %d", len(stages))
	}
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Backend", "company": "Beta", "next_action": "Tailor the CV", "next_due": "2026-01-01"})
	if j["overdue"] != true || j["stage_kind"] != "saved" {
		t.Fatalf("new job: %v", j)
	}
	path := "/api/v1/jobs/" + id(j)
	m := a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(stageByKind(a, a.alice, "applied"), "id")})
	if m["applied_on"] != today() || num(m, "reached") != 1 || m["responded_at"] != "" {
		t.Fatalf("applied: %v", m)
	}
	m = a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(stageByKind(a, a.alice, "active"), "id")})
	if m["responded_at"] == "" || num(m, "reached") != 2 {
		t.Fatalf("they answered: %v", m)
	}
	a.expect(400, a.alice, "POST", path+"/move", map[string]any{"stage_id": num(stageByKind(a, a.alice, "closed"), "id"), "end_reason": "bored"})
	m = a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(stageByKind(a, a.alice, "closed"), "id"), "end_reason": "rejected", "note": "Went with someone senior"})
	if m["status"] != "closed" || m["end_reason"] != "rejected" || m["closed_at"] == "" || m["applied_on"] != today() {
		t.Fatalf("closed: %v", m)
	}
	m = a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(stageByKind(a, a.alice, "active"), "id")})
	if m["status"] != "open" || m["closed_at"] != "" || m["end_reason"] != "" {
		t.Fatalf("opened again: %v", m)
	}
	// a job saved straight into Applied counts as applied today
	j2 := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Ops", "stage": "applied"})
	if j2["applied_on"] != today() || j2["stage_kind"] != "applied" {
		t.Fatalf("applied on create: %v", j2)
	}
	sum := a.ok(a.alice, "GET", "/api/v1/summary", nil)
	if num(sum, "applied_week") != 2 || num(sum, "weekly_goal") != 5 {
		t.Fatalf("summary: %v", sum)
	}
	// a stage needs a kind from the list; deleting one with jobs asks where they go
	a.expect(400, a.alice, "POST", "/api/v1/stages", map[string]any{"name": "Waiting", "kind": "won"})
	extra := a.ok(a.alice, "POST", "/api/v1/stages", map[string]any{"name": "Take-home", "kind": "active"})
	a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(extra, "id")})
	a.expect(409, a.alice, "DELETE", "/api/v1/stages/"+id(extra), nil)
	a.expect(204, a.alice, "DELETE", "/api/v1/stages/"+id(extra)+"?move_to="+id(stageByKind(a, a.alice, "active")), nil)
}

func TestInterviewsAndOffers(t *testing.T) {
	a := newApp(t, nil)
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Platform", "company": "Gamma", "stage": "applied", "location": "Vienna"})
	path := "/api/v1/jobs/" + id(j)
	a.expect(400, a.alice, "POST", path+"/interviews", map[string]any{"kind": "video"})
	a.expect(400, a.alice, "POST", path+"/interviews", map[string]any{"kind": "chat", "date": "2099-01-02"})
	res := a.ok(a.alice, "POST", path+"/interviews", map[string]any{"kind": "onsite", "date": "2099-01-02", "time": "14:00", "minutes": 90,
		"location": "Ringstraße 1, Vienna", "with_people": "Lukas", "questions": []map[string]string{{"q": "Why us?", "a": "Smart meters"}, {"q": " "}}})
	iv := res["interview"].(map[string]any)
	if iv["starts_at"] != "2099-01-02T14:00" || len(iv["questions"].([]any)) != 1 || iv["past"] != false {
		t.Fatalf("interview: %v", iv)
	}
	// an interview means they answered
	if got := a.ok(a.alice, "GET", path, nil)["job"].(map[string]any); got["stage_kind"] != "active" || got["next_interview"] != "2099-01-02T14:00" {
		t.Fatalf("after an interview the job moves on: %v", got)
	}
	up := a.ok(a.alice, "PUT", "/api/v1/interviews/"+id(iv), map[string]any{"time": "15:30", "outcome": "good", "feedback": "Nice team"})["interview"].(map[string]any)
	if up["starts_at"] != "2099-01-02T15:30" || up["outcome"] != "good" {
		t.Fatalf("moved: %v", up)
	}
	code, ics, _ := a.raw(a.alice, "GET", "/api/v1/interviews/"+id(iv)+"/interview.ics", "", nil)
	if code != 200 || !bytes.Contains(ics, []byte("BEGIN:VEVENT")) || !bytes.Contains(ics, []byte("On-site interview")) || !bytes.Contains(ics, []byte("LOCATION:Ringstraße 1\\, Vienna")) {
		t.Fatalf("ics: %d %s", code, ics)
	}
	if l := a.list(a.alice, "/api/v1/interviews?when=upcoming"); len(l) != 1 {
		t.Fatalf("upcoming: %v", l)
	}
	if l := a.list(a.alice, "/api/v1/interviews?when=past"); len(l) != 0 {
		t.Fatalf("past: %v", l)
	}
	// an offer moves the job to Offer; accepting it hires
	o := a.ok(a.alice, "PUT", path+"/offer", map[string]any{"base_amount": 5000, "period": "month", "bonus_amount": 4000, "currency": "eur", "decide_by": "2099-01-10"})
	if num(o, "yearly") != (5000*12+4000)*100 || o["status"] != "open" || o["currency"] != "EUR" {
		t.Fatalf("offer: %v", o)
	}
	if got := a.ok(a.alice, "GET", path, nil)["job"].(map[string]any); got["stage_kind"] != "offer" || got["has_offer"] != true {
		t.Fatalf("after an offer: %v", got)
	}
	a.expect(400, a.alice, "PUT", path+"/offer", map[string]any{"status": "maybe"})
	a.ok(a.alice, "PUT", path+"/offer", map[string]any{"status": "accepted"})
	got := a.ok(a.alice, "GET", path, nil)["job"].(map[string]any)
	if got["status"] != "hired" || num(got, "reached") != 4 {
		t.Fatalf("accepted: %v", got)
	}
	if l := a.list(a.alice, "/api/v1/offers"); len(l) != 1 {
		t.Fatalf("offers: %v", l)
	}
	a.expect(204, a.alice, "DELETE", "/api/v1/interviews/"+id(iv), nil)
}

func TestAppsWithoutCoreAreExplained(t *testing.T) {
	a := newApp(t, nil)
	res := a.ok(a.alice, "GET", "/api/v1/apps", nil)
	apps := res["apps"].([]any)
	if len(apps) != len(pocketApps) {
		t.Fatalf("apps: %d", len(apps))
	}
	for _, x := range apps {
		st := x.(map[string]any)
		if st["via"] != "" || st["state"] == "on" || st["fix"] == "" {
			t.Fatalf("an app without Core or a token should say what to do: %v", st)
		}
	}
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Audit", "contact_email": "x@y.example", "company": "Ypsilon", "location": "Sarajevo"})
	path := "/api/v1/jobs/" + id(j)
	for _, target := range []string{"contact", "task", "note", "email"} {
		m := a.expect(400, a.alice, "POST", path+"/make", map[string]any{"target": target, "body": "Hello", "subject": "Hi"})
		if !strings.Contains(m["error"].(string), "Pocket") {
			t.Fatalf("%s: %v", target, m)
		}
	}
	a.expect(400, a.alice, "POST", path+"/make", map[string]any{"target": "invoice"})
	_, _, l := a.call(a.alice, "GET", path+"/elsewhere", nil)
	if len(l) != 2 {
		t.Fatalf("elsewhere: %v", l)
	}
	for _, x := range l {
		if st := x.(map[string]any); st["items"] == nil || st["state"] == "ok" {
			t.Fatalf("each part greys out on its own, never null: %v", x)
		}
	}
	iv := a.ok(a.alice, "POST", path+"/interviews", map[string]any{"kind": "onsite", "date": time.Now().AddDate(0, 0, 2).Format("2006-01-02"), "time": "10:00",
		"questions": []map[string]string{{"q": "Why?"}}})["interview"].(map[string]any)
	if iv["event_ref"] != "" {
		t.Fatalf("no calendar to put it in: %v", iv)
	}
	ivp := "/api/v1/interviews/" + id(iv)
	for _, p := range []string{ivp + "/clashes", ivp + "/weather"} {
		if m := a.ok(a.alice, "GET", p, nil); m["state"] == "ok" {
			t.Fatalf("%s answered without the app: %v", p, m)
		}
	}
	a.expect(400, a.alice, "POST", ivp+"/cards", nil)
	a.expect(400, a.alice, "POST", ivp+"/note", nil)
	a.expect(400, a.alice, "POST", ivp+"/calendar", nil)
	if m := a.ok(a.alice, "GET", "/api/v1/docs/from-documents", nil); m["state"] == "ok" || m["fix"] == "" {
		t.Fatalf("documents without Core: %v", m)
	}
	// no AI: plain words, not a crash
	m := a.expect(503, a.alice, "POST", path+"/ai", map[string]any{"want": "fit"})
	if m["error"] == "" {
		t.Fatalf("ai: %v", m)
	}
}

func TestExportImportIsIdempotent(t *testing.T) {
	a := newApp(t, nil)
	cv := a.ok(a.alice, "POST", "/api/v1/docs", map[string]any{"kind": "cv", "name": "CV", "body": "Go"})
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "SRE", "company": "Delta", "cv_id": num(cv, "id"), "tags": []string{"remote", "#remote", "go"}, "stage": "applied"})
	if len(j["tags"].([]any)) != 2 || j["cv_name"] != "CV" {
		t.Fatalf("job: %v", j)
	}
	path := "/api/v1/jobs/" + id(j)
	a.ok(a.alice, "POST", path+"/activities", map[string]any{"title": "Met them"})
	a.ok(a.alice, "POST", path+"/interviews", map[string]any{"date": "2099-02-03", "kind": "phone"})
	a.ok(a.alice, "PUT", path+"/offer", map[string]any{"base_amount": 70000})
	doc, _ := json.Marshal(a.s.export(context.Background(), a.ids["Alice"]))
	post := func(tok string, body []byte) map[string]any {
		code, b, _ := a.raw(tok, "POST", "/api/v1/import", "", body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if code != 200 {
			t.Fatalf("import → %d %v", code, m)
		}
		return m
	}
	if m := post(a.bob, doc); num(m, "jobs") != 1 || num(m, "docs") != 1 {
		t.Fatalf("first import: %v", m)
	}
	if m := post(a.bob, doc); num(m, "jobs") != 0 || num(m, "skipped") != 1 || num(m, "docs") != 0 {
		t.Fatalf("second import: %v", m)
	}
	bobs := a.ok(a.bob, "GET", "/api/v1/jobs", nil)["items"].([]any)
	if len(bobs) != 1 {
		t.Fatalf("Bob's jobs: %v", bobs)
	}
	full := a.ok(a.bob, "GET", "/api/v1/jobs/"+id(bobs[0].(map[string]any)), nil)
	bj := full["job"].(map[string]any)
	if len(full["interviews"].([]any)) != 1 || full["offer"] == nil || bj["cv_name"] != "CV" || bj["applied_on"] == "" {
		t.Fatalf("imported: %v", full)
	}
	// a Teal-like CSV, twice
	csv := []byte("Job Position,Company,Status,Min Salary,Max Salary,Location,URL,Excitement,Date Applied,Notes\n" +
		"Go Developer,Echo,Interviewing,\"50,000\",\"60,000\",Remote,https://echo.example/1,4,2026-10-01,Nice\n" +
		"Data Engineer,Foxtrot,Bookmarked,,,\"Berlin, Germany\",,3,,\n" +
		"Backend,Golf,Rejected,,,Hybrid - Sarajevo,,2,04.09.2026,\n")
	if m := post(a.alice, csv); num(m, "jobs") != 3 {
		t.Fatalf("csv: %v", m)
	}
	if m := post(a.alice, csv); num(m, "jobs") != 0 {
		t.Fatalf("csv again: %v", m)
	}
	l := a.ok(a.alice, "GET", "/api/v1/jobs?q=Echo", nil)["items"].([]any)
	e := l[0].(map[string]any)
	if len(l) != 1 || num(e, "salary_max") != 6000000 || e["stage_kind"] != "active" || e["work_mode"] != "remote" || num(e, "excitement") != 4 || e["applied_on"] != "2026-10-01" {
		t.Fatalf("csv job: %v", l)
	}
	g := a.ok(a.alice, "GET", "/api/v1/jobs?q=Golf", nil)["items"].([]any)[0].(map[string]any)
	if g["status"] != "closed" || g["applied_on"] != "2026-09-04" || g["work_mode"] != "hybrid" {
		t.Fatalf("csv closed: %v", g)
	}
}

func TestSettingsArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "km", "weekly_goal": 8, "about_me": " Go developer ", "period": "month"})
	al := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	if al["currency"] != "BAM" || num(al, "weekly_goal") != 8 || al["about_me"] != "Go developer" || al["period"] != "month" {
		t.Fatalf("alice: %v", al)
	}
	bo := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if bo["currency"] != "EUR" || num(bo, "weekly_goal") != 5 || bo["about_me"] != "" {
		t.Fatalf("bob got alice's settings: %v", bo)
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "euro"})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"period": "week"})
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "x"})
	if j["currency"] != "BAM" || j["salary_period"] != "month" {
		t.Fatalf("new jobs take the person's currency and period: %v", j)
	}
}

func TestLinkScopes(t *testing.T) {
	type c struct {
		method, path string
		scopes       []string
		want         bool
	}
	for _, tc := range []c{
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/jobs", []string{"jobs:read"}, true},
		{"GET", "/api/v1/jobs/12", []string{"jobs:read"}, true},
		{"GET", "/api/v1/jobs/12/interviews", []string{"jobs:read"}, true},
		{"GET", "/api/v1/interviews", []string{"jobs:read"}, true},
		{"GET", "/api/v1/offers", []string{"jobs:read"}, true},
		{"GET", "/api/v1/lookup", []string{"jobs:read"}, true},
		{"GET", "/api/v1/day/2026-11-07", []string{"jobs:read"}, true},
		{"GET", "/api/v1/jobs", []string{"jobs:write"}, false},
		{"POST", "/api/v1/jobs", []string{"jobs:read"}, false},
		{"POST", "/api/v1/jobs", []string{"jobs:write"}, true},
		{"POST", "/api/v1/jobs/3/activities", []string{"jobs:write"}, true},
		{"POST", "/api/v1/jobs/3/interviews", []string{"jobs:write"}, true},
		{"POST", "/api/v1/jobs/3/move", []string{"jobs:write"}, false},
		{"POST", "/api/v1/jobs/3/make", []string{"jobs:read", "jobs:write"}, false},
		{"POST", "/api/v1/jobs/3/ai", []string{"jobs:read", "jobs:write"}, false},
		{"PUT", "/api/v1/jobs/3/offer", []string{"jobs:write"}, false},
		{"DELETE", "/api/v1/jobs/3", []string{"jobs:write"}, false},
		{"GET", "/api/v1/docs", []string{"jobs:read", "jobs:write"}, false},
		{"GET", "/api/v1/settings", []string{"jobs:read", "jobs:write"}, false},
		{"GET", "/api/v1/export.json", []string{"jobs:read"}, false},
	} {
		r, _ := http.NewRequest(tc.method, tc.path, nil)
		if got := linkAllows(tc.scopes, r); got != tc.want {
			t.Errorf("%s %s %v: got %v", tc.method, tc.path, tc.scopes, got)
		}
	}
	for _, o := range linkOffers {
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,60}$`).MatchString(o.Scope) {
			t.Errorf("bad scope %q", o.Scope)
		}
	}
}

func TestSourceRefNeverTwice(t *testing.T) {
	a := newApp(t, nil)
	first := a.expect(201, a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "From an email", "source_ref": "mail:msg:7"})
	again := a.expect(200, a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "From an email", "source_ref": "mail:msg:7"})
	if id(first) != id(again) {
		t.Fatalf("the same job was added twice")
	}
	b := a.expect(201, a.bob, "POST", "/api/v1/jobs", map[string]any{"title": "From an email", "source_ref": "mail:msg:7"})
	if id(b) == id(first) {
		t.Fatalf("Bob got Alice's job")
	}
	// the same posting link, when asked for once, gives the saved job
	x := a.expect(201, a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Linked", "url": "https://jobs.example/9"})
	y := a.expect(200, a.alice, "POST", "/api/v1/jobs?once=1", map[string]any{"title": "Linked", "url": "https://jobs.example/9"})
	if id(x) != id(y) {
		t.Fatalf("the same posting was saved twice")
	}
}

func TestParsing(t *testing.T) {
	for in, want := range map[string]float64{"€2,000 – €5,000": 2000, "5.000 €": 5000, "about 3000": 3000, "12,50": 12.5, "1.250,75 KM": 1250.75, "Not sure yet": 0, "": 0} {
		if got := parseAmount(in); got != want {
			t.Errorf("parseAmount(%q) = %v, want %v", in, got, want)
		}
	}
	if fmtMoney(450000, "EUR") != "4,500 EUR" || fmtMoney(123456789, "BAM") != "1,234,567.89 BAM" {
		t.Errorf("fmtMoney: %s %s", fmtMoney(450000, "EUR"), fmtMoney(123456789, "BAM"))
	}
	page := `<html><head><title>Jobs | Board</title><script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"Organization","name":"x"},
		{"@type":"JobPosting","title":"Senior Go Engineer","hiringOrganization":{"@type":"Organization","name":"Northwind &amp; Co"},
		"jobLocation":{"@type":"Place","address":{"addressLocality":"Berlin","addressCountry":"DE"}},"jobLocationType":"TELECOMMUTE",
		"employmentType":"FULL_TIME","validThrough":"2099-03-01T00:00","baseSalary":{"@type":"MonetaryAmount","currency":"eur",
		"value":{"@type":"QuantitativeValue","minValue":75000,"maxValue":"90.000","unitText":"YEAR"}},"description":"<p>Own the <b>ledger</b>.</p><ul><li>Go</li><li>SQL</li></ul>"}]}</script></head><body>x</body></html>`
	p := parsePosting(page)
	if !p.Structured || p.Title != "Senior Go Engineer" || p.Company != "Northwind & Co" || p.Location != "Berlin, DE" || p.WorkMode != "remote" ||
		p.Employment != "full_time" || p.ApplyBy != "2099-03-01" || p.SalaryMin != 75000 || p.SalaryMax != 90000 || p.Currency != "EUR" || p.Period != "year" ||
		!strings.Contains(p.Text, "Own the ledger.") || !strings.Contains(p.Text, "• Go") {
		t.Fatalf("posting: %+v", p)
	}
	q := parsePosting(`<html><head><title>Backend developer – Acme</title><style>x{}</style></head><body><nav>menu</nav><h1>Backend developer</h1><p>We want Go.</p></body></html>`)
	if q.Structured || q.Title != "Backend developer – Acme" || strings.Contains(q.Text, "menu") || !strings.Contains(q.Text, "We want Go.") {
		t.Fatalf("plain page: %+v", q)
	}
	if k := kindFromWord("Interviewing"); k != "active" {
		t.Errorf("kindFromWord: %s", k)
	}
	if k := kindFromWord("Bookmarked"); k != "saved" {
		t.Errorf("kindFromWord: %s", k)
	}
	a := newApp(t, nil)
	for _, u := range []string{"http://127.0.0.1:1/x", "http://localhost:1/", "http://10.0.0.5/", "http://192.168.1.1/", "http://100.111.226.27/", "file:///etc/passwd"} {
		m := a.expect(400, a.alice, "POST", "/api/v1/postings/read", map[string]any{"url": u})
		if m["error"] == "" {
			t.Fatalf("%s: %v", u, m)
		}
	}
	// pasted text alone works without AI
	m := a.ok(a.alice, "POST", "/api/v1/postings/read", map[string]any{"text": "Go developer\n\nWe are hiring."})
	if m["posting"].(map[string]any)["title"] != "Go developer" || m["ai"] != false {
		t.Fatalf("pasted: %v", m)
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	l := a.ok(a.alice, "GET", "/api/v1/jobs?limit=500", nil)
	if num(l, "total") < 28 {
		t.Fatalf("demo jobs: %v", l["total"])
	}
	td := a.ok(a.alice, "GET", "/api/v1/today", nil)
	lists := td["lists"].(map[string]any)
	if len(td["interviews"].([]any)) == 0 || len(td["offers"].([]any)) != 2 || len(lists["apply"].([]any)) == 0 || len(td["week"].([]any)) != 7 {
		t.Fatalf("today: %v", td)
	}
	sum := td["summary"].(map[string]any)
	if num(sum, "applied_total") < 20 || num(sum, "response_rate") == 0 || sum["next_interview"] == nil {
		t.Fatalf("summary: %v", sum)
	}
	rep := a.ok(a.alice, "GET", "/api/v1/reports", nil)
	if len(rep["weeks"].([]any)) != 12 || len(rep["funnel"].([]any)) != 5 || len(rep["sources"].([]any)) < 3 {
		t.Fatalf("reports: %v", rep)
	}
	if l := a.list(a.alice, "/api/v1/companies"); len(l) < 25 {
		t.Fatalf("companies: %d", len(l))
	}
	if l := a.list(a.alice, "/api/v1/docs"); len(l) != 3 {
		t.Fatalf("docs: %d", len(l))
	}
	a.ok(a.alice, "GET", "/api/v1/day/"+td["date"].(string), nil)
	// twice is once
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	if l := a.ok(a.alice, "GET", "/api/v1/jobs?limit=500", nil); num(l, "total") != float64(len(demoJobs)) {
		t.Fatalf("demo twice: %v", l["total"])
	}
}
