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
		ID          string               `json:"id"`
		Name        string               `json:"name"`
		Instruction string               `json:"instruction"`
		Schedule    automations.Schedule `json:"schedule"`
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
	spec := automations.Spec{Name: req.Name, Instruction: req.Instruction, Schedule: req.Schedule}
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
	switch t.Operation {
	case "register_job", "update_job":
		description = "ユーザーが依頼した自然言語の指示を予約ジョブとして永続化します。実行時刻ごとにAIが指示を解釈してツールを実行します。scriptやコードは登録しません。scheduleはafter_seconds（指定秒後）、at（時差付きRFC3339日時）、cron（分・時・日・月・曜日の5項目）の1つを指定します。タイムゾーンの既定はAsia/Tokyo。実行結果をユーザーに知らせる必要がない処理（無言でのモード変更など）は、instructionの末尾に「この作業は報告不要なので、終わったらwait_for_userの呼び出しだけを行うこと」と書きます。現在日時から予定を計算し、登録後に次回日時を確認してください。いつ実行するか（「1分後に」「毎朝8時に」など）はscheduleだけで表し、nameとinstructionには含めないでください。日時指定の予約は実行後に自動で削除されます。"
		props = map[string]any{"name": map[string]any{"type": "string", "description": "予約の内容を表す短い名前。時刻・頻度は含めない。例: 室温を知らせる"}, "instruction": map[string]any{"type": "string", "description": "予定時刻になった時点でAIが実行する内容だけを書いた自然言語の指示。時刻・頻度・「〜後に」などの表現は含めない。例: Hub2の温度を取得して、現在の室温を音声で伝える"}, "schedule": map[string]any{"type": "object", "properties": map[string]any{"after_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 31536000}, "at": map[string]any{"type": "string", "description": "例: 2026-11-10T08:00:00+09:00"}, "cron": map[string]any{"type": "string", "description": "例: 平日8時は0 8 * * 1-5"}, "timezone": map[string]any{"type": "string"}}, "additionalProperties": false}}
		required = []string{"name", "instruction", "schedule"}
		if t.Operation == "update_job" {
			props["id"] = map[string]any{"type": "string"}
			required = append(required, "id")
		}
	case "list_jobs":
		description = "登録済みの予約ジョブとID・次回日時・状態を確認します。"
	case "delete_job":
		description = "ユーザーが削除を依頼した予約ジョブを削除します。"
		props = map[string]any{"id": map[string]any{"type": "string"}}
		required = []string{"id"}
	}
	return map[string]any{"type": "function", "name": t.Name(), "description": description, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}
}

var _ tools.Handler = (*Tool)(nil)
