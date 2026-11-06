package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultInstructions = `あなたはスマートスピーカーを補助するテキスト専用サブエージェントです。
ユーザーと直接会話するのではなく、間にいるRealtime APIの会話エージェントが通訳・仲介役として依頼を渡し、あなたの回答をユーザー向けに伝えます。
複雑な質問や作業を慎重に検討し、結果を日本語のsummaryとdetailsに分けてください。
summaryは口頭で伝える要点だけを1〜3文、200文字以内にまとめます。Realtimeエージェントはこの要点だけを読み上げます。
detailsはホワイトボードに書き残す補足です。通常5〜10行以内、必要なければさらに短くし、1要素を改行のない1行、各120文字以内にします。長大な説明やsummaryの繰り返しは避けます。
不明点・推測・制約は明示してください。外部ツールや実環境を操作したと装わないでください。`

type Answer struct {
	ID      string   `json:"-"`
	Summary string   `json:"summary"`
	Details []string `json:"details"`
}
type Client interface {
	Respond(context.Context, string, string) (Answer, error)
}
type OpenAIClient struct {
	APIKey, Model, Endpoint string
	HTTP                    *http.Client
}

func (c *OpenAIClient) Respond(ctx context.Context, input, previous string) (Answer, error) {
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/responses"
	}
	payload := map[string]any{
		"model": c.Model, "input": input, "store": true,
		"reasoning": map[string]any{"effort": "high"}, "max_output_tokens": 8192,
		"instructions": defaultInstructions,
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "subagent_answer", "strict": true,
			"schema": map[string]any{"type": "object", "properties": map[string]any{
				"summary": map[string]any{"type": "string"},
				"details": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 10},
			}, "required": []string{"summary", "details"}, "additionalProperties": false},
		}},
	}

	if previous != "" {
		payload["previous_response_id"] = previous
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Answer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Answer{}, fmt.Errorf("サブエージェントAPIが失敗しました (HTTP %d)", resp.StatusCode)
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Output []struct {
			Type    string                        `json:"type"`
			Content []struct{ Type, Text string } `json:"content"`
		} `json:"output"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return Answer{}, err
	}
	if result.Status != "completed" || result.ID == "" {
		return Answer{}, fmt.Errorf("サブエージェントの応答が完了しませんでした")
	}
	var texts []string
	for _, item := range result.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				texts = append(texts, content.Text)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" {
		return Answer{}, fmt.Errorf("サブエージェントの回答が空でした")
	}
	var answer Answer
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return Answer{}, fmt.Errorf("サブエージェントの回答形式が不正です")
	}
	answer.Summary = strings.TrimSpace(answer.Summary)
	if answer.Summary == "" || len([]rune(answer.Summary)) > 200 || len(answer.Details) > 10 {
		return Answer{}, fmt.Errorf("サブエージェントの要点または詳細が長すぎるか空です")
	}
	for i, line := range answer.Details {
		line = strings.TrimSpace(line)
		if line == "" || strings.ContainsAny(line, "\r\n") || len([]rune(line)) > 120 {
			return Answer{}, fmt.Errorf("サブエージェントの詳細は1項目120文字以内の1行で指定してください")
		}
		answer.Details[i] = line
	}
	answer.ID = result.ID
	return answer, nil
}
