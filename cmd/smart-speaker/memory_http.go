package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
)

type memoryListResponse struct {
	Memories []memoryListItem `json:"memories"`
}

type memoryListItem struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type memoryWriteRequest struct {
	Content string `json:"content"`
}

type memoryWriteResponse struct {
	Memory memoryListItem `json:"memory"`
}

func registerMemoryAPI(mux *http.ServeMux, memoryStore *memorystate.Store, writer manualMemoryWriter) {
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

func handleMemoryList(w http.ResponseWriter, memoryStore *memorystate.Store) {
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
		http.Error(w, memorystate.ErrEmptyContent.Error(), http.StatusBadRequest)
		return memoryWriteRequest{}, false
	}
	return req, true
}

func writeMemoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, memorystate.ErrEmptyContent):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, memorystate.ErrRecordNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func memoryItemFromRecord(record memorystate.Record) memoryListItem {
	tags := record.Tags
	if tags == nil {
		tags = []string{}
	}
	return memoryListItem{
		ID:        record.ID,
		Content:   record.Content,
		Tags:      tags,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}
