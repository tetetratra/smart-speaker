package state

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GetDiaryContent returns diary content for system prompt injection.
func GetDiaryContent() string {
	entries, err := os.ReadDir(filepath.Join("tmp", "diary"))
	if err != nil {
		return ""
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".md") {
			files = append(files, filepath.Join("tmp", "diary", name))
		}
	}
	if len(files) == 0 {
		return ""
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
	return strings.Join(parts, "\n\n")
}
