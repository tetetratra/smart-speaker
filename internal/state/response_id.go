package state

import (
	"os"
	"path/filepath"
	"strings"
)

const responseIDFile = "tmp/response_id.txt"

// GetResponseID reads the persisted response ID.
func GetResponseID() string {
	data, err := os.ReadFile(responseIDFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SetResponseID persists the response ID.
func SetResponseID(responseID string) {
	id := strings.TrimSpace(responseID)
	if id == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(responseIDFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(responseIDFile, []byte(id+"\n"), 0o644)
}

// ClearResponseID removes the persisted response ID.
func ClearResponseID() {
	_ = os.Remove(responseIDFile)
}
