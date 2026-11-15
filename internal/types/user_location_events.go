package types

import "time"

// EventUserLocationChanged は、位置情報 API で受け取った場所が直前の場所から変わったことを知らせる。
// 同じ場所が続けて届いたときは流さない。サーバー起動後の最初の1件は、直前の場所がないので変化として扱う。
type EventUserLocationChanged struct {
	Place string `json:"place"`
	// PreviousPlace は変わる前の場所。サーバー起動後の最初の1件では nil（JSON では null）。
	PreviousPlace *string   `json:"previous_place"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (EventUserLocationChanged) isEvent() {}
