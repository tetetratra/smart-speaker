package automations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

type Tool struct {
	Service   *automations.Service
	Operation string
}

func (t *Tool) Name() string { return t.Operation }
func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	var req struct {
		ID       string               `json:"id"`
		Name     string               `json:"name"`
		Script   string               `json:"script"`
		Schedule automations.Schedule `json:"schedule"`
	}
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&req); err != nil {
		return nil, err
	}
	spec := automations.Spec{Name: req.Name, Script: req.Script, Schedule: req.Schedule}
	switch t.Operation {
	case "list_jobs":
		return map[string]any{"jobs": t.Service.List()}, nil
	case "register_job":
		job, err := t.Service.Create(spec, time.Now())
		return map[string]any{"job": job}, err
	case "update_job":
		job, err := t.Service.Update(req.ID, spec, time.Now())
		return map[string]any{"job": job}, err
	case "delete_job":
		err := t.Service.Delete(req.ID)
		return map[string]any{"deleted": err == nil}, err
	default:
		return nil, fmt.Errorf("未対応の予約操作")
	}
}
func (t *Tool) Definition() map[string]any {
	props := map[string]any{}
	required := []string{}
	description := ""
	mode := tools.ToolModeWrite
	switch t.Operation {
	case "register_job", "update_job":
		description = "ユーザーが依頼した処理を予約ジョブとして永続化します。予定時刻になるとscriptのJavaScriptが実行されます。scriptで使える関数は次の2つです。" +
			"remind(text): textをあなた（AI）に渡し、音声で応答させます。天気や予定を調べて伝える、起こすなど、考えて話す必要がある処理に使います。" +
			"toolcall(name, args): あなたを通さずにツールを直接呼び、結果のオブジェクトを返します。nameとargsは、あなたが使えるツールの名前・引数と同じです。照明やモードの切り替えなど、決まった操作に使います。応答や読み上げは起きませんが、呼び出しと結果は会話履歴に残ります。ユーザー本人の了承が必要なツール（ask_claude_codeやメモリの変更など）は呼べません。" +
			"例: remind(\"今日の天気を教えて\") / toolcall(\"set_wake_mode\", {required: true})。toolcallの結果を変数に受けてifで分岐し、条件を満たすときだけremindすることもできます。" +
			"決まった操作だけで済むならtoolcallを使い、remindは話す必要があるときだけ使ってください。" +
			"scheduleはafter_seconds（指定秒後）、at（時差付きRFC3339日時）、cron（分・時・日・月・曜日の5項目）の1つを指定します。タイムゾーンの既定はAsia/Tokyo。現在日時から予定を計算し、登録後に次回日時を確認してください。いつ実行するか（「1分後に」「毎朝8時に」など）はscheduleだけで表し、nameとscript（remindに渡す文も含む）には含めないでください。日時指定の予約は実行後に自動で削除されます。"
		props = map[string]any{"name": map[string]any{"type": "string", "description": "予約の内容を表す短い名前。時刻・頻度は含めない。例: 室温を知らせる"}, "script": map[string]any{"type": "string", "description": "予定時刻に実行するJavaScript。remindとtoolcallを使う。時刻・頻度・「〜後に」などの表現は含めない。"}, "schedule": map[string]any{"type": "object", "properties": map[string]any{"after_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 31536000}, "at": map[string]any{"type": "string", "description": "例: 2026-11-10T08:00:00+09:00"}, "cron": map[string]any{"type": "string", "description": "例: 平日8時は0 8 * * 1-5"}, "timezone": map[string]any{"type": "string"}}, "additionalProperties": false}}
		required = []string{"name", "script", "schedule"}
		if t.Operation == "update_job" {
			props["id"] = map[string]any{"type": "string"}
			required = append(required, "id")
		}
	case "list_jobs":
		description = "登録済みの予約ジョブとID・スクリプト・次回日時・状態を確認します。"
		mode = tools.ToolModeRead
	case "delete_job":
		description = "ユーザーが削除を依頼した予約ジョブを削除します。"
		props = map[string]any{"id": map[string]any{"type": "string"}}
		required = []string{"id"}
	}
	return tools.DefinitionWithMode(map[string]any{"type": "function", "name": t.Name(), "description": description, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}, mode)
}

var _ tools.Handler = (*Tool)(nil)
