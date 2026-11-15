package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

const maxLocationRequestBytes = 4 * 1024

// userLocationSource は、場所の変化の記録の Source。予約ジョブの schedule.source に指定できる。
const userLocationSource = "user_location"

// registerLocationAPI は位置情報 API を登録する。deliver が nil でなければ、場所が変わったときに
// その記録を購読可能な記録として会話へ届ける（main では gateway の Deliver）。
func registerLocationAPI(mux *http.ServeMux, store *userlocation.Store, deliver func(context.Context, types.ConversationInput) error) {
	mux.HandleFunc("/api/location", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.Snapshot())
		case http.MethodPost:
			var req struct {
				Place string `json:"place"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxLocationRequestBytes)
			decoder := json.NewDecoder(r.Body)
			err := decoder.Decode(&req)
			if err == nil {
				// 本文を最後まで検証してから現在値を更新する。
				if trailingErr := decoder.Decode(new(any)); trailingErr != io.EOF {
					err = trailingErr
					if err == nil {
						err = errors.New("複数のJSON値は指定できません")
					}
				}
			}
			if err != nil {
				var sizeErr *http.MaxBytesError
				if errors.As(err, &sizeErr) {
					http.Error(w, "本文は4KiB以内で指定してください", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "有効なJSONオブジェクトを指定してください", http.StatusBadRequest)
				}
				return
			}
			if strings.TrimSpace(req.Place) == "" {
				http.Error(w, "placeには空でない文字列を指定してください", http.StatusBadRequest)
				return
			}
			current, previous := store.Update(req.Place)
			if deliver != nil && (previous == nil || previous.Place != current.Place) {
				if err := deliver(r.Context(), locationChangedInput(current, previous)); err != nil {
					log.Printf("位置情報: 場所の変化を会話へ届けられません: %v", err)
				}
			}
			writeJSON(w, http.StatusOK, current)
		default:
			w.Header().Set("Allow", "GET, POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

// locationChangedInput は、場所の変化を会話へ届ける記録を作る。
// AIは文脈に入れるだけで、その場では話さない（帰宅後の会話で「お仕事お疲れ様」などと言えるようにするため）。
// ホワイトボードには出す。
// サーバー起動後の最初の1件は、前の場所がないので previous_place を nil（JSON では null）にする。
func locationChangedInput(current userlocation.Record, previous *userlocation.Record) types.ConversationInput {
	text := "ユーザーの居場所が「" + current.Place + "」になりました。"
	var previousPlace any
	if previous != nil {
		text = "ユーザーの居場所が「" + previous.Place + "」から「" + current.Place + "」に変わりました。"
		previousPlace = previous.Place
	}
	record := types.ConversationRecord{
		ID:        "location/" + uuid.NewString(),
		Role:      types.RoleSystem,
		Source:    userLocationSource,
		Title:     current.Place,
		Text:      text,
		Metadata:  map[string]any{"place": current.Place, "previous_place": previousPlace},
		CreatedAt: current.UpdatedAt.In(time.Local),
		External:  true,
	}
	return types.ConversationInput{Record: record, Subscribable: true}
}
