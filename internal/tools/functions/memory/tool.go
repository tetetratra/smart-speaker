// Package memory は、会話の中から長期メモリを確認・追加・変更・削除するツールを提供する。
package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	memoryhook "github.com/tetetratra/smart-speaker/internal/hooks/memory"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

// Operations は登録するツール名。書き込みの3つは、ユーザー本人の発話がないと呼べない（realtimeの userOnlyTools）。
var Operations = []string{"list_memories", "add_memory", "update_memory", "delete_memory"}

// memoryRole は、長期メモリが何のためのものかの説明。全ツールの説明の冒頭に添える。
const memoryRole = "長期メモリは、次回以降の別の会話でもあなたが参照する、ユーザーについての情報です。会話の冒頭に毎回渡されます。"

// ユーザーの依頼か了承があるときだけ書き換えるよう、書き込みツールの説明に共通で添える。
// 提案してよいものの基準は、会話終了時の自動抽出と同じものを使う。
const writePolicy = "呼び出してよいのは、ユーザーが「覚えて」「変えて」「忘れて」などとはっきり頼んだときか、あなたの提案にユーザーが了承したときだけです。" +
	"はっきり頼まれたときは、その場限りの内容でも、聞き返さずにそのまま呼び出します（その場限りの内容は、不要になると会話終了時に自動で削除されます）。ただしパスワード、暗証番号、カード番号などの秘密情報は、頼まれても保存しません。" +
	"ユーザーが覚えておけそうな事実を話しただけのときは呼び出さず、次の基準で残す価値があるときだけ、残す内容を口頭で伝えて「覚えておこうか？」と提案し、返事を待ってください。\n" +
	memoryhook.RetentionCriteria + "\n" +
	"この基準は、あなたから提案するかを決めるときだけに使います。ユーザーにはっきり頼まれたときは、基準で「保存しないもの」に当たる内容（その日の天気など）でも断らずに保存してください。"

type Tool struct {
	Store     *memorystate.Store
	Operation string
}

func (t *Tool) Name() string { return t.Operation }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	var req struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("引数を読み取れません: %w", err)
	}
	switch t.Operation {
	case "list_memories":
		return map[string]any{"memories": memoryItems(t.Store.Snapshot())}, nil
	case "add_memory":
		record, result, err := t.Store.Upsert(memorystate.UpsertInput{Content: req.Content})
		if err != nil {
			return nil, describeError(err)
		}
		// 同じ本文が既にあるときは追加せず、既存のメモリを返す。
		return map[string]any{"memory": memoryItem(record), "created": result.Created}, nil
	case "update_memory":
		record, err := t.Store.Update(req.ID, memorystate.UpdateInput{Content: req.Content})
		if err != nil {
			return nil, describeError(err)
		}
		return map[string]any{"memory": memoryItem(record), "note": "この会話でも、今後は書き換えた内容を前提にしてください。"}, nil
	case "delete_memory":
		// 何を消したかを口頭で伝えられるよう、消す前の本文を返す。
		deleted, ok := findRecord(t.Store.Snapshot(), req.ID)
		if !ok {
			return nil, describeError(memorystate.ErrRecordNotFound)
		}
		if err := t.Store.Delete(deleted.ID); err != nil {
			return nil, describeError(err)
		}
		// 会話の冒頭に渡したメモリの文脈は、接続し直すまで変わらないため、使わないよう結果で伝える。
		return map[string]any{"deleted": memoryItem(deleted), "note": "このメモリは削除しました。この会話でも、今後はこの内容を前提にしないでください。"}, nil
	default:
		return nil, fmt.Errorf("未対応のメモリ操作: %s", t.Operation)
	}
}

func (t *Tool) Definition() map[string]any {
	idProp := map[string]any{"type": "string", "description": "list_memoriesで確かめたメモリのID"}
	// 会話終了時の自動抽出（internal/hooks/memory）と書き方を揃え、言い換えの重複を防ぐ。
	contentProp := map[string]any{"type": "string", "description": "メモリの本文。1件に1つの話題を、「ユーザーは」か「ユーザーの」で始まる短い1文で書く。「今日」「明日」などの相対的な日付だけを、2026-05-03 のような年付きの日付に直す。誕生日や記念日のように毎年くり返す日付には、年を付けない。例: ユーザーの長女の誕生日は5月3日、ユーザーの2026-05-03の天気は晴れ"}
	props := map[string]any{}
	required := []string{}
	description := ""
	switch t.Operation {
	case "list_memories":
		description = memoryRole + "保存済みの長期メモリを、ID・本文・更新日時つきで一覧します。メモリの変更・削除の前に、対象のIDを確かめるために使います。"
	case "add_memory":
		description = memoryRole + "長期メモリを1件追加します。同じ話題のメモリが既にあるときは、追加せずupdate_memoryで書き換えてください。同じ本文が既にあれば追加せず、既存のメモリを返します（created=false）。" + writePolicy
		props = map[string]any{"content": contentProp}
		required = []string{"content"}
	case "update_memory":
		description = memoryRole + "長期メモリ1件の本文を書き換えます。先にlist_memoriesで対象のIDを確かめてください。" + writePolicy
		props = map[string]any{"id": idProp, "content": contentProp}
		required = []string{"id", "content"}
	case "delete_memory":
		description = memoryRole + "長期メモリを1件削除し、削除した本文を返します。先にlist_memoriesで対象のIDを確かめてください。" + writePolicy
		props = map[string]any{"id": idProp}
		required = []string{"id"}
	}
	return map[string]any{
		"type":        "function",
		"name":        t.Name(),
		"description": description,
		"parameters":  map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false},
	}
}

func memoryItems(records []memorystate.Record) []map[string]any {
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, memoryItem(record))
	}
	return items
}

func memoryItem(record memorystate.Record) map[string]any {
	return map[string]any{"id": record.ID, "content": record.Content, "updated_at": record.UpdatedAt}
}

func findRecord(records []memorystate.Record, id string) (memorystate.Record, bool) {
	id = strings.TrimSpace(id)
	for _, record := range records {
		if id != "" && record.ID == id {
			return record, true
		}
	}
	return memorystate.Record{}, false
}

// describeError は、AIが次の行動を選べるように、よくある失敗を言い換える。
func describeError(err error) error {
	switch {
	case errors.Is(err, memorystate.ErrEmptyContent):
		return fmt.Errorf("contentが空です")
	case errors.Is(err, memorystate.ErrRecordNotFound):
		return fmt.Errorf("指定したIDのメモリがありません。list_memoriesでIDを確かめてください")
	default:
		return err
	}
}

var _ tools.Handler = (*Tool)(nil)
