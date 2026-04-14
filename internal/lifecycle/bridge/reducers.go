package bridge

import (
	"github.com/artpar/gogent/internal/model"
)

// MessageReducer appends []model.Message slices. The generic ReducerAppendList
// works with []any; this is typed for []model.Message to avoid casting overhead
// and preserve type safety through the bridge layer.
func MessageReducer(existing, incoming any) any {
	inMsgs, ok := incoming.([]model.Message)
	if !ok {
		return incoming
	}
	exMsgs, ok := existing.([]model.Message)
	if !ok {
		return inMsgs
	}
	result := make([]model.Message, len(exMsgs)+len(inMsgs))
	copy(result, exMsgs)
	copy(result[len(exMsgs):], inMsgs)
	return result
}

// UsageReducer sums model.TokenUsage fields across LLM calls.
func UsageReducer(existing, incoming any) any {
	inUsage, ok := incoming.(model.TokenUsage)
	if !ok {
		return incoming
	}
	exUsage, ok := existing.(model.TokenUsage)
	if !ok {
		return inUsage
	}
	return model.TokenUsage{
		InputTokens:              exUsage.InputTokens + inUsage.InputTokens,
		OutputTokens:             exUsage.OutputTokens + inUsage.OutputTokens,
		CacheCreationInputTokens: exUsage.CacheCreationInputTokens + inUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     exUsage.CacheReadInputTokens + inUsage.CacheReadInputTokens,
	}
}

// ReflectionReducer appends []string slices for accumulated reflections.
func ReflectionReducer(existing, incoming any) any {
	inStrs, ok := incoming.([]string)
	if !ok {
		return incoming
	}
	exStrs, ok := existing.([]string)
	if !ok {
		return inStrs
	}
	result := make([]string, len(exStrs)+len(inStrs))
	copy(result, exStrs)
	copy(result[len(exStrs):], inStrs)
	return result
}
