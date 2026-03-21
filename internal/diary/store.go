package diary

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultDiaryPath      = "data/diary.md"
	defaultLegacyDiaryDir = "tmp/diary"
)

type Config struct {
	Path      string
	LegacyDir string
}

type Store struct {
	path      string
	legacyDir string
	mu        sync.Mutex
}

func NewStore(cfg Config) *Store {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = defaultDiaryPath
	}
	legacyDir := strings.TrimSpace(cfg.LegacyDir)
	if legacyDir == "" {
		legacyDir = defaultLegacyDiaryDir
	}
	return &Store{
		path:      path,
		legacyDir: legacyDir,
	}
}

func (s *Store) Content() (string, error) {
	path, err := s.ensureFile()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *Store) AppendEntry(when time.Time, content string) (string, error) {
	path, err := s.ensureFile()
	if err != nil {
		return "", err
	}
	header := "# " + when.Format("2006-01-02 15:04")
	body := header + "\n" + strings.TrimLeft(content, "\n")
	body = strings.TrimRight(body, "\n") + "\n"

	s.mu.Lock()
	defer s.mu.Unlock()

	sep := ""
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		sep = "\n\n"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(sep + body); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) ensureFile() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.path); err == nil {
		return s.path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return "", err
	}
	entries, err := s.readLegacyEntries()
	if err != nil {
		return "", err
	}
	content := strings.Join(entries, "\n\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(s.path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return s.path, nil
}

func (s *Store) readLegacyEntries() ([]string, error) {
	entries, err := os.ReadDir(s.legacyDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".md") {
			files = append(files, filepath.Join(s.legacyDir, name))
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	sort.Strings(files)
	var parts []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return parts, nil
}
