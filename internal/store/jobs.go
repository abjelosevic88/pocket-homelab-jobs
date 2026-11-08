package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ---- jobs ----

const jobCols = `j.id, j.user_id, j.title, j.company, j.company_ref, j.stage_id, j.status, j.reached, j.location, j.work_mode, j.employment,
	j.url, j.posting, j.salary_min, j.salary_max, j.currency, j.salary_period, j.source, j.excitement, j.apply_by, j.applied_on, j.responded_at,
	j.contact_name, j.contact_email, j.contact_phone, j.contact_ref, j.notes, j.cover_letter, COALESCE(j.cv_id, 0), j.fit_score, j.fit_note,
	j.next_action, j.next_due, j.next_task_ref, j.tags, j.starred, j.position, j.end_reason, j.outcome_note, j.closed_at,
	j.stage_changed_at, j.last_activity_at, j.source_app, j.source_ref, j.client_key, j.created_at, j.updated_at,
	COALESCE(s.name, ''), COALESCE(s.kind, ''), COALESCE(s.color, ''), COALESCE(d.name, ''),
	(SELECT COUNT(1) FROM interviews i WHERE i.job_id = j.id),
	COALESCE((SELECT MIN(i.starts_at) FROM interviews i WHERE i.job_id = j.id AND i.starts_at >= ?), ''),
	(SELECT COUNT(1) FROM offers o WHERE o.job_id = j.id),
	(SELECT COUNT(1) FROM made m WHERE m.job_id = j.id AND m.error = ''),
	(SELECT COUNT(1) FROM activities a WHERE a.job_id = j.id)`

const jobFrom = ` FROM jobs j LEFT JOIN stages s ON s.id = j.stage_id LEFT JOIN docs d ON d.id = j.cv_id `

// nowLocal is the local "YYYY-MM-DDTHH:MM" interviews are compared with.
func nowLocal() string { return time.Now().Format("2006-01-02T15:04") }

func scanJob(sc interface{ Scan(...any) error }) (*Job, error) {
	var j Job
	var tags string
	var starred, offers int
	if err := sc.Scan(&j.ID, &j.UserID, &j.Title, &j.Company, &j.CompanyRef, &j.StageID, &j.Status, &j.Reached, &j.Location, &j.WorkMode, &j.Employment,
		&j.URL, &j.Posting, &j.SalaryMin, &j.SalaryMax, &j.Currency, &j.SalaryPeriod, &j.Source, &j.Excitement, &j.ApplyBy, &j.AppliedOn, &j.RespondedAt,
		&j.ContactName, &j.ContactEmail, &j.ContactPhone, &j.ContactRef, &j.Notes, &j.CoverLetter, &j.CVID, &j.FitScore, &j.FitNote,
		&j.NextAction, &j.NextDue, &j.NextTaskRef, &tags, &starred, &j.Position, &j.EndReason, &j.OutcomeNote, &j.ClosedAt,
		&j.StageChangedAt, &j.LastActivityAt, &j.SourceApp, &j.SourceRef, &j.ClientKey, &j.CreatedAt, &j.UpdatedAt,
		&j.StageName, &j.StageKind, &j.StageColor, &j.CVName, &j.Interviews, &j.NextInterview, &offers, &j.MadeCount, &j.Activities); err != nil {
		return nil, err
	}
	j.Starred = starred == 1
	j.HasOffer = offers > 0
	_ = json.Unmarshal([]byte(tags), &j.Tags)
	if j.Tags == nil {
		j.Tags = []string{}
	}
	j.MinAmount = float64(j.SalaryMin) / 100
	j.MaxAmount = float64(j.SalaryMax) / 100
	return &j, nil
}

// JobFilter narrows a list of jobs.
type JobFilter struct {
	Status   string // open | hired | closed | ended (hired or closed) | "" = all
	StageID  int64
	Kind     string // a stage kind
	Search   string
	Company  string
	Email    string
	Due      string // overdue | today | week | none | any
	Today    string // YYYY-MM-DD the Due filter counts from
	Starred  bool
	Tag      string
	Source   string
	WorkMode string
	Sort     string // stage | due | updated | created | applied | salary | excitement | fit
	Limit    int
	Offset   int
}

