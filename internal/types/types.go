package types

// Base64 で表現された PCM チャンク
type AudioChunk string

// アシスタントやユーザーの 1 行分の出力
type OutputLine struct {
	Role string
	Text string
}
