package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	DefaultOpenAIEndpoint      = "https://api.openai.com/v1/responses"
	DefaultMaxMemoryOperations = 5
)

type OpenAIClientConfig struct {
	APIKey        string
	Model         string
	Endpoint      string
	HTTPClient    *http.Client
	MaxOperations int
}

type OpenAIClient struct {
	apiKey        string
	model         string
	endpoint      string
	client        *http.Client
	maxOperations int
	// now は今の日時。古くなった記憶を判断する基準として渡す。テストで差し替える。
	now func() time.Time
}

const (
	OperationAdd    = "add"
	OperationUpdate = "update"
	OperationDelete = "delete"
)

// Operation は既存メモリに対する1件の変更操作を表す。
// Action が add のとき TargetID は空、delete のとき Content は空になる。
type Operation struct {
	Action   string `json:"action"`
	TargetID string `json:"target_id"`
	Content  string `json:"content"`
}

// ExistingMemory は抽出時に LLM へ渡す保存済みメモリ。
// ID には store の ID ではなく、LLM が書き写しやすい短い参照番号を入れる。
type ExistingMemory struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	// UpdatedAt は日本時間のRFC3339。古くなって不要になった記憶を見分けるために渡す。
	UpdatedAt string `json:"updated_at,omitempty"`
}

func NewOpenAIClient(cfg OpenAIClientConfig) (*OpenAIClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("memory openai: API key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("memory openai: model is required")
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultOpenAIEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("memory openai: endpoint is invalid")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	maxOperations := cfg.MaxOperations
	if maxOperations <= 0 {
		maxOperations = DefaultMaxMemoryOperations
	}
	return &OpenAIClient{
		apiKey:        apiKey,
		model:         model,
		endpoint:      endpoint,
		client:        httpClient,
		maxOperations: maxOperations,
		now:           time.Now,
	}, nil
}

func (c *OpenAIClient) CreateOperations(ctx context.Context, records []types.ConversationRecord, existing []ExistingMemory) ([]Operation, error) {
	conversation := conversationTurns(records)
	if len(conversation) == 0 {
		return nil, nil
	}
	inputJSON, err := encodeOperationInput(c.now(), conversation, existing)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model": c.model,
		"input": []map[string]any{
			{"role": "system", "content": memoryOperationInstructions},
			{"role": "user", "content": inputJSON},
		},
		"text": memoryOperationTextFormat(c.maxOperations),
	}
	text, err := c.createStructuredText(ctx, payload)
	if err != nil {
		return nil, err
	}
	var decoded operationResponse
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("memory openai: parse operations: %w", err)
	}
	return normalizeOperations(decoded.Operations, c.maxOperations), nil
}