func (f JobFilter) where(uid int64) (string, []any) {
	w := []string{"j.user_id = ?"}
	args := []any{uid}
	add := func(c string, a ...any) { w = append(w, c); args = append(args, a...) }
	switch f.Status {
	case StatusOpen, StatusHired, StatusClosed:
		add("j.status = ?", f.Status)
	case "ended":
		add("j.status <> 'open'")
	}
	if f.StageID > 0 {
		add("j.stage_id = ?", f.StageID)
	}
	if f.Kind != "" {
		add("s.kind = ?", f.Kind)
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		like := "%" + q + "%"
		add(`(j.title LIKE ? OR j.company LIKE ? OR j.contact_name LIKE ? OR j.contact_email LIKE ? OR j.location LIKE ? OR j.notes LIKE ? OR j.next_action LIKE ? OR j.tags LIKE ?)`,
			like, like, like, like, like, like, like, like)
	}
	if f.Company != "" {
		add("lower(j.company) = lower(?)", f.Company)
	}
	if f.Email != "" {
		add("lower(j.contact_email) = lower(?)", f.Email)
	}
	today := f.Today
	if today == "" {
		today = Today()
	}
	switch f.Due {
	case "overdue":
		add("j.next_due <> '' AND j.next_due < ?", today)
	case "today":
		add("j.next_due <> '' AND j.next_due <= ?", today)
	case "week":
		add("j.next_due <> '' AND j.next_due <= ?", AddDays(today, 7))
	case "none":
		add("j.next_due = '' AND j.next_action = ''")
	case "any":
		add("(j.next_due <> '' OR j.next_action <> '')")
	}
	if f.Starred {
		add("j.starred = 1")
	}
	if f.Tag != "" {
		add("j.tags LIKE ?", `%"`+f.Tag+`"%`)
	}
	if f.Source != "" {
		add("j.source = ?", f.Source)
	}
	if f.WorkMode != "" {
		add("j.work_mode = ?", f.WorkMode)
	}
	return " WHERE " + strings.Join(w, " AND "), args
}

func (f JobFilter) order() string {
	switch f.Sort {
	case "due":
		return ` ORDER BY j.next_due = '', j.next_due, j.updated_at DESC`
	case "created":
		return ` ORDER BY j.created_at DESC, j.id DESC`
	case "applied":
		return ` ORDER BY j.applied_on = '', j.applied_on DESC, j.id DESC`
	case "salary":
		return ` ORDER BY MAX(j.salary_min, j.salary_max) DESC, j.id DESC`
	case "excitement":
		return ` ORDER BY j.excitement DESC, j.updated_at DESC`
	case "fit":
		return ` ORDER BY j.fit_score DESC, j.updated_at DESC`
	case "stage":
		return ` ORDER BY s.position, j.position, j.id`
	}
	return ` ORDER BY j.updated_at DESC, j.id DESC`
}

