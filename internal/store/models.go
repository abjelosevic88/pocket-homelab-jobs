package store

import (
	"encoding/json"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Stage kinds: what a job in the stage means.
const (
	KindSaved   = "saved"   // not applied yet
	KindApplied = "applied" // waiting to hear
	KindActive  = "active"  // talking: calls, interviews, tests
	KindOffer   = "offer"   // an offer is on the table
	KindHired   = "hired"   // took the job
	KindClosed  = "closed"  // it ended
)

// StageKinds in their order on the way.
var StageKinds = []string{KindSaved, KindApplied, KindActive, KindOffer, KindHired, KindClosed}

// Job statuses: open until hired or closed.
const (
	StatusOpen   = "open"
	StatusHired  = "hired"
	StatusClosed = "closed"
)

// StatusOf is the status of a job in a stage of this kind.
func StatusOf(kind string) string {
	switch kind {
	case KindHired:
		return StatusHired
	case KindClosed:
		return StatusClosed
	}
	return StatusOpen
}

// Rank is how far along a kind is (0 saved … 4 hired; closed = -1).
func Rank(kind string) int {
	switch kind {
	case KindApplied:
		return 1
	case KindActive:
		return 2
	case KindOffer:
		return 3
	case KindHired:
		return 4
	case KindClosed:
		return -1
	}
	return 0
}

// Stage is one step of a person's job search.
type Stage struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Position  int    `json:"position"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`

	// Filled for lists.
	Jobs int `json:"jobs"`
}

// DefaultStages is the board a person starts with.
var DefaultStages = []Stage{
	{Name: "Saved", Kind: KindSaved, Color: "#868e96"},
	{Name: "Applied", Kind: KindApplied, Color: "#3d5a98"},
	{Name: "Screening", Kind: KindActive, Color: "#0c8599"},
	{Name: "Interviewing", Kind: KindActive, Color: "#7048e8"},
	{Name: "Offer", Kind: KindOffer, Color: "#f08c00"},
	{Name: "Hired", Kind: KindHired, Color: "#2f9e44"},
	{Name: "Closed", Kind: KindClosed, Color: "#e03131"},
}

// Sources are where a job can be found.
var Sources = []string{"linkedin", "company_site", "job_board", "referral", "recruiter", "network", "other"}

// WorkModes, Employments and Periods are the choices for a job.
var (
	WorkModes   = []string{"onsite", "hybrid", "remote"}
	Employments = []string{"full_time", "part_time", "contract", "freelance", "internship"}
	Periods     = []string{"year", "month", "hour"}
)

// EndReasons are why a job closed.
var EndReasons = []string{"rejected", "withdrew", "no_answer", "filled", "other_job", "other"}

// Job is one opening on a person's search.
type Job struct {
	ID             int64    `json:"id"`
	UserID         int64    `json:"-"`
	Title          string   `json:"title"`
	Company        string   `json:"company"`
	CompanyRef     string   `json:"company_ref"`
	StageID        int64    `json:"stage_id"`
	Status         string   `json:"status"`
	Reached        int      `json:"reached"`
	Location       string   `json:"location"`
	WorkMode       string   `json:"work_mode"`
	Employment     string   `json:"employment"`
	URL            string   `json:"url"`
	Posting        string   `json:"posting"`
	SalaryMin      int64    `json:"salary_min"` // cents
	SalaryMax      int64    `json:"salary_max"`
	Currency       string   `json:"currency"`
	SalaryPeriod   string   `json:"salary_period"`
	Source         string   `json:"source"`
	Excitement     int      `json:"excitement"`
	ApplyBy        string   `json:"apply_by"`
	AppliedOn      string   `json:"applied_on"`
	RespondedAt    string   `json:"responded_at"`
	ContactName    string   `json:"contact_name"`
	ContactEmail   string   `json:"contact_email"`
	ContactPhone   string   `json:"contact_phone"`
	ContactRef     string   `json:"contact_ref"`
	Notes          string   `json:"notes"`
	CoverLetter    string   `json:"cover_letter"`
	CVID           int64    `json:"cv_id"`
	FitScore       int      `json:"fit_score"`
	FitNote        string   `json:"fit_note"`
	NextAction     string   `json:"next_action"`
	NextDue        string   `json:"next_due"`
	NextTaskRef    string   `json:"next_task_ref"`
	Tags           []string `json:"tags"`
	Starred        bool     `json:"starred"`
	Position       float64  `json:"position"`
	EndReason      string   `json:"end_reason"`
	OutcomeNote    string   `json:"outcome_note"`
	ClosedAt       string   `json:"closed_at"`
	StageChangedAt string   `json:"stage_changed_at"`
	LastActivityAt string   `json:"last_activity_at"`
	SourceApp      string   `json:"source_app"`
	SourceRef      string   `json:"source_ref"`
	ClientKey      string   `json:"client_key,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`

	// Filled for lists and pages.
	StageName     string  `json:"stage_name,omitempty"`
	StageKind     string  `json:"stage_kind,omitempty"`
	StageColor    string  `json:"stage_color,omitempty"`
	CVName        string  `json:"cv_name,omitempty"`
	MinAmount     float64 `json:"min_amount"` // salary in the currency's units
	MaxAmount     float64 `json:"max_amount"`
	Interviews    int     `json:"interviews"`
	NextInterview string  `json:"next_interview,omitempty"`
	HasOffer      bool    `json:"has_offer"`
	Overdue       bool    `json:"overdue"`
	Stale         bool    `json:"stale"` // applied and nothing heard for a while
	MadeCount     int     `json:"made_count"`
	Activities    int     `json:"activities"`
	DaysInStage   int     `json:"days_in_stage"`
}

