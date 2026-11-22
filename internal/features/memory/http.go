package memory

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

type memoryListResponse struct {
	Memories []memoryListItem `json:"memories"`
}

type memoryListItem struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type memoryWriteRequest struct {
	Content string `json:"content"`
}

type memoryWriteResponse struct {
	Memory memoryListItem `json:"memory"`
}

// RegisterHTTP は、画面から長期メモリを一覧・追加・変更・削除する API を mux に登録する。
func RegisterHTTP(mux *http.ServeMux, store *Store) {
	registerHTTP(mux, store, &manualMemoryService{store: store})
}

func registerHTTP(mux *http.ServeMux, memoryStore *Store, writer manualMemoryWriter) {
	mux.HandleFunc("/api/memories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleMemoryList(w, memoryStore)
		case http.MethodPost:
			handleMemoryCreate(w, r, writer)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/memories/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(path.Clean(r.URL.Path), "/api/memories/")
		if id == "." || id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			handleMemoryUpdate(w, r, writer, id)
		case http.MethodDelete:
			handleMemoryDelete(w, writer, id)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func handleMemoryList(w http.ResponseWriter, memoryStore *Store) {
	if memoryStore == nil {
		http.Error(w, "memory store is unavailable", http.StatusInternalServerError)
		return
	}

	records := memoryStore.Snapshot()
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	items := make([]memoryListItem, 0, len(records))
	for _, record := range records {
		items = append(items, memoryItemFromRecord(record))
	}

	writeJSON(w, http.StatusOK, memoryListResponse{Memories: items})
}

func handleMemoryCreate(w http.ResponseWriter, r *http.Request, writer manualMemoryWriter) {
	if writer == nil {
		http.Error(w, "memory writer is unavailable", http.StatusInternalServerError)
		return
	}
	req, ok := decodeMemoryWriteRequest(w, r)
	if !ok {
		return
	}
	record, _, err := writer.Create(r.Context(), req.Content)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, memoryWriteResponse{Memory: memoryItemFromRecord(record)})
}

func handleMemoryUpdate(w http.ResponseWriter, r *http.Request, writer manualMemoryWriter, id string) {
	if writer == nil {
		http.Error(w, "memory writer is unavailable", http.StatusInternalServerError)
		return
	}
	req, ok := decodeMemoryWriteRequest(w, r)
	if !ok {
		return
	}
	record, err := writer.Update(r.Context(), id, req.Content)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryWriteResponse{Memory: memoryItemFromRecord(record)})
}

func handleMemoryDelete(w http.ResponseWriter, writer manualMemoryWriter, id string) {
	if writer == nil {
		http.Error(w, "memory writer is unavailable", http.StatusInternalServerError)
		return
	}
	if err := writer.Delete(id); err != nil {
		writeMemoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeMemoryWriteRequest(w http.ResponseWriter, r *http.Request) (memoryWriteRequest, bool) {
	var req memoryWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid memory request", http.StatusBadRequest)
		return memoryWriteRequest{}, false
	}
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, ErrEmptyContent.Error(), http.StatusBadRequest)
		return memoryWriteRequest{}, false
	}
	return req, true
}

func writeMemoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmptyContent):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrRecordNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func memoryItemFromRecord(record Record) memoryListItem {
	return memoryListItem{
		ID:        record.ID,
		Content:   record.Content,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("http: encode response: %v", err)
	}
}
