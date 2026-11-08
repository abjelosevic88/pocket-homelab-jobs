package store

import (
	"context"
	"encoding/json"
	"strings"
)

// ---- interviews ----

const interviewCols = `i.id, i.user_id, i.job_id, i.kind, i.starts_at, i.minutes, i.location, i.with_people, i.prep, i.questions,
	i.outcome, i.feedback, i.thanks_sent, i.event_ref, i.event_url, i.created_at, i.updated_at,
	COALESCE(j.title, ''), COALESCE(j.company, ''), COALESCE(j.status, ''), COALESCE(s.color, '')`

const interviewFrom = ` FROM interviews i JOIN jobs j ON j.id = i.job_id LEFT JOIN stages s ON s.id = j.stage_id `

func scanInterview(sc interface{ Scan(...any) error }) (*Interview, error) {
	var iv Interview
	var qs string
	var thanks int
	if err := sc.Scan(&iv.ID, &iv.UserID, &iv.JobID, &iv.Kind, &iv.StartsAt, &iv.Minutes, &iv.Location, &iv.WithPeople, &iv.Prep, &qs,
		&iv.Outcome, &iv.Feedback, &thanks, &iv.EventRef, &iv.EventURL, &iv.CreatedAt, &iv.UpdatedAt,
		&iv.JobTitle, &iv.Company, &iv.JobStatus, &iv.StageColor); err != nil {
		return nil, err
	}
	iv.ThanksSent = thanks == 1
	_ = json.Unmarshal([]byte(qs), &iv.Questions)
	if iv.Questions == nil {
		iv.Questions = []QA{}
	}
	now := nowLocal()
	if len(iv.StartsAt) == 10 {
		iv.Past = iv.StartsAt < now[:10]
	} else {
		iv.Past = iv.StartsAt != "" && iv.StartsAt < now
	}
	return &iv, nil
}

func cleanQA(in []QA) []QA {
	out := []QA{}
	for _, q := range in {
		q.Q, q.A = strings.TrimSpace(q.Q), strings.TrimSpace(q.A)
		if q.Q == "" || len(out) >= 60 {
			continue
		}
		out = append(out, q)
	}
	return out
}

// InterviewFilter narrows a list of interviews.
type InterviewFilter struct {
	JobID int64
	When  string // upcoming | past | "" = all
	From  string // local YYYY-MM-DD (inclusive)
	To    string // local YYYY-MM-DD (exclusive)
	Limit int
}

