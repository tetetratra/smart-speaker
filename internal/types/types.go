package types

// AudioChunk represents a base64 encoded PCM chunk.
type AudioChunk string

// OutputLine represents a single output entry from the assistant or user.
type OutputLine struct {
	Role string
	Text string
}
