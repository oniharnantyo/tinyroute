// Package concerns holds small, dependency-free mappings shared by the
// translators (ported from 9router's translator/concerns/). Each concern is a
// pure mapping with no package-level state.
package concerns

// FromOpenAIFinish maps an OpenAI finish_reason to the equivalent Anthropic
// stop_reason for the given target dialect. target is currently always
// "anthropic"; an unknown reason falls through to a default rather than
// breaking the translation.
func FromOpenAIFinish(reason, target string) string {
	switch target {
	case "anthropic":
		switch reason {
		case "stop":
			return "end_turn"
		case "length":
			return "max_tokens"
		case "tool_calls", "function_call":
			return "tool_use"
		case "content_filter":
			return "content_filter"
		case "":
			return "end_turn"
		default:
			return "end_turn"
		}
	default:
		// Unknown target: be lenient and keep the upstream value.
		return reason
	}
}
