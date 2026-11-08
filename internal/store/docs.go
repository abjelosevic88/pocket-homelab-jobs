package store

import (
	"context"
)

// ---- CVs, cover letters and other papers ----

const docCols = `d.id, d.user_id, d.kind, d.name, d.body, d.file_name, d.file_type, d.file_size, d.doc_ref, d.doc_url, d.is_default,
	d.created_at, d.updated_at, (SELECT COUNT(1) FROM jobs j WHERE j.cv_id = d.id)`

func scanDoc(sc interface{ Scan(...any) error }) (*Doc, error) {
	var d Doc
	var def int
	if err := sc.Scan(&d.ID, &d.UserID, &d.Kind, &d.Name, &d.Body, &d.FileName, &d.FileType, &d.FileSize, &d.DocRef, &d.DocURL, &def,
		&d.CreatedAt, &d.UpdatedAt, &d.Used); err != nil {
		return nil, err
	}
	d.IsDefault = def == 1
	return &d, nil
}

// ListDocs returns a person's papers (of one kind, "" = all), the default first.
func (s *Store) ListDocs(ctx context.Context, uid int64, kind string) ([]*Doc, error) {
	q := `SELECT ` + docCols + ` FROM docs d WHERE d.user_id = ?`
	args := []any{uid}
	if kind != "" {
		q += ` AND d.kind = ?`
		args = append(args, kind)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY d.kind, d.is_default DESC, d.updated_at DESC, d.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Doc{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDoc returns one of a person's papers (without the file).
func (s *Store) GetDoc(ctx context.Context, uid, id int64) (*Doc, error) {
	d, err := scanDoc(s.DB.QueryRowContext(ctx, `SELECT `+docCols+` FROM docs d WHERE d.user_id = ? AND d.id = ?`, uid, id))
	return d, notFound(err)
}

// DefaultCV is the CV a person marked as their main one (else the newest).
func (s *Store) DefaultCV(ctx context.Context, uid int64) (*Doc, error) {
	d, err := scanDoc(s.DB.QueryRowContext(ctx, `SELECT `+docCols+` FROM docs d WHERE d.user_id = ? AND d.kind = 'cv'
		ORDER BY d.is_default DESC, d.updated_at DESC, d.id DESC LIMIT 1`, uid))
	return d, notFound(err)
}

// DocFile returns a paper's file.
func (s *Store) DocFile(ctx context.Context, uid, id int64) ([]byte, string, string, error) {
	var b []byte
	var name, typ string
	err := s.DB.QueryRowContext(ctx, `SELECT file, file_name, file_type FROM docs WHERE user_id = ? AND id = ? AND file IS NOT NULL`, uid, id).Scan(&b, &name, &typ)
	return b, name, typ, notFound(err)
}

// CreateDoc stores a new paper (and its file, if any).
func (s *Store) CreateDoc(ctx context.Context, d *Doc, file []byte) error {
	var blob any
	if len(file) > 0 {
		blob = file
		d.FileSize = int64(len(file))
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO docs (user_id, kind, name, body, file, file_name, file_type, file_size, doc_ref, doc_url, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.UserID, d.Kind, d.Name, d.Body, blob, d.FileName, d.FileType, d.FileSize, d.DocRef, d.DocURL, boolInt(d.IsDefault))
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	if d.IsDefault {
		return s.SetDefaultDoc(ctx, d.UserID, d.ID)
	}
	return nil
}

// UpdateDoc saves a paper's name and text.
func (s *Store) UpdateDoc(ctx context.Context, d *Doc) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE docs SET kind = ?, name = ?, body = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		d.Kind, d.Name, d.Body, Now(), d.UserID, d.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDocFile replaces (or with nil removes) a paper's file.
func (s *Store) SetDocFile(ctx context.Context, uid, id int64, file []byte, name, typ string) error {
	var blob any
	if len(file) > 0 {
		blob = file
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE docs SET file = ?, file_name = ?, file_type = ?, file_size = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		blob, name, typ, len(file), Now(), uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDefaultDoc makes one paper the main one of its kind.
func (s *Store) SetDefaultDoc(ctx context.Context, uid, id int64) error {
	d, err := s.GetDoc(ctx, uid, id)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE docs SET is_default = (id = ?) WHERE user_id = ? AND kind = ?`, id, uid, d.Kind); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DeleteDoc removes a paper (jobs that used it keep their other details).
func (s *Store) DeleteDoc(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM docs WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
