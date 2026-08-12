package translate

import "github.com/oniharnantyo/tinyroute/internal/core"

// StreamState is the mutable state threaded through a streaming response
// translation. It re-exports core.StreamState (the canonical definition lives
// in core so the core.ResponseTranslator interface and the translate
// subpackages can share it without an import cycle).
type StreamState = core.StreamState

// NewStreamState returns an initialized StreamState ready for a stream.
func NewStreamState() *StreamState {
	return &StreamState{
		ToolCalls:      make(map[int]core.ToolCallState),
		ToolArgBuffers: make(map[int][]byte),
	}
}