// Label is how to call a job in one line: "Go developer at Acme".
func (j *Job) Label() string {
	switch {
	case j.Title != "" && j.Company != "":
		return j.Title + " at " + j.Company
	case j.Title != "":
		return j.Title
	}
	return j.Company
}

// Who is the person to talk to about a job.
func (j *Job) Who() string {
	if j.ContactName != "" {
		return j.ContactName
	}
	return j.ContactEmail
}

// Activity kinds a person can add by hand; the rest are written by the app.
var ActivityKinds = []string{"note", "call", "email", "step"}

// Activity is one thing that happened on a job.
type Activity struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	JobID     int64  `json:"job_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	At        string `json:"at"`
	App       string `json:"app"`
	URL       string `json:"url"`
	SourceApp string `json:"source_app"`
	CreatedAt string `json:"created_at"`
}

// Made is something a job has in another Pocket app.
type Made struct {
	ID        int64  `json:"id"`
	JobID     int64  `json:"job_id"`
	Target    string `json:"target"`
	App       string `json:"app"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checked_at"`
	CreatedAt string `json:"created_at"`
}

// InterviewKinds are the kinds of interview.
var InterviewKinds = []string{"phone", "video", "onsite", "technical", "take_home", "panel", "final", "other"}

// QA is one question for an interview and the answer to practise.
type QA struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// Interview is one call, video, visit or test.
type Interview struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	JobID      int64  `json:"job_id"`
	Kind       string `json:"kind"`
	StartsAt   string `json:"starts_at"` // local YYYY-MM-DDTHH:MM, or a day
	Minutes    int    `json:"minutes"`
	Location   string `json:"location"`
	WithPeople string `json:"with_people"`
	Prep       string `json:"prep"`
	Questions  []QA   `json:"questions"`
	Outcome    string `json:"outcome"`
	Feedback   string `json:"feedback"`
	ThanksSent bool   `json:"thanks_sent"`
	EventRef   string `json:"event_ref"`
	EventURL   string `json:"event_url"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`

	// Filled for lists.
	JobTitle   string `json:"job_title,omitempty"`
	Company    string `json:"company,omitempty"`
	JobStatus  string `json:"job_status,omitempty"`
	StageColor string `json:"stage_color,omitempty"`
	Past       bool   `json:"past"`
}

// Day is the interview's day (YYYY-MM-DD).
func (i *Interview) Day() string {
	if len(i.StartsAt) >= 10 {
		return i.StartsAt[:10]
	}
	return ""
}

// Clock is the interview's time (HH:MM), "" for a whole day.
func (i *Interview) Clock() string {
	if len(i.StartsAt) >= 16 {
		return i.StartsAt[11:16]
	}
	return ""
}

// OfferStatuses are where an offer stands.
var OfferStatuses = []string{"open", "negotiating", "accepted", "declined"}

// Offer is an offer for a job.
type Offer struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	JobID     int64  `json:"job_id"`
	Base      int64  `json:"base"` // cents per period
	Currency  string `json:"currency"`
	Period    string `json:"period"`
	Bonus     int64  `json:"bonus"` // cents a year
	Equity    string `json:"equity"`
	PTODays   int    `json:"pto_days"`
	Benefits  string `json:"benefits"`
	StartDate string `json:"start_date"`
	DecideBy  string `json:"decide_by"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Filled for pages.
	BaseAmount  float64 `json:"base_amount"`
	BonusAmount float64 `json:"bonus_amount"`
	Yearly      int64   `json:"yearly"` // base for a year + bonus, cents
	JobTitle    string  `json:"job_title,omitempty"`
	Company     string  `json:"company,omitempty"`
	Location    string  `json:"location,omitempty"`
	WorkMode    string  `json:"work_mode,omitempty"`
	Excitement  int     `json:"excitement"`
}

// DocKinds are the kinds of paper.
var DocKinds = []string{"cv", "letter", "other"}

// Doc is a CV, a cover letter or another paper.
type Doc struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	FileName  string `json:"file_name"`
	FileType  string `json:"file_type"`
	FileSize  int64  `json:"file_size"`
	DocRef    string `json:"doc_ref"`
	DocURL    string `json:"doc_url"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Filled for lists.
	Used int `json:"used"` // jobs it went to
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// The search.
	Currency   string `json:"currency"`    // new jobs start in this currency
	Period     string `json:"period"`      // and this salary period
	WeeklyGoal int    `json:"weekly_goal"` // applications a week to aim for (0 = no goal)
	StaleDays  int    `json:"stale_days"`  // applied and nothing heard for this many days: follow up
	ActionDays int    `json:"action_days"` // a new next step is due this many days later
	Home       string `json:"home"`        // where you live, for the commute and the weather

	// The other apps.
	TaskMirror       bool   `json:"task_mirror"`       // next steps also go to Pocket Tasks
	InterviewMinutes int    `json:"interview_minutes"` // how long a new interview is
	CalendarSync     bool   `json:"calendar_sync"`     // interviews go to Pocket Calendar by themselves
	LearnDeck        string `json:"learn_deck"`        // Pocket Learn deck for interview questions ("" = one per job)
	MailFrom         string `json:"mail_from"`         // which Pocket Mail account emails leave from ("" = its default)
	AIHelp           bool   `json:"ai_help"`
	AboutMe          string `json:"about_me"` // a few lines the AI may use: what you want, your strengths

	// Notifications.
	NotifyDigest bool   `json:"notify_digest"` // morning: interviews, next steps due, jobs gone quiet
	NotifyHour   int    `json:"notify_hour"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		k := strings.ToLower(t)
		if t == "" || seen[k] || len(out) >= 20 {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
