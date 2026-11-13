package automations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

const directScriptExample = `toolcall("set_whiteboard", {content: "予約を実行しました"});`

const remindScriptExample = `remind("時間になったので、休憩するよう伝えてください");`

const temperatureScriptExample = `const result = toolcall("hub2_get_temperature", {});
const temperature = Number(result.temperature);
if (Number.isFinite(temperature) && temperature >= 28) {
  toolcall("set_whiteboard", {content: "室温は" + temperature + "℃です"});
  remind("室温が" + temperature + "℃です。暑さに注意するよう伝えてください");
}`

type Tool struct {
	Service   *automations.Service
	Operation string
}

func (t *Tool) Name() string { return t.Operation }
func (t *Tool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
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
	switch t.Operation {
	case "register_job", "update_job":
		description = "ユーザーが依頼した処理をJavaScriptの予約ジョブとして永続化します。JSへ公開されるアプリの関数はtoolcallとremindだけです。switchbot_execute_sceneなどのツール名そのものはJSの関数ではありません。必ずtoolcallへツール名の文字列と引数オブジェクトを渡します。toolcall(name, args)は登録済みツールを同期で直接実行して結果を返します。失敗はerror付きオブジェクトで返します。非同期ツールは受付結果だけを返し、後続結果も履歴に記録するだけで発話しません。remind(text)はAIに応答させたいときだけ使います。Node.jsのAPIやPromiseによる非同期処理は使えません。scheduleはafter_seconds（指定秒後）、at（時差付きRFC3339日時）、cron（分・時・日・月・曜日の5項目）の1つを指定します。タイムゾーンの既定はAsia/Tokyo。ユーザーに知らせる必要がない処理はtoolcallだけを使い、remindやwait_for_userは呼びません。現在日時から予定を計算し、登録後に次回日時を確認してください。いつ実行するか（「1分後に」「毎朝8時に」など）はscheduleだけで表し、nameとscriptには含めないでください。日時指定の予約は実行後に自動で削除されます。"
		description += fmt.Sprintf("\n\nscriptの記述例（時刻と頻度はscheduleに指定）:\n例1: ツールを直接呼び、発話せず画面へ残す。\n%s\n\n例2: AIに応答させるリマインダー。\n%s\n\n例3: Hub2が利用可能な場合、温度取得と画面表示の複数ツールを使い、28℃以上のときだけリマインダーで知らせる。temperatureは文字列なのでNumberで数値へ変換する。\n%s", directScriptExample, remindScriptExample, temperatureScriptExample)

		props = map[string]any{"name": map[string]any{"type": "string", "description": "予約の内容を表す短い名前。時刻・頻度は含めない。例: 室温を知らせる"}, "script": map[string]any{"type": "string", "description": "予定時刻に実行するJavaScript。ツール説明の3例を参考にtoolcallとremindを使う。ツール名を直接JSの関数として呼ばない。時刻や頻度はscheduleに指定する。"}, "schedule": map[string]any{"type": "object", "properties": map[string]any{"after_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 31536000}, "at": map[string]any{"type": "string", "description": "例: 2026-11-10T08:00:00+09:00"}, "cron": map[string]any{"type": "string", "description": "例: 平日8時は0 8 * * 1-5"}, "timezone": map[string]any{"type": "string"}}, "additionalProperties": false}}
		required = []string{"name", "script", "schedule"}
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
