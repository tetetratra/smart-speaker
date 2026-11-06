package subagent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrPermissionGone = errors.New("許可要求は期限切れ、または回答済みです")

type Request struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Input     string    `json:"input"`
	CreatedAt time.Time `json:"created_at"`
	decision  chan bool
}
type session struct {
	mu       sync.Mutex
	previous string
	touched  time.Time
}
type Service struct {
	mu              sync.Mutex
	client          Client
	requests        map[string]*Request
	sessions        map[string]*session
	ApprovalTimeout time.Duration
	SessionTTL      time.Duration
}
type Result struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

func New(client Client) *Service {
	return &Service{client: client, requests: map[string]*Request{}, sessions: map[string]*session{}, ApprovalTimeout: 2 * time.Minute, SessionTTL: 24 * time.Hour}
}

func (s *Service) Pending() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Request, 0, len(s.requests))
	for _, req := range s.requests {
		result = append(result, Request{ID: req.ID, SessionID: req.SessionID, Input: req.Input, CreatedAt: req.CreatedAt})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}
func (s *Service) Resolve(id string, approved bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	req := s.requests[id]
	if req == nil {
		return ErrPermissionGone
	}
	if time.Since(req.CreatedAt) >= s.ApprovalTimeout {
		delete(s.requests, id)
		return ErrPermissionGone
	}
	delete(s.requests, id)
	req.decision <- approved
	return nil
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
	// モデルへの送信は、この依頼文に対する画面上の許可が届くまで行わない。
	req := &Request{ID: uuid.NewString(), Input: input, SessionID: sessionID, CreatedAt: time.Now(), decision: make(chan bool, 1)}
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
	if len(s.requests) >= 32 {
		s.mu.Unlock()
		return Result{}, fmt.Errorf("許可待ちの依頼が多すぎます")
	}
	s.requests[req.ID] = req
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.requests, req.ID); s.mu.Unlock() }()
	timer := time.NewTimer(s.ApprovalTimeout)
	defer timer.Stop()
	select {
	case approved := <-req.decision:
		if !approved {
			return Result{}, fmt.Errorf("ユーザーがサブエージェントへの送信を拒否しました")
		}
	case <-timer.C:
		return Result{}, fmt.Errorf("サブエージェントへの送信許可が期限切れになりました")
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	s.mu.Lock()
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
	return Result{SessionID: sessionID, Text: answer.Text}, nil
}
