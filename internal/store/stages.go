package store

import (
	"context"
	"database/sql"
	"errors"
)

// ---- the stages of a search ----

// ErrStageInUse means jobs still sit in a stage being removed.
var ErrStageInUse = errors.New("jobs are still in this stage")

const stageCols = `id, user_id, name, kind, position, color, created_at`

func scanStage(sc interface{ Scan(...any) error }) (*Stage, error) {
	var st Stage
	if err := sc.Scan(&st.ID, &st.UserID, &st.Name, &st.Kind, &st.Position, &st.Color, &st.CreatedAt); err != nil {
		return nil, err
	}
	return &st, nil
}

// EnsureStages gives a person the default stages the first time.
func (s *Store) EnsureStages(ctx context.Context, uid int64) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM stages WHERE user_id = ?`, uid).Scan(&n); err != nil || n > 0 {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, st := range DefaultStages {
		if _, err := tx.ExecContext(ctx, `INSERT INTO stages (user_id, name, kind, position, color) VALUES (?, ?, ?, ?, ?)`,
			uid, st.Name, st.Kind, i, st.Color); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ListStages returns a person's stages in order, with the jobs in each.
func (s *Store) ListStages(ctx context.Context, uid int64) ([]*Stage, error) {
	if err := s.EnsureStages(ctx, uid); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+stageCols+` FROM stages WHERE user_id = ? ORDER BY position, id`, uid)
	if err != nil {
		return nil, err
	}
	var out []*Stage
	for rows.Next() {
		st, err := scanStage(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, st)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	crows, err := s.DB.QueryContext(ctx, `SELECT stage_id, COUNT(1) FROM jobs WHERE user_id = ? GROUP BY stage_id`, uid)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var id int64
		var n int
		if err := crows.Scan(&id, &n); err == nil {
			counts[id] = n
		}
	}
	crows.Close()
	for _, st := range out {
		st.Jobs = counts[st.ID]
	}
	return out, nil
}

// GetStage returns one of a person's stages.
func (s *Store) GetStage(ctx context.Context, uid, id int64) (*Stage, error) {
	st, err := scanStage(s.DB.QueryRowContext(ctx, `SELECT `+stageCols+` FROM stages WHERE user_id = ? AND id = ?`, uid, id))
	return st, notFound(err)
}

// FirstStage is the first stage of this kind (new jobs start in the first saved one).
func (s *Store) FirstStage(ctx context.Context, uid int64, kind string) (*Stage, error) {
	if err := s.EnsureStages(ctx, uid); err != nil {
		return nil, err
	}
	st, err := scanStage(s.DB.QueryRowContext(ctx, `SELECT `+stageCols+` FROM stages WHERE user_id = ? AND kind = ? ORDER BY position, id LIMIT 1`, uid, kind))
	return st, notFound(err)
}

// StageByName finds a stage by its name (any case), for imports.
func (s *Store) StageByName(ctx context.Context, uid int64, name string) (*Stage, error) {
	st, err := scanStage(s.DB.QueryRowContext(ctx, `SELECT `+stageCols+` FROM stages WHERE user_id = ? AND lower(name) = lower(?) LIMIT 1`, uid, name))
	return st, notFound(err)
}

// CreateStage adds a stage after the last one of its kind (or where asked).
func (s *Store) CreateStage(ctx context.Context, st *Stage) error {
	if st.Position <= 0 {
		var max sql.NullInt64
		_ = s.DB.QueryRowContext(ctx, `SELECT MAX(position) FROM stages WHERE user_id = ? AND kind = ?`, st.UserID, st.Kind).Scan(&max)
		if !max.Valid {
			_ = s.DB.QueryRowContext(ctx, `SELECT MAX(position) FROM stages WHERE user_id = ? AND kind NOT IN ('hired', 'closed')`, st.UserID).Scan(&max)
		}
		st.Position = int(max.Int64) + 1
		_, _ = s.DB.ExecContext(ctx, `UPDATE stages SET position = position + 1 WHERE user_id = ? AND position >= ?`, st.UserID, st.Position)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO stages (user_id, name, kind, position, color) VALUES (?, ?, ?, ?, ?)`,
		st.UserID, st.Name, st.Kind, st.Position, st.Color)
	if err != nil {
		return err
	}
	st.ID, _ = res.LastInsertId()
	return nil
}

// UpdateStage saves a stage's name, kind and colour. Its kind changes the
// status of the jobs in it too.
func (s *Store) UpdateStage(ctx context.Context, st *Stage) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE stages SET name = ?, kind = ?, color = ? WHERE user_id = ? AND id = ?`,
		st.Name, st.Kind, st.Color, st.UserID, st.ID)
	if err == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			err = ErrNotFound
		}
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE jobs SET status = ? WHERE user_id = ? AND stage_id = ? AND status <> ?`, StatusOf(st.Kind), st.UserID, st.ID, StatusOf(st.Kind))
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ReorderStages puts a person's stages in this order (ids not listed keep theirs after).
func (s *Store) ReorderStages(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE stages SET position = ? WHERE user_id = ? AND id = ?`, i, uid, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeleteStage removes a stage; its jobs move to moveTo first (0 = refuse when it has any).
func (s *Store) DeleteStage(ctx context.Context, uid, id, moveTo int64) error {
	if _, err := s.GetStage(ctx, uid, id); err != nil {
		return err
	}
	var n, left int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM jobs WHERE user_id = ? AND stage_id = ?`, uid, id).Scan(&n)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM stages WHERE user_id = ? AND id <> ?`, uid, id).Scan(&left)
	if left == 0 {
		return ErrStageInUse
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if n > 0 {
		if moveTo == 0 || moveTo == id {
			_ = tx.Rollback()
			return ErrStageInUse
		}
		var kind string
		if err := tx.QueryRowContext(ctx, `SELECT kind FROM stages WHERE user_id = ? AND id = ?`, uid, moveTo).Scan(&kind); err != nil {
			_ = tx.Rollback()
			return notFound(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE jobs SET stage_id = ?, status = ?, stage_changed_at = ? WHERE user_id = ? AND stage_id = ?`, moveTo, StatusOf(kind), Now(), uid, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM stages WHERE user_id = ? AND id = ?`, uid, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
