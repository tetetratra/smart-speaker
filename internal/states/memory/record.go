package memory

import "time"

// Record is a persisted memory item.
// 旧形式の JSON に残っている embedding と tags は読み込み時に無視され、次回保存時に消える。
type Record struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertInput struct {
	Content string
}

type UpdateInput struct {
	Content string
}

type UpsertResult struct {
	Created bool
}