// ListJobs returns a page of a person's jobs and how many match.
func (s *Store) ListJobs(ctx context.Context, uid int64, f JobFilter) ([]*Job, int, error) {
	where, args := f.where(uid)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1)`+jobFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+jobCols+jobFrom+where+f.order()+` LIMIT ? OFFSET ?`, append(append([]any{nowLocal()}, args...), limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}

// GetJob returns one of a person's jobs.
func (s *Store) GetJob(ctx context.Context, uid, id int64) (*Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+jobFrom+` WHERE j.user_id = ? AND j.id = ?`, nowLocal(), uid, id))
	return j, notFound(err)
}

// JobBySource finds the job made from something elsewhere (mail:msg:12).
func (s *Store) JobBySource(ctx context.Context, uid int64, ref string) (*Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+jobFrom+` WHERE j.user_id = ? AND j.source_ref = ?`, nowLocal(), uid, ref))
	return j, notFound(err)
}

// JobByClientKey finds an imported job.
func (s *Store) JobByClientKey(ctx context.Context, uid int64, key string) (*Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+jobFrom+` WHERE j.user_id = ? AND j.client_key = ?`, nowLocal(), uid, key))
	return j, notFound(err)
}

// JobByURL finds a job by its posting's address (so a posting is not added twice).
func (s *Store) JobByURL(ctx context.Context, uid int64, url string) (*Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+jobFrom+` WHERE j.user_id = ? AND j.url = ? AND j.url <> '' ORDER BY j.id DESC LIMIT 1`, nowLocal(), uid, url))
	return j, notFound(err)
}

func (s *Store) stageKind(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, uid, stageID int64) (string, error) {
	var kind string
	err := q.QueryRowContext(ctx, `SELECT kind FROM stages WHERE user_id = ? AND id = ?`, uid, stageID).Scan(&kind)
	return kind, notFound(err)
}

// reach applies what entering a stage of this kind means: how far the job
// got, when it was applied for and when they first answered.
func reach(j *Job, kind, now string) {
	if r := Rank(kind); r > j.Reached {
		j.Reached = r
	}
	if j.AppliedOn == "" && Rank(kind) >= 1 {
		j.AppliedOn = Today()
	}
	if j.RespondedAt == "" && (Rank(kind) >= 2 || (kind == KindClosed && j.EndReason == "rejected" && j.AppliedOn != "")) {
		j.RespondedAt = now
	}
}

// CreateJob stores a new job at the top of its stage, and its first line
// on the timeline.
func (s *Store) CreateJob(ctx context.Context, j *Job, first *Activity) error {
	if j.StageID == 0 {
		st, err := s.FirstStage(ctx, j.UserID, KindSaved)
		if err != nil {
			return err
		}
		j.StageID = st.ID
	}
	kind, err := s.stageKind(ctx, s.DB, j.UserID, j.StageID)
	if err != nil {
		return err
	}
	j.Status = StatusOf(kind)
	if j.Status != StatusOpen && j.ClosedAt == "" {
		j.ClosedAt = Now()
	}
	now := Now()
	if j.CreatedAt == "" {
		j.CreatedAt = now
	}
	reach(j, kind, j.CreatedAt)
	j.Tags = cleanTags(j.Tags)
	var minPos sql.NullFloat64
	_ = s.DB.QueryRowContext(ctx, `SELECT MIN(position) FROM jobs WHERE user_id = ? AND stage_id = ?`, j.UserID, j.StageID).Scan(&minPos)
	j.Position = minPos.Float64 - 1
	if j.LastActivityAt == "" {
		j.LastActivityAt = j.CreatedAt
	}
	if j.StageChangedAt == "" {
		j.StageChangedAt = j.CreatedAt
	}
	if j.FitScore == 0 && j.FitNote == "" {
		j.FitScore = -1
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO jobs (user_id, title, company, company_ref, stage_id, status, reached, location, work_mode, employment,
		url, posting, salary_min, salary_max, currency, salary_period, source, excitement, apply_by, applied_on, responded_at,
		contact_name, contact_email, contact_phone, contact_ref, notes, cover_letter, cv_id, fit_score, fit_note,
		next_action, next_due, next_task_ref, tags, starred, position, end_reason, outcome_note, closed_at, stage_changed_at, last_activity_at,
		source_app, source_ref, client_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.UserID, j.Title, j.Company, j.CompanyRef, j.StageID, j.Status, j.Reached, j.Location, j.WorkMode, j.Employment,
		j.URL, j.Posting, j.SalaryMin, j.SalaryMax, j.Currency, j.SalaryPeriod, j.Source, j.Excitement, j.ApplyBy, j.AppliedOn, j.RespondedAt,
		j.ContactName, j.ContactEmail, j.ContactPhone, j.ContactRef, j.Notes, j.CoverLetter, nullI(&j.CVID), j.FitScore, j.FitNote,
		j.NextAction, j.NextDue, j.NextTaskRef, toJSON(j.Tags), boolInt(j.Starred), j.Position, j.EndReason, j.OutcomeNote, j.ClosedAt,
		j.StageChangedAt, j.LastActivityAt, j.SourceApp, j.SourceRef, j.ClientKey, j.CreatedAt, now)
	if err != nil {
		_ = tx.Rollback()
		return conflict(err)
	}
	j.ID, _ = res.LastInsertId()
	if first != nil {
		first.UserID, first.JobID = j.UserID, j.ID
		if first.At == "" {
			first.At = j.CreatedAt
		}
		if err := insertActivity(ctx, tx, first); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// UpdateJob saves a job's fields (not its stage: MoveJob does that).
func (s *Store) UpdateJob(ctx context.Context, j *Job) error {
	j.Tags = cleanTags(j.Tags)
	res, err := s.DB.ExecContext(ctx, `UPDATE jobs SET title = ?, company = ?, company_ref = ?, location = ?, work_mode = ?, employment = ?,
		url = ?, posting = ?, salary_min = ?, salary_max = ?, currency = ?, salary_period = ?, source = ?, excitement = ?, apply_by = ?, applied_on = ?,
		responded_at = ?, reached = ?, contact_name = ?, contact_email = ?, contact_phone = ?, contact_ref = ?, notes = ?, cover_letter = ?, cv_id = ?,
		fit_score = ?, fit_note = ?, next_action = ?, next_due = ?, next_task_ref = ?, tags = ?, starred = ?, end_reason = ?, outcome_note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		j.Title, j.Company, j.CompanyRef, j.Location, j.WorkMode, j.Employment,
		j.URL, j.Posting, j.SalaryMin, j.SalaryMax, j.Currency, j.SalaryPeriod, j.Source, j.Excitement, j.ApplyBy, j.AppliedOn,
		j.RespondedAt, j.Reached, j.ContactName, j.ContactEmail, j.ContactPhone, j.ContactRef, j.Notes, j.CoverLetter, nullI(&j.CVID),
		j.FitScore, j.FitNote, j.NextAction, j.NextDue, j.NextTaskRef, toJSON(j.Tags), boolInt(j.Starred), j.EndReason, j.OutcomeNote, Now(), j.UserID, j.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveJob puts a job in a stage (and a place in it: between the jobs with
// these positions; both nil = the top). Hired or closed ends it; moving back
// to an open stage opens it again. It returns the old stage.
func (s *Store) MoveJob(ctx context.Context, uid, id, stageID int64, before, after *float64) (old *Stage, err error) {
	j, err := s.GetJob(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	old, _ = s.GetStage(ctx, uid, j.StageID)
	kind, err := s.stageKind(ctx, s.DB, uid, stageID)
	if err != nil {
		return nil, err
	}
	pos := j.Position
	switch {
	case before != nil && after != nil:
		pos = (*before + *after) / 2
	case before != nil:
		pos = *before + 1
	case after != nil:
		pos = *after - 1
	case stageID != j.StageID:
		var minPos sql.NullFloat64
		_ = s.DB.QueryRowContext(ctx, `SELECT MIN(position) FROM jobs WHERE user_id = ? AND stage_id = ?`, uid, stageID).Scan(&minPos)
		pos = minPos.Float64 - 1
	}
	now := Now()
	changed, closed := j.StageChangedAt, j.ClosedAt
	if stageID != j.StageID {
		changed = now
		reach(j, kind, now)
	}
	status := StatusOf(kind)
	switch {
	case status == StatusOpen:
		closed = ""
	case j.Status != status || closed == "":
		closed = now
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE jobs SET stage_id = ?, status = ?, reached = ?, applied_on = ?, responded_at = ?, position = ?,
		stage_changed_at = ?, closed_at = ?, updated_at = ?, end_reason = CASE WHEN ? = 'closed' THEN end_reason ELSE '' END WHERE user_id = ? AND id = ?`,
		stageID, status, j.Reached, j.AppliedOn, j.RespondedAt, pos, changed, closed, now, status, uid, id)
	return old, err
}

// SetNextStep changes only a job's next step.
func (s *Store) SetNextStep(ctx context.Context, uid, id int64, action, due, taskRef string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE jobs SET next_action = ?, next_due = ?, next_task_ref = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		action, due, taskRef, Now(), uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetContactRef links a job to cards in Pocket Contacts.
func (s *Store) SetContactRef(ctx context.Context, uid, id int64, contactRef, companyRef string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET contact_ref = CASE WHEN ? <> '' THEN ? ELSE contact_ref END,
		company_ref = CASE WHEN ? <> '' THEN ? ELSE company_ref END WHERE user_id = ? AND id = ?`,
		contactRef, contactRef, companyRef, companyRef, uid, id)
	return err
}

// SetFit stores what the AI thought of the fit.
func (s *Store) SetFit(ctx context.Context, uid, id int64, score int, note string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET fit_score = ?, fit_note = ?, updated_at = ? WHERE user_id = ? AND id = ?`, score, note, Now(), uid, id)
	return err
}

// DeleteJob removes a job with its timeline, interviews and offer.
func (s *Store) DeleteJob(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM jobs WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Tags lists the tags a person uses, most used first.
func (s *Store) Tags(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tags FROM jobs WHERE user_id = ? AND tags <> '[]'`, uid)
	if err != nil {
		return nil, err
	}
	n := map[string]int{}
	var order []string
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var ts []string
		_ = json.Unmarshal([]byte(raw), &ts)
		for _, t := range ts {
			if n[t] == 0 {
				order = append(order, t)
			}
			n[t]++
		}
	}
	rows.Close()
	for i := 1; i < len(order); i++ {
		for k := i; k > 0 && n[order[k]] > n[order[k-1]]; k-- {
			order[k], order[k-1] = order[k-1], order[k]
		}
	}
	if order == nil {
		order = []string{}
	}
	return order, rows.Err()
}

