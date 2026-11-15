package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"prd-assistant/internal/domain"
)

var ErrNotFound = errors.New("not found")

// MaxVersions is how many previous versions of a block we keep (and show).
const MaxVersions = 3

type Store interface {
	CreateSession(ctx context.Context, s *domain.Session) error
	GetSession(ctx context.Context, id string) (*domain.Session, error)
	UpdateSession(ctx context.Context, s *domain.Session) error
	ListSessions(ctx context.Context) ([]domain.Session, error)

	ListAnswers(ctx context.Context, sessionID string) ([]domain.Answer, error)
	GetAnswer(ctx context.Context, sessionID, answerID string) (*domain.Answer, error)
	// SaveAnswer inserts (empty ID) or updates by ID.
	SaveAnswer(ctx context.Context, a *domain.Answer) error

	// AddVersion stores a snapshot of an answer's previous value and prunes to MaxVersions.
	AddVersion(ctx context.Context, v *domain.AnswerVersion) error
	ListVersions(ctx context.Context, answerID string) ([]domain.AnswerVersion, error) // newest first

	AddMessage(ctx context.Context, m *domain.Message) error
	ListMessages(ctx context.Context, sessionID string) ([]domain.Message, error)
}

func NewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
