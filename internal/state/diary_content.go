package state

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GetDiaryContent returns diary content for system prompt injection.
func GetDiaryContent() string {
	path, err := ensureDiaryFile()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func AppendDiaryEntry(when time.Time, content string) (string, error) {
	path, err := ensureDiaryFile()
	if err != nil {
		return "", err
	}
	header := "# " + when.Format("2006-01-02 15:04")
	body := header + "\n" + strings.TrimLeft(content, "\n")
	body = strings.TrimRight(body, "\n") + "\n"

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

const (
	diaryDir       = "data"
	diaryPath      = "data/diary.md"
	legacyDiaryDir = "tmp/diary"
)

func ensureDiaryFile() (string, error) {
	if _, err := os.Stat(diaryPath); err == nil {
		return diaryPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(diaryDir, 0o755); err != nil {
		return "", err
	}
	entries, err := readLegacyDiaryEntries()
	if err != nil {
		return "", err
	}
	content := strings.Join(entries, "\n\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(diaryPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return diaryPath, nil
}

func readLegacyDiaryEntries() ([]string, error) {
	entries, err := os.ReadDir(legacyDiaryDir)
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
			files = append(files, filepath.Join(legacyDiaryDir, name))
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
