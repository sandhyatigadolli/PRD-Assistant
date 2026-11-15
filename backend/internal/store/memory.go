package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"prd-assistant/internal/domain"
)

// Memory is an in-process Store used for tests and quick local dev (STORE=memory).
type Memory struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session
	answers  map[string]*domain.Answer
	versions map[string][]domain.AnswerVersion
	messages map[string][]domain.Message
	seq      int64
}

func NewMemory() *Memory {
	return &Memory{
		sessions: map[string]*domain.Session{}, answers: map[string]*domain.Answer{},
		versions: map[string][]domain.AnswerVersion{}, messages: map[string][]domain.Message{},
	}
}

func (m *Memory) CreateSession(_ context.Context, s *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if s.ID == "" {
		s.ID = NewID()
	}
	s.CreatedAt, s.UpdatedAt = now, now
	c := *s
	m.sessions[s.ID] = &c
	return nil
}

func (m *Memory) GetSession(_ context.Context, id string) (*domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *s
	c.Repeat = map[string]domain.RepeatState{}
	for k, v := range s.Repeat {
		c.Repeat[k] = v
	}
	return &c, nil
}

func (m *Memory) UpdateSession(_ context.Context, s *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[s.ID]; !ok {
		return ErrNotFound
	}
	s.UpdatedAt = time.Now()
	c := *s
	m.sessions[s.ID] = &c
	return nil
}

func (m *Memory) ListSessions(_ context.Context) ([]domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.Session{}
	for _, s := range m.sessions {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (m *Memory) ListAnswers(_ context.Context, sid string) ([]domain.Answer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.Answer{}
	for _, a := range m.answers {
		if a.SessionID == sid {
			out = append(out, *a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetAnswer(_ context.Context, sid, aid string) (*domain.Answer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.answers[aid]
	if !ok || a.SessionID != sid {
		return nil, ErrNotFound
	}
	c := *a
	return &c, nil
}

func (m *Memory) SaveAnswer(_ context.Context, a *domain.Answer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if a.ID == "" {
		a.ID = NewID()
		a.CreatedAt = now
		m.seq++
		a.CreatedAt = a.CreatedAt.Add(time.Duration(m.seq)) // stable ordering
	}
	a.UpdatedAt = now
	c := *a
	m.answers[a.ID] = &c
	return nil
}

func (m *Memory) AddVersion(_ context.Context, v *domain.AnswerVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	v.ID = m.seq
	v.CreatedAt = time.Now()
	vs := append(m.versions[v.AnswerID], *v)
	if len(vs) > MaxVersions {
		vs = vs[len(vs)-MaxVersions:]
	}
	m.versions[v.AnswerID] = vs
	return nil
}

func (m *Memory) ListVersions(_ context.Context, aid string) ([]domain.AnswerVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	vs := m.versions[aid]
	out := make([]domain.AnswerVersion, len(vs))
	for i, v := range vs {
		out[len(vs)-1-i] = v // newest first
	}
	return out, nil
}

func (m *Memory) AddMessage(_ context.Context, msg *domain.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	msg.ID = m.seq
	msg.CreatedAt = time.Now()
	m.messages[msg.SessionID] = append(m.messages[msg.SessionID], *msg)
	return nil
}

func (m *Memory) ListMessages(_ context.Context, sid string) ([]domain.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.Message{}, m.messages[sid]...), nil
}
