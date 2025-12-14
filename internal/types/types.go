package types

// Base64 で表現された PCM チャンク
type AudioChunk string

// アシスタントやユーザーの 1 行分の出力
type OutputLine struct {
	Role       string
	Text       string
	ResponseID string
	Final      bool
	NoResponse bool // optional: when true, request no assistant response
}

// OutputAudio represents an assistant audio response chunk.
type OutputAudio struct {
	Role  string
	Audio string
}
