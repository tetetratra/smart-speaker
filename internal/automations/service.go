package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Spec struct {
	Name        string   `json:"name"`
	Instruction string   `json:"instruction"`
	Schedule    Schedule `json:"schedule"`
}
type Job struct {
	ID string `json:"id"`
	Spec
	NextRunAt  time.Time `json:"next_run_at,omitzero"`
	LastRunAt  time.Time `json:"last_run_at,omitzero"`
	LastStatus string    `json:"last_status"`
	LastError  string    `json:"last_error,omitempty"`
}
type Delivery struct {
	ID, Name, Instruction string
}
type Service struct {
	mu      sync.Mutex
	path    string
	jobs    map[string]Job
	deliver func(context.Context, Delivery) error
}

func New(path string, deliver func(context.Context, Delivery) error) (*Service, error) {
	s := &Service{path: path, jobs: map[string]Job{}, deliver: deliver}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var jobs []Job
	if err = json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("予約ジョブの読み込み: %w", err)
	}
	if len(jobs) > 100 {
		return nil, fmt.Errorf("予約ジョブが100件を超えています")
	}
	for _, job := range jobs {
		if job.ID == "" {
			return nil, fmt.Errorf("予約ジョブのIDが空です")
		}
		if _, err := validate(job.Spec, time.Now(), false); err != nil {
			return nil, err
		}
		if _, exists := s.jobs[job.ID]; exists {
			return nil, fmt.Errorf("予約ジョブのIDが重複しています")
		}
		s.jobs[job.ID] = job
	}
	return s, nil
}
func validate(spec Spec, now time.Time, creating bool) (Spec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Instruction = strings.TrimSpace(spec.Instruction)
	if spec.Name == "" || len(spec.Name) > 240 || spec.Instruction == "" || len(spec.Instruction) > 32768 {
		return spec, fmt.Errorf("名前は1〜240バイト、指示は1〜32768バイトで指定してください")
	}
	if spec.Schedule.Timezone == "" {
		spec.Schedule.Timezone = "Asia/Tokyo"
	}
	next, err := spec.Schedule.next(now, creating)
	if err != nil {
		return spec, err
	}
	if spec.Schedule.AfterSeconds != 0 {
		spec.Schedule.At = next
		spec.Schedule.AfterSeconds = 0
	}
	return spec, nil
}
func (s *Service) List() []Job { s.mu.Lock(); defer s.mu.Unlock(); return s.listLocked() }
func (s *Service) listLocked() []Job {
	jobs := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].NextRunAt.Equal(jobs[j].NextRunAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].NextRunAt.Before(jobs[j].NextRunAt)
	})
	return jobs
}
func (s *Service) saveLocked() error {
	if s.path == "" {
		return fmt.Errorf("予約ジョブの保存先がありません")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.listLocked(), "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".automations-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
func (s *Service) Create(spec Spec, now time.Time) (Job, error) {
	spec, err := validate(spec, now, true)
	if err != nil {
		return Job{}, err
	}
	next, _ := spec.Schedule.next(now, false)
	job := Job{ID: uuid.NewString(), Spec: spec, NextRunAt: next, LastStatus: "scheduled"}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jobs) >= 100 {
		return Job{}, fmt.Errorf("予約ジョブは100件までです")
	}
	s.jobs[job.ID] = job
	if err := s.saveLocked(); err != nil {
		delete(s.jobs, job.ID)
		return Job{}, err
	}
	return job, nil
}
func (s *Service) Update(id string, spec Spec, now time.Time) (Job, error) {
	spec, err := validate(spec, now, true)
	if err != nil {
		return Job{}, err
	}
	next, _ := spec.Schedule.next(now, false)
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.jobs[id]
	if !ok {
		return Job{}, os.ErrNotExist
	}
	job := old
	job.Spec = spec
	job.NextRunAt = next
	job.LastStatus = "scheduled"
	job.LastError = ""
	s.jobs[id] = job
	if err := s.saveLocked(); err != nil {
		s.jobs[id] = old
		return Job{}, err
	}
	return job, nil
}
func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.jobs[id]
	if !ok {
		return os.ErrNotExist
	}
	delete(s.jobs, id)
	if err := s.saveLocked(); err != nil {
		s.jobs[id] = old
		return err
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := s.RunDue(ctx, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "予約ジョブの実行: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) RunDue(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	var due []Job
	old := map[string]Job{}
	for _, job := range s.listLocked() {
		if job.NextRunAt.IsZero() || job.NextRunAt.After(now) {
			continue
		}
		due = append(due, job)
		old[job.ID] = job
		job.LastRunAt = now
		job.LastStatus = "queued"
		job.LastError = ""
		if job.Schedule.Cron == "" {
			// 日時指定の予約は一度きりなので、実行時に一覧から削除する。
			delete(s.jobs, job.ID)
			continue
		}
		job.NextRunAt, _ = job.Schedule.next(now, false)
		s.jobs[job.ID] = job
	}
	if len(due) > 0 {
		if err := s.saveLocked(); err != nil {
			for id, j := range old {
				s.jobs[id] = j
			}
			s.mu.Unlock()
			return err
		}
	}
	s.mu.Unlock()
	// 実行済みの予定を先に保存し、再起動で同じ一度きりの予約を繰り返さない。
	for _, job := range due {
		var err error
		if s.deliver == nil {
			err = fmt.Errorf("AIへの送信経路がありません")
		} else {
			deliveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = s.deliver(deliveryCtx, Delivery{ID: job.ID, Name: job.Name, Instruction: job.Instruction})
			cancel()
		}
		if err != nil && job.Schedule.Cron == "" {
			// 削除済みの一度きりの予約は画面に失敗を残せないため、ログへ出す。
			fmt.Fprintf(os.Stderr, "予約ジョブの送信失敗 id=%s name=%s: %v\n", job.ID, job.Name, err)
		}
		s.mu.Lock()
		current, ok := s.jobs[job.ID]
		if ok && current.LastRunAt.Equal(now) {
			current.LastStatus = "submitted"
			if err != nil {
				current.LastStatus = "failed"
				current.LastError = err.Error()
			}
			s.jobs[job.ID] = current
			if saveErr := s.saveLocked(); saveErr != nil {
				s.mu.Unlock()
				return saveErr
			}
		}
		s.mu.Unlock()
	}
	return nil
}
