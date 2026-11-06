package wakegate

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalSTTUsesWAVAndReturnsSeparateConfidence(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Type") != "audio/wav" || string(b[:4]) != "RIFF" || binary.LittleEndian.Uint32(b[24:]) != 8000 {
			t.Error("WAV形式が違います")
		}
		_, _ = w.Write([]byte(`{"text":"スピーカー","confidence":0.91,"words":[{"word":"スピーカー","probability":0.91}]}`))
	}))
	defer api.Close()
	c := HTTPClient{Endpoint: api.URL}
	r, err := c.Transcribe(context.Background(), make([]byte, 1600), 8000)
	if err != nil || r.Text != "スピーカー" || r.Confidence != 0.91 {
		t.Fatal(r, err)
	}
}