func (c *OpenAIClient) createStructuredText(ctx context.Context, payload map[string]any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("memory openai: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return readOpenAIResponseBody(resp.Body)
}

// RetentionCriteria は、長期記憶に残す価値があるかの判断基準。
// 古い会話の抽出と、会話中にメモリを操作するツール（internal/tools/functions/memory）で共有する。
const RetentionCriteria = `「数週間後の別の会話で、これを知っているとアシスタントの応答が明確に良くなるか」で判断してください。
迷ったら保存しないでください。変更が不要なら operations を空配列にしてください。多くの会話では空配列が正解です（下の「不要になった記憶の削除」に当たる記憶があるときを除きます）。

保存してよいもの:
- ユーザー本人が話した、長く変わらないプロフィール（住んでいる地域、仕事、経歴、家族など）
- 好き嫌いや関心（食べ物、趣味、好きな作品など）
- 繰り返す習慣や生活リズム（家事の周期、就寝時刻など）
- アシスタントの応答の仕方についての希望（説明の長さ、話すか表示するかなど）

保存しないもの:
- アシスタントやシステムが既に分かること: 日本語で話す、音声で話しかける、タイムゾーン、アシスタントに連携済みのサービス・家電やそのシーン名
- その場限りの依頼や出来事: 天気を確認した、今日どこへ行く、一度だけ言ったこと
- 「〜を頼むことがある」「〜をよく尋ねる」のような利用傾向
- アシスタント自体の動作確認・開発・テストの話（音の回り込み、マイク、端末での検証など）
- アシスタントの発言だけを根拠にした推測。ユーザーの発言を根拠にしてください
- 会話ログの生コピーや秘密情報らしき内容`

const memoryOperationInstructions = `あなたはスマートスピーカーの長期記憶を管理します。
入力 JSON の now（今の日時）、conversation（ユーザーとアシスタントの会話）、existing_memories（保存済みの長期記憶。updated_at は最後に書き換えた日時）を読み、長期記憶に対する変更操作だけを operations として出力してください。

## 保存する価値の判断基準
` + RetentionCriteria + `

ユーザーが「覚えておいて」と明示した内容は、上の基準にかかわらず保存してください。ただしパスワード、暗証番号、カード番号などの秘密情報は保存しないでください。「忘れて」と言われた内容は delete してください。
アシスタントが「覚えておこうか？」などと保存を提案し、ユーザーが断った内容は保存しないでください。

## 操作
- add: existing_memories にない新しい事実を追加します。target_id は空文字にしてください。
- update: existing_memories の記憶が変わった・補足されたとき、target_id の記憶を content で置き換えます。同じ話題は add せず update で1件にまとめてください。好みや状況が別のものに変わった場合も、delete ではなく新しい内容への update にしてください。
- delete: ユーザーが「もう当てはまらない」「間違っている」「忘れて」と言い、置き換える内容がない記憶を target_id で削除します。下の「不要になった記憶の削除」に当たる記憶も削除します。content は空文字にしてください。
- existing_memories と同じ内容を言い換えて add しないでください。
- target_id には existing_memories の id をそのまま使ってください。1つの記憶に対する操作は1件までにしてください。

## 不要になった記憶の削除
existing_memories には、ユーザーに頼まれて保存した、その場限りの記憶も含まれます（例: 特定の日の天気、その日の予定、一度きりの用事）。
今回の会話で話題に出ていなくても、existing_memories を1件ずつ見て、次に当たる記憶は delete してください。
- 特定の日付に結びついた天気・予定・用事・出来事で、その日付が now の日付より前のもの（例: now が 2026-11-09 なら「2026-11-08の天気は晴れ」「2026-10-25に歯医者の予約がある」は削除）
- 日付が書かれていないその場限りの情報で、updated_at から3日以上たったもの
次の記憶は削除しないでください。
- 日付が now の日付と同じか、それより後のもの（例: 今日の天気、来月の旅行の予定）
- 誕生日や記念日のように、毎年くり返す日付
- 上の「保存してよいもの」に当たる記憶（長く変わらないプロフィール、好き嫌い、習慣、応答の仕方の希望）。updated_at が古くても削除しません

## content の書き方
- content は1件につき1つの話題を、「ユーザーは」で始まる短い自然文で書いてください。
- 「今日」「来月」などの相対的な日時は、会話の created_at を基準に絶対日付で書いてください。
- 将来の予定は、その日付を過ぎても意味がある場合だけ保存してください。

## 出力例
- 新しい好みを聞いた場合:
  {"action": "add", "target_id": "", "content": "ユーザーは雨の日に頭痛が出やすい"}
- 引っ越したと聞いた場合（既存: {"id": "1", "content": "ユーザーは大岡山に住んでいる"}）:
  {"action": "update", "target_id": "1", "content": "ユーザーは横浜に住んでいる"}
- 好みが変わったと聞いた場合（既存: {"id": "2", "content": "ユーザーは朝にコーヒーを飲む"}）:
  {"action": "update", "target_id": "2", "content": "ユーザーは朝に紅茶を飲む"}
- 「もう猫は飼っていない」と聞いた場合（既存: {"id": "3", "content": "ユーザーは猫を飼っている"}）:
  {"action": "delete", "target_id": "3", "content": ""}
- 「エアコンつけて」「今日の天気は？」だけの会話:
  operations: []`

// 会話の日時はサーバーのタイムゾーンに依存せず日本時間で渡す。
var japanLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

type conversationTurn struct {
	Role      string `json:"role"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at,omitempty"`
}

type operationInput struct {
	Now              string             `json:"now"`
	ExistingMemories []ExistingMemory   `json:"existing_memories"`
	Conversation     []conversationTurn `json:"conversation"`
}

// conversationTurns はユーザーとアシスタントの発話だけを残す。
// ツール呼び出し・結果や system メッセージは、ユーザー自身の事実ではないため抽出に渡さない。
func conversationTurns(records []types.ConversationRecord) []conversationTurn {
	turns := make([]conversationTurn, 0, len(records))
	for _, record := range records {
		role := strings.TrimSpace(record.Role)
		if role != types.RoleUser && role != types.RoleAgent {
			continue
		}
		text := strings.TrimSpace(record.Text)
		if text == "" {
			continue
		}
		turn := conversationTurn{Role: role, Text: text}
		if !record.CreatedAt.IsZero() {
			turn.CreatedAt = record.CreatedAt.In(japanLocation).Format(time.RFC3339)
		}
		turns = append(turns, turn)
	}
	return turns
}

func encodeOperationInput(now time.Time, conversation []conversationTurn, existing []ExistingMemory) (string, error) {
	if existing == nil {
		existing = []ExistingMemory{}
	}
	raw, err := json.Marshal(operationInput{Now: now.In(japanLocation).Format(time.RFC3339), ExistingMemories: existing, Conversation: conversation})
	if err != nil {
		return "", fmt.Errorf("memory openai: encode operation input: %w", err)
	}
	return string(raw), nil
}

func memoryOperationTextFormat(maxOperations int) map[string]any {
	return map[string]any{
		"format": map[string]any{
			"type":   "json_schema",
			"name":   "memory_operations",
			"strict": true,
			"schema": memoryOperationSchema(maxOperations),
		},
	}
}

func memoryOperationSchema(maxOperations int) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operations": map[string]any{
				"type":     "array",
				"maxItems": maxOperations,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type": "string",
							"enum": []string{OperationAdd, OperationUpdate, OperationDelete},
						},
						"target_id": map[string]any{"type": "string"},
						"content":   map[string]any{"type": "string"},
					},
					"required":             []string{"action", "target_id", "content"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"operations"},
		"additionalProperties": false,
	}
}