// ---- the timeline ----

const activityCols = `id, user_id, job_id, kind, title, body, at, app, url, source_app, created_at`

func scanActivity(sc interface{ Scan(...any) error }) (*Activity, error) {
	var a Activity
	err := sc.Scan(&a.ID, &a.UserID, &a.JobID, &a.Kind, &a.Title, &a.Body, &a.At, &a.App, &a.URL, &a.SourceApp, &a.CreatedAt)
	return &a, err
}

func insertActivity(ctx context.Context, tx *sql.Tx, a *Activity) error {
	if a.At == "" {
		a.At = Now()
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO activities (user_id, job_id, kind, title, body, at, app, url, source_app) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, a.JobID, a.Kind, a.Title, a.Body, a.At, a.App, a.URL, a.SourceApp)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	// what a person did (not what is planned) counts as the job's last activity
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET last_activity_at = MAX(last_activity_at, ?) WHERE user_id = ? AND id = ?`, a.At, a.UserID, a.JobID)
	return err
}

// AddActivity adds a line to a job's timeline.
func (s *Store) AddActivity(ctx context.Context, a *Activity) error {
	if _, err := s.GetJob(ctx, a.UserID, a.JobID); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := insertActivity(ctx, tx, a); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ListActivities returns a job's timeline, newest first.
func (s *Store) ListActivities(ctx context.Context, uid, jobID int64, limit int) ([]*Activity, error) {
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+activityCols+` FROM activities WHERE user_id = ? AND job_id = ? ORDER BY at DESC, id DESC LIMIT ?`, uid, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActivitiesBetween returns a person's timeline lines in [from, to) (RFC3339).
func (s *Store) ActivitiesBetween(ctx context.Context, uid int64, from, to string, limit int) ([]*Activity, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+activityCols+` FROM activities WHERE user_id = ? AND at >= ? AND at < ? ORDER BY at DESC, id DESC LIMIT ?`, uid, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetActivity returns one line of a person's timeline.
func (s *Store) GetActivity(ctx context.Context, uid, id int64) (*Activity, error) {
	a, err := scanActivity(s.DB.QueryRowContext(ctx, `SELECT `+activityCols+` FROM activities WHERE user_id = ? AND id = ?`, uid, id))
	return a, notFound(err)
}

// UpdateActivity saves a line's words and time.
func (s *Store) UpdateActivity(ctx context.Context, a *Activity) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE activities SET kind = ?, title = ?, body = ?, at = ? WHERE user_id = ? AND id = ?`, a.Kind, a.Title, a.Body, a.At, a.UserID, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteActivity removes a line.
func (s *Store) DeleteActivity(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM activities WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- in the other apps ----

const madeCols = `id, job_id, target, app, ref, title, url, state, error, checked_at, created_at`

// ListMade returns what a job has in the other apps (failed tries last).
func (s *Store) ListMade(ctx context.Context, uid, jobID int64) ([]*Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+madeCols+` FROM made WHERE user_id = ? AND job_id = ? ORDER BY error <> '', created_at DESC, id DESC`, uid, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.JobID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.State, &m.Error, &m.CheckedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// AddMade remembers something a job has in another app.
func (s *Store) AddMade(ctx context.Context, uid int64, m *Made) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO made (user_id, job_id, target, app, ref, title, url, state, error, checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, m.JobID, m.Target, m.App, m.Ref, m.Title, m.URL, m.State, m.Error, m.CheckedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	if m.CreatedAt == "" {
		m.CreatedAt = Now()
	}
	return nil
}

// SetMadeState records what a linked thing is now (sent, signed, paid…).
func (s *Store) SetMadeState(ctx context.Context, uid, id int64, state, title string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE made SET state = ?, title = CASE WHEN ? <> '' THEN ? ELSE title END, checked_at = ? WHERE user_id = ? AND id = ?`,
		state, title, title, Now(), uid, id)
	return err
}

// DeleteMade forgets a linked thing (it stays in the other app).
func (s *Store) DeleteMade(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DropMadeErrors forgets the failed tries of one kind once one worked.
func (s *Store) DropMadeErrors(ctx context.Context, uid, jobID int64, target string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND job_id = ? AND target = ? AND error <> ''`, uid, jobID, target)
}

// MadeToCheck returns linked things whose state may still change
// (tasks not ticked off yet), oldest look first.
func (s *Store) MadeToCheck(ctx context.Context, uid int64, limit int) ([]*Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+madeCols+` FROM made WHERE user_id = ? AND error = '' AND ref <> ''
		AND target = 'task' AND state NOT IN ('completed', 'done', 'cancelled')
		ORDER BY checked_at, id LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.JobID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.State, &m.Error, &m.CheckedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// AllJobs returns every job a person has (for reports, people and exports).
func (s *Store) AllJobs(ctx context.Context, uid int64) ([]*Job, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+jobCols+jobFrom+` WHERE j.user_id = ? ORDER BY j.created_at, j.id`, nowLocal(), uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Job{}
	for rows.Next() {
		d, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
