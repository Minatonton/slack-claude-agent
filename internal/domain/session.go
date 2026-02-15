package domain

import (
	"context"
	"sync"
	"time"
)

type AgentMode int

const (
	ModeImplementation AgentMode = iota // デフォルト: 実装モード
	ModeReview                           // レビューモード
)

func (m AgentMode) String() string {
	switch m {
	case ModeReview:
		return "レビュー"
	case ModeImplementation:
		return "実装"
	default:
		return "不明"
	}
}

type ExecutionMode int

const (
	ExecutionAsync ExecutionMode = iota // デフォルト: 非同期実行（並列）
	ExecutionSync                        // 同期実行（順次）
)

func (e ExecutionMode) String() string {
	switch e {
	case ExecutionSync:
		return "順次実行"
	case ExecutionAsync:
		return "並列実行"
	default:
		return "不明"
	}
}

// Session represents a conversation session in a Slack thread.
type Session struct {
	Mu sync.Mutex // Exported for external access

	ThreadTS      string
	Channel       string
	Mode          AgentMode
	ExecutionMode ExecutionMode // Sync or Async execution
	Repository    *Repository   // Current repository for this session
	SessionID     string        // Claude session ID for resume
	IsRunning     bool
	IsActive      bool
	StatusMsgTS   string
	LastActivity  time.Time
	CancelFunc    context.CancelFunc
}

func NewSession(channel, threadTS string, defaultRepo *Repository) *Session {
	return &Session{
		ThreadTS:      threadTS,
		Channel:       channel,
		Mode:          ModeImplementation, // デフォルトは実装モード
		ExecutionMode: ExecutionAsync,     // デフォルトは並列実行
		Repository:    defaultRepo,
		IsActive:      true,
		LastActivity:  time.Now(),
	}
}

func (s *Session) SetMode(mode AgentMode) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Mode = mode
}

func (s *Session) GetMode() AgentMode {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.Mode
}

func (s *Session) SetRunning(running bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.IsRunning = running
}

func (s *Session) Running() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.IsRunning
}

func (s *Session) UpdateActivity() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.LastActivity = time.Now()
}

func (s *Session) Deactivate() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.IsActive = false
	if s.CancelFunc != nil {
		s.CancelFunc()
	}
}

func (s *Session) Active() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.IsActive
}

func (s *Session) SetRepository(repo *Repository) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Repository = repo
}

func (s *Session) GetRepository() *Repository {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.Repository
}

func (s *Session) SetExecutionMode(mode ExecutionMode) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.ExecutionMode = mode
}

func (s *Session) GetExecutionMode() ExecutionMode {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.ExecutionMode
}
