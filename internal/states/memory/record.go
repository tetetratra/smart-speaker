package memory

import "time"

// Record is a persisted memory item.
// 旧形式の JSON に残っている embedding は読み込み時に無視され、次回保存時に消える。
type Record struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertInput struct {
	Content string
	Tags    []string
}

type UpdateInput struct {
	Content string
	Tags    []string
}

type UpsertResult struct {
	Created bool
}

func cloneRecord(record Record) Record {
	record.Tags = cloneStrings(record.Tags)
	return record
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
