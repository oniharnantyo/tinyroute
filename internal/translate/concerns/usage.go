package concerns

import "github.com/oniharnantyo/tinyroute/internal/core"

// UsageFromOpenAI builds an Anthropic usage value from the token counts in an
// OpenAI usage chunk. Anthropic's input_tokens excludes cached and
// cache-creation tokens, which the proxy reports separately, so input =
// prompt − cached − cache_creation (clamped at zero to stay defensive).
func UsageFromOpenAI(promptTokens, completionTokens, cachedTokens, cacheCreationTokens int64) *core.AnthropicUsage {
	out := &core.AnthropicUsage{
		InputTokens:              promptTokens,
		OutputTokens:             completionTokens,
		CacheReadInputTokens:     cachedTokens,
		CacheCreationInputTokens: cacheCreationTokens,
	}
	in := promptTokens - cachedTokens - cacheCreationTokens
	if in < 0 {
		in = 0
	}
	out.InputTokens = in
	return out
}
