package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type session struct {
	mu       sync.Mutex
	previous string
	touched  time.Time
}
type Service struct {
	mu         sync.Mutex
	client     Client
	sessions   map[string]*session
	SessionTTL time.Duration
}
type Result struct {
	SessionID string   `json:"session_id"`
	Summary   string   `json:"summary"`
	Details   []string `json:"details"`
}

func New(client Client) *Service {
	return &Service{client: client, sessions: map[string]*session{}, SessionTTL: 24 * time.Hour}
}

func (s *Service) Call(ctx context.Context, input, sessionID string) (Result, error) {
	input = strings.TrimSpace(input)
	sessionID = strings.TrimSpace(sessionID)
	if input == "" || len(input) > 32768 {
		return Result{}, fmt.Errorf("依頼文は1〜32768バイトで指定してください")
	}
	if s.client == nil {
		return Result{}, fmt.Errorf("サブエージェントが設定されていません")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	s.mu.Lock()
	now := time.Now()
	for id, session := range s.sessions {
		if now.Sub(session.touched) > s.SessionTTL {
			delete(s.sessions, id)
		}
	}
	if sessionID != "" && s.sessions[sessionID] == nil {
		s.mu.Unlock()
		return Result{}, fmt.Errorf("指定されたセッションは存在しないか期限切れです")
	}
	sess := s.sessions[sessionID]
	if sessionID == "" {
		if len(s.sessions) >= 32 {
			s.mu.Unlock()
			return Result{}, fmt.Errorf("セッションの上限に達しました。期限切れ後に再試行してください")
		}
		sessionID = uuid.NewString()
		sess = &session{}
		s.sessions[sessionID] = sess
	}
	if sess == nil {
		s.mu.Unlock()
		return Result{}, fmt.Errorf("セッションが期限切れです")
	}
	sess.touched = time.Now()
	s.mu.Unlock()
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	answer, err := s.client.Respond(runCtx, input, sess.previous)
	if err != nil {
		s.mu.Lock()
		if sess.previous == "" {
			delete(s.sessions, sessionID)
		}
		s.mu.Unlock()
		return Result{}, err
	}
	sess.previous = answer.ID
	s.mu.Lock()
	sess.touched = time.Now()
	s.mu.Unlock()
	return Result{SessionID: sessionID, Summary: answer.Summary, Details: answer.Details}, nil
}