type operationResponse struct {
	Operations []Operation `json:"operations"`
}

type openAIResponseBody struct {
	OutputText string             `json:"output_text"`
	Output     []openAIOutputItem `json:"output"`
	Error      *openAIError       `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
}

type openAIOutputItem struct {
	Content []openAIOutputContent `json:"content"`
}

type openAIOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func readOpenAIResponseBody(r io.Reader) (string, error) {
	var body openAIResponseBody
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return "", err
	}
	if body.Error != nil {
		msg := strings.TrimSpace(body.Error.Message)
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("memory openai: response failed: %s", msg)
	}
	if text := strings.TrimSpace(body.OutputText); text != "" {
		return text, nil
	}
	for _, item := range body.Output {
		for _, content := range item.Content {
			if content.Type != "output_text" {
				continue
			}
			if text := strings.TrimSpace(content.Text); text != "" {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("memory openai: response text is empty")
}

// normalizeOperations は action ごとの必須項目を満たさない操作を捨て、最大件数で切り詰める。
func normalizeOperations(operations []Operation, maxOperations int) []Operation {
	if maxOperations <= 0 {
		maxOperations = DefaultMaxMemoryOperations
	}
	out := make([]Operation, 0, len(operations))
	for _, operation := range operations {
		action := strings.ToLower(strings.TrimSpace(operation.Action))
		targetID := strings.TrimSpace(operation.TargetID)
		content := strings.TrimSpace(operation.Content)
		var normalized Operation
		switch {
		case action == OperationAdd && content != "":
			normalized = Operation{Action: action, Content: content}
		case action == OperationUpdate && content != "" && targetID != "":
			normalized = Operation{Action: action, TargetID: targetID, Content: content}
		case action == OperationDelete && targetID != "":
			normalized = Operation{Action: action, TargetID: targetID}
		default:
			continue
		}
		out = append(out, normalized)
		if len(out) >= maxOperations {
			break
		}
	}
	return out
}
