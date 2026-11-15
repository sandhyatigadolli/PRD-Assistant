// Package domain holds the plain data types shared by every layer.
package domain

import "time"

// RepeatState tracks a repeatable section (e.g. personas, requirements).
type RepeatState struct {
	Items  int  `json:"items"`  // how many items have been started (>=1)
	Closed bool `json:"closed"` // user said "no more"
}

type Session struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	PRDType     string                 `json:"prd_type"`
	Repeat      map[string]RepeatState `json:"repeat"`
	Generated   string                 `json:"generated,omitempty"` // polished PRD (markdown)
	GeneratedAt *time.Time             `json:"generated_at,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

const (
	StatusAccepted    = "accepted"
	StatusNeedsReview = "needs_review"
)

type Answer struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Section   string    `json:"section"`
	Item      int       `json:"item"`
	Field     string    `json:"field"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	Skipped   bool      `json:"skipped"`
	Status    string    `json:"status"`             // accepted | needs_review
	Feedback  []string  `json:"feedback,omitempty"` // validator issues
	Suggest   string    `json:"suggestion,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AnswerVersion struct {
	ID        int64     `json:"id"`
	AnswerID  string    `json:"answer_id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // agent | user | system
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Tag is a label the user can attach to an answer.
type Tag struct {
	Key         string `json:"key"`
	Group       string `json:"group"` // priority | release | status
	Description string `json:"description"`
}

var Tags = []Tag{
	{"P0", "priority", "Must-have. Launch is blocked without it."},
	{"P1", "priority", "Should-have. Important but not launch-blocking."},
	{"P2", "priority", "Nice-to-have. Do if time allows."},
	{"V1", "release", "Ships in version 1 / MVP."},
	{"V2", "release", "Planned for version 2."},
	{"V3", "release", "Planned for version 3."},
	{"FUTURE", "release", "Backlog / later."},
	{"IMPORTANT", "status", "Call-out: reviewers must read this."},
	{"NEEDS REVIEW", "status", "Draft, needs stakeholder review."},
	{"OPEN QUESTION", "status", "Unresolved, needs a decision."},
}

func IsPriorityTag(t string) bool { return t == "P0" || t == "P1" || t == "P2" }

func ValidTag(t string) bool {
	for _, x := range Tags {
		if x.Key == t {
			return true
		}
	}
	return false
}
