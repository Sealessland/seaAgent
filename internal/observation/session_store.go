package observation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/agent"
)

type conversationSession struct {
	ID        string                   `json:"id"`
	Summary   string                   `json:"summary,omitempty"`
	Messages  []agent.ConversationTurn `json:"messages"`
	Images    []sessionImageRef        `json:"images,omitempty"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type sessionImageRef struct {
	Path       string    `json:"path"`
	Source     string    `json:"source,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
}

type sessionStore struct {
	dir string
}

func newSessionStore(workdir string) (*sessionStore, error) {
	dir := filepath.Join(workdir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &sessionStore{dir: dir}, nil
}

func (s *sessionStore) Load(id string) (*conversationSession, error) {
	path := s.path(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var session conversationSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *sessionStore) Save(session *conversationSession) error {
	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(session.ID), data, 0o644)
}

func (s *sessionStore) New() *conversationSession {
	return &conversationSession{
		ID:        fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Messages:  []agent.ConversationTurn{},
		Images:    []sessionImageRef{},
		UpdatedAt: time.Now(),
	}
}

func (s *sessionStore) path(id string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return -1
		}
	}, id)
	if clean == "" {
		clean = "invalid"
	}
	return filepath.Join(s.dir, clean+".json")
}

func summarizeHistory(summary string, messages []agent.ConversationTurn) (string, []agent.ConversationTurn) {
	const keepRecent = 8
	if len(messages) <= keepRecent {
		return summary, messages
	}

	archived := messages[:len(messages)-keepRecent]
	recent := messages[len(messages)-keepRecent:]

	var builder strings.Builder
	if strings.TrimSpace(summary) != "" {
		builder.WriteString(summary)
		builder.WriteString("\n")
	}
	builder.WriteString("Earlier conversation summary:\n")
	for _, turn := range archived {
		text := strings.TrimSpace(turn.Content)
		if text == "" {
			continue
		}
		if len(text) > 180 {
			text = text[:180] + "..."
		}
		builder.WriteString("- ")
		builder.WriteString(turn.Role)
		builder.WriteString(": ")
		builder.WriteString(text)
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String()), recent
}

func (s *conversationSession) rememberImage(path string, source string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	filtered := make([]sessionImageRef, 0, len(s.Images)+1)
	for _, item := range s.Images {
		if strings.TrimSpace(item.Path) == "" || item.Path == path {
			continue
		}
		filtered = append(filtered, item)
	}
	filtered = append(filtered, sessionImageRef{
		Path:       path,
		Source:     strings.TrimSpace(source),
		CapturedAt: time.Now(),
	})

	const keepRecentImages = 6
	if len(filtered) > keepRecentImages {
		filtered = filtered[len(filtered)-keepRecentImages:]
	}
	s.Images = filtered
}

func (s *conversationSession) latestImagePath() string {
	for i := len(s.Images) - 1; i >= 0; i-- {
		path := strings.TrimSpace(s.Images[i].Path)
		if path != "" {
			return path
		}
	}
	return ""
}

func (s *conversationSession) previousImagePath(exclude string) string {
	exclude = strings.TrimSpace(exclude)
	for i := len(s.Images) - 1; i >= 0; i-- {
		path := strings.TrimSpace(s.Images[i].Path)
		if path == "" || path == exclude {
			continue
		}
		return path
	}
	return ""
}
