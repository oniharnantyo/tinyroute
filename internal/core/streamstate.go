package core

import "bytes"

// AnthropicUsage is the token-usage shape carried by Anthropic message_start
// and message_delta events. It is produced by concerns/usage.go from an OpenAI
// usage chunk and stored on the streaming state so the translator can attach it
// to the final message_delta.
type AnthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// ToolCallState tracks one OpenAI tool call being converted into an Anthropic
// tool_use content block across streaming chunks. Arguments are buffered and
// emitted as a single sanitized input_json_delta at finish.
type ToolCallState struct {
	BlockIndex int    // anthropic content block index for the tool_use block
	ID         string // synthesized "toolu_…" id
	Name       string
	Closed     bool // content_block_stop already emitted
}

// StreamState is the mutable state threaded through a streaming response
// translation. It is initialized once per upstream stream and mutated by each
// chunk (and by the nil/drain call at end-of-stream). It bookkeeps the ordering
// of Anthropic content blocks, synthesized identifiers, buffered tool
// arguments, and the final usage captured from the upstream usage chunk.
//
// The type lives in core so both core.ResponseTranslator and the translate
// subpackages can reference it without an import cycle; translate re-exports it
// as translate.StreamState.
type StreamState struct {
	MessageStartSent bool
	MessageClosed    bool
	MessageID        string // synthesized "msg_…" (upstream id is "chatcmpl-…")
	Model            string

	NextBlockIndex     int // anthropic content blocks are ordered
	TextBlockIndex     int
	TextBlockOpen      bool
	ThinkingBlockIndex int
	ThinkingBlockOpen  bool

	ToolCalls      map[int]ToolCallState // openai tool index → anthropic block
	ToolArgBuffers map[int][]byte        // buffered args, emitted sanitized at finish

	FinishReason string
	Usage        *AnthropicUsage // captured from final openai usage chunk
	TextBuffer   bytes.Buffer    // accumulated text for non-streaming response synthesis

	// GeminiNameMap carries tool name sanitization mappings across request and response translation.
	GeminiNameMap interface {
		Sanitize(name string) string
		Restore(san string) string
	}
}
