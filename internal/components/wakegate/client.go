package wakegate

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Transcript struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}
type Transcriber interface {
	Transcribe(context.Context, []byte, int) (Transcript, error)
}
type HTTPClient struct {
	Endpoint string
	Client   *http.Client
}

func (c *HTTPClient) Transcribe(ctx context.Context, pcm []byte, rate int) (Transcript, error) {
	if rate < 8000 || rate > 96000 || len(pcm) > rate*2*30 {
		return Transcript{}, fmt.Errorf("ローカルSTTの音声形式が不正です")
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+len(pcm)))
	wav.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		_ = binary.Write(&wav, binary.LittleEndian, v)
	}
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(len(pcm)))
	wav.Write(pcm)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(wav.Bytes()))
	if err != nil {
		return Transcript{}, err
	}
	req.Header.Set("Content-Type", "audio/wav")
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Transcript{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Transcript{}, fmt.Errorf("ローカルSTT HTTP %d", resp.StatusCode)
	}
	var result Transcript
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&result); err != nil {
		return Transcript{}, err
	}
	if len([]rune(result.Text)) > 2048 || result.Confidence < 0 || result.Confidence > 1 {
		return Transcript{}, fmt.Errorf("ローカルSTTの結果が不正です")
	}
	return result, nil
}
