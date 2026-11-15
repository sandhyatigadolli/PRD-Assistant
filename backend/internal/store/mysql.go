package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"prd-assistant/internal/domain"
)

//go:embed migrations/001_init.sql
var initSQL string

type MySQL struct{ db *sql.DB }

// NewMySQL connects (retrying while the DB container boots) and runs migrations.
func NewMySQL(dsn string) (*MySQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	var last error
	for i := 0; i < 40; i++ {
		if last = db.Ping(); last == nil {
			break
		}
		log.Printf("waiting for MySQL (%d/40): %v", i+1, last)
		time.Sleep(2 * time.Second)
	}
	if last != nil {
		return nil, fmt.Errorf("mysql unreachable: %w", last)
	}
	for _, stmt := range strings.Split(initSQL, ";\n") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
	}
	return &MySQL{db: db}, nil
}

func jsonStr(v any) string { b, _ := json.Marshal(v); return string(b) }

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (m *MySQL) CreateSession(ctx context.Context, s *domain.Session) error {
	now := time.Now().UTC()
	if s.ID == "" {
		s.ID = NewID()
	}
	s.CreatedAt, s.UpdatedAt = now, now
	if s.Repeat == nil {
		s.Repeat = map[string]domain.RepeatState{}
	}
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO sessions (id,title,prd_type,repeat_state,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		s.ID, s.Title, s.PRDType, jsonStr(s.Repeat), now, now)
	return err
}

const sessionCols = `id,title,prd_type,repeat_state,generated,generated_at,created_at,updated_at`

func scanSession(sc interface{ Scan(...any) error }) (*domain.Session, error) {
	var s domain.Session
	var rep string
	var gen sql.NullString
	var genAt sql.NullTime
	if err := sc.Scan(&s.ID, &s.Title, &s.PRDType, &rep, &gen, &genAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Repeat = map[string]domain.RepeatState{}
	_ = json.Unmarshal([]byte(rep), &s.Repeat)
	s.Generated = gen.String
	if genAt.Valid {
		t := genAt.Time
		s.GeneratedAt = &t
	}
	return &s, nil
}

func (m *MySQL) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	s, err := scanSession(m.db.QueryRowContext(ctx, `SELECT `+sessionCols+` FROM sessions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

func (m *MySQL) UpdateSession(ctx context.Context, s *domain.Session) error {
	s.UpdatedAt = time.Now().UTC()
	var genAt any
	if s.GeneratedAt != nil {
		genAt = s.GeneratedAt.UTC()
	}
	_, err := m.db.ExecContext(ctx,
		`UPDATE sessions SET title=?,prd_type=?,repeat_state=?,generated=?,generated_at=?,updated_at=? WHERE id=?`,
		s.Title, s.PRDType, jsonStr(s.Repeat), nullStr(s.Generated), genAt, s.UpdatedAt, s.ID)
	return err
}

func (m *MySQL) ListSessions(ctx context.Context) ([]domain.Session, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT `+sessionCols+` FROM sessions ORDER BY updated_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Session{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		s.Generated = "" // keep list light
		out = append(out, *s)
	}
	return out, rows.Err()
}

const answerCols = `id,session_id,section,item,field,content,tags,skipped,status,feedback,suggestion,created_at,updated_at`

func scanAnswer(sc interface{ Scan(...any) error }) (*domain.Answer, error) {
	var a domain.Answer
	var tags, fb string
	var sug sql.NullString
	if err := sc.Scan(&a.ID, &a.SessionID, &a.Section, &a.Item, &a.Field, &a.Content, &tags, &a.Skipped,
		&a.Status, &fb, &sug, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tags), &a.Tags)
	_ = json.Unmarshal([]byte(fb), &a.Feedback)
	a.Suggest = sug.String
	if a.Tags == nil {
		a.Tags = []string{}
	}
	return &a, nil
}

func (m *MySQL) ListAnswers(ctx context.Context, sid string) ([]domain.Answer, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT `+answerCols+` FROM answers WHERE session_id=? ORDER BY created_at`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Answer{}
	for rows.Next() {
		a, err := scanAnswer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (m *MySQL) GetAnswer(ctx context.Context, sid, aid string) (*domain.Answer, error) {
	a, err := scanAnswer(m.db.QueryRowContext(ctx, `SELECT `+answerCols+` FROM answers WHERE id=? AND session_id=?`, aid, sid))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (m *MySQL) SaveAnswer(ctx context.Context, a *domain.Answer) error {
	now := time.Now().UTC()
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if a.Feedback == nil {
		a.Feedback = []string{}
	}
	if a.ID == "" {
		a.ID = NewID()
		a.CreatedAt, a.UpdatedAt = now, now
		_, err := m.db.ExecContext(ctx,
			`INSERT INTO answers (`+answerCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.SessionID, a.Section, a.Item, a.Field, a.Content, jsonStr(a.Tags), a.Skipped,
			a.Status, jsonStr(a.Feedback), nullStr(a.Suggest), now, now)
		return err
	}
	a.UpdatedAt = now
	_, err := m.db.ExecContext(ctx,
		`UPDATE answers SET content=?,tags=?,skipped=?,status=?,feedback=?,suggestion=?,updated_at=? WHERE id=?`,
		a.Content, jsonStr(a.Tags), a.Skipped, a.Status, jsonStr(a.Feedback), nullStr(a.Suggest), now, a.ID)
	return err
}

func (m *MySQL) AddVersion(ctx context.Context, v *domain.AnswerVersion) error {
	if v.Tags == nil {
		v.Tags = []string{}
	}
	now := time.Now().UTC()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO answer_versions (answer_id,content,tags,created_at) VALUES (?,?,?,?)`,
		v.AnswerID, v.Content, jsonStr(v.Tags), now)
	if err != nil {
		return err
	}
	v.ID, _ = res.LastInsertId()
	v.CreatedAt = now
	// prune: keep only the newest MaxVersions
	_, err = m.db.ExecContext(ctx,
		`DELETE FROM answer_versions WHERE answer_id=? AND id NOT IN (
		   SELECT id FROM (SELECT id FROM answer_versions WHERE answer_id=? ORDER BY id DESC LIMIT ?) t)`,
		v.AnswerID, v.AnswerID, MaxVersions)
	return err
}

func (m *MySQL) ListVersions(ctx context.Context, aid string) ([]domain.AnswerVersion, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id,answer_id,content,tags,created_at FROM answer_versions WHERE answer_id=? ORDER BY id DESC LIMIT ?`,
		aid, MaxVersions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AnswerVersion{}
	for rows.Next() {
		var v domain.AnswerVersion
		var tags string
		if err := rows.Scan(&v.ID, &v.AnswerID, &v.Content, &tags, &v.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tags), &v.Tags)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *MySQL) AddMessage(ctx context.Context, msg *domain.Message) error {
	now := time.Now().UTC()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO messages (session_id,role,content,created_at) VALUES (?,?,?,?)`,
		msg.SessionID, msg.Role, msg.Content, now)
	if err != nil {
		return err
	}
	msg.ID, _ = res.LastInsertId()
	msg.CreatedAt = now
	return nil
}

func (m *MySQL) ListMessages(ctx context.Context, sid string) ([]domain.Message, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id,session_id,role,content,created_at FROM messages WHERE session_id=? ORDER BY id`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Message{}
	for rows.Next() {
		var x domain.Message
		if err := rows.Scan(&x.ID, &x.SessionID, &x.Role, &x.Content, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