// ListInterviews returns a person's interviews: upcoming soonest first,
// past newest first.
func (s *Store) ListInterviews(ctx context.Context, uid int64, f InterviewFilter) ([]*Interview, error) {
	w := []string{"i.user_id = ?"}
	args := []any{uid}
	order := " ORDER BY i.starts_at, i.id"
	if f.JobID > 0 {
		w = append(w, "i.job_id = ?")
		args = append(args, f.JobID)
	}
	now := nowLocal()
	switch f.When {
	case "upcoming":
		w = append(w, "(i.starts_at >= ? OR (length(i.starts_at) = 10 AND i.starts_at >= ?))")
		args = append(args, now, now[:10])
	case "past":
		w = append(w, "i.starts_at <> '' AND i.starts_at < ? AND NOT (length(i.starts_at) = 10 AND i.starts_at >= ?)")
		args = append(args, now, now[:10])
		order = " ORDER BY i.starts_at DESC, i.id DESC"
	}
	if f.From != "" {
		w = append(w, "i.starts_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		w = append(w, "i.starts_at < ?")
		args = append(args, f.To)
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+interviewCols+interviewFrom+` WHERE `+strings.Join(w, " AND ")+order+` LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Interview{}
	for rows.Next() {
		iv, err := scanInterview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, iv)
	}
	return out, rows.Err()
}

// GetInterview returns one of a person's interviews.
func (s *Store) GetInterview(ctx context.Context, uid, id int64) (*Interview, error) {
	iv, err := scanInterview(s.DB.QueryRowContext(ctx, `SELECT `+interviewCols+interviewFrom+` WHERE i.user_id = ? AND i.id = ?`, uid, id))
	return iv, notFound(err)
}

// CreateInterview stores an interview for one of the person's jobs.
func (s *Store) CreateInterview(ctx context.Context, iv *Interview) error {
	if _, err := s.GetJob(ctx, iv.UserID, iv.JobID); err != nil {
		return err
	}
	iv.Questions = cleanQA(iv.Questions)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO interviews (user_id, job_id, kind, starts_at, minutes, location, with_people, prep, questions,
		outcome, feedback, thanks_sent, event_ref, event_url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iv.UserID, iv.JobID, iv.Kind, iv.StartsAt, iv.Minutes, iv.Location, iv.WithPeople, iv.Prep, toJSON(iv.Questions),
		iv.Outcome, iv.Feedback, boolInt(iv.ThanksSent), iv.EventRef, iv.EventURL)
	if err != nil {
		return err
	}
	iv.ID, _ = res.LastInsertId()
	return nil
}

// UpdateInterview saves an interview.
func (s *Store) UpdateInterview(ctx context.Context, iv *Interview) error {
	iv.Questions = cleanQA(iv.Questions)
	res, err := s.DB.ExecContext(ctx, `UPDATE interviews SET kind = ?, starts_at = ?, minutes = ?, location = ?, with_people = ?, prep = ?, questions = ?,
		outcome = ?, feedback = ?, thanks_sent = ?, event_ref = ?, event_url = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		iv.Kind, iv.StartsAt, iv.Minutes, iv.Location, iv.WithPeople, iv.Prep, toJSON(iv.Questions),
		iv.Outcome, iv.Feedback, boolInt(iv.ThanksSent), iv.EventRef, iv.EventURL, Now(), iv.UserID, iv.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteInterview removes an interview.
func (s *Store) DeleteInterview(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM interviews WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- offers ----

const offerCols = `o.id, o.user_id, o.job_id, o.base, o.currency, o.period, o.bonus, o.equity, o.pto_days, o.benefits, o.start_date,
	o.decide_by, o.status, o.notes, o.created_at, o.updated_at,
	COALESCE(j.title, ''), COALESCE(j.company, ''), COALESCE(j.location, ''), COALESCE(j.work_mode, ''), COALESCE(j.excitement, 0)`

const offerFrom = ` FROM offers o JOIN jobs j ON j.id = o.job_id `

// Yearly is a salary for a year: a month ×12, an hour ×1,760 (40 h × 44 weeks).
func Yearly(amount int64, period string) int64 {
	switch period {
	case "month":
		return amount * 12
	case "hour":
		return amount * 1760
	}
	return amount
}

func scanOffer(sc interface{ Scan(...any) error }) (*Offer, error) {
	var o Offer
	if err := sc.Scan(&o.ID, &o.UserID, &o.JobID, &o.Base, &o.Currency, &o.Period, &o.Bonus, &o.Equity, &o.PTODays, &o.Benefits, &o.StartDate,
		&o.DecideBy, &o.Status, &o.Notes, &o.CreatedAt, &o.UpdatedAt,
		&o.JobTitle, &o.Company, &o.Location, &o.WorkMode, &o.Excitement); err != nil {
		return nil, err
	}
	o.BaseAmount = float64(o.Base) / 100
	o.BonusAmount = float64(o.Bonus) / 100
	o.Yearly = Yearly(o.Base, o.Period) + o.Bonus
	return &o, nil
}

// ListOffers returns a person's offers, the ones still to decide first.
func (s *Store) ListOffers(ctx context.Context, uid int64) ([]*Offer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+offerCols+offerFrom+` WHERE o.user_id = ?
		ORDER BY o.status NOT IN ('open', 'negotiating'), o.decide_by = '', o.decide_by, o.created_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Offer{}
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// OfferFor returns the offer for one of a person's jobs.
func (s *Store) OfferFor(ctx context.Context, uid, jobID int64) (*Offer, error) {
	o, err := scanOffer(s.DB.QueryRowContext(ctx, `SELECT `+offerCols+offerFrom+` WHERE o.user_id = ? AND o.job_id = ?`, uid, jobID))
	return o, notFound(err)
}

// SaveOffer adds or replaces the offer for a job.
func (s *Store) SaveOffer(ctx context.Context, o *Offer) error {
	if _, err := s.GetJob(ctx, o.UserID, o.JobID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO offers (user_id, job_id, base, currency, period, bonus, equity, pto_days, benefits, start_date, decide_by, status, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, job_id) DO UPDATE SET base = excluded.base, currency = excluded.currency, period = excluded.period, bonus = excluded.bonus,
		equity = excluded.equity, pto_days = excluded.pto_days, benefits = excluded.benefits, start_date = excluded.start_date,
		decide_by = excluded.decide_by, status = excluded.status, notes = excluded.notes, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		o.UserID, o.JobID, o.Base, o.Currency, o.Period, o.Bonus, o.Equity, o.PTODays, o.Benefits, o.StartDate, o.DecideBy, o.Status, o.Notes)
	if err != nil {
		return err
	}
	saved, err := s.OfferFor(ctx, o.UserID, o.JobID)
	if err == nil {
		*o = *saved
	}
	return err
}

// DeleteOffer removes the offer for a job.
func (s *Store) DeleteOffer(ctx context.Context, uid, jobID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM offers WHERE user_id = ? AND job_id = ?`, uid, jobID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
