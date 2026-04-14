package bridge

import (
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// MessageReducer appends []model.Message slices. The generic ReducerAppendList
// works with []any; this is typed for []model.Message to avoid casting overhead
// and preserve type safety through the bridge layer.
func MessageReducer(existing, incoming any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	inMsgs, ok := incoming.([]model.Message)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	exMsgs, ok := existing.([]model.Message)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: inMsgs")
		return inMsgs
	}
	result := make([]model.Message, len(exMsgs)+len(inMsgs))
	copy(result, exMsgs)
	copy(result[len(exMsgs):], inMsgs)
	observe.GlobalTrace("return: result")
	return result
}

// UsageReducer sums model.TokenUsage fields across LLM calls.
func UsageReducer(existing, incoming any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	inUsage, ok := incoming.(model.TokenUsage)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	exUsage, ok := existing.(model.TokenUsage)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: inUsage")
		return inUsage
	}
	observe.GlobalTrace("return: model.TokenUsage{\n\tInputTokens:\t\t\texUsage.InputTokens + inUsage.InputTokens,\n...")
	return model.TokenUsage{
		InputTokens:              exUsage.InputTokens + inUsage.InputTokens,
		OutputTokens:             exUsage.OutputTokens + inUsage.OutputTokens,
		CacheCreationInputTokens: exUsage.CacheCreationInputTokens + inUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     exUsage.CacheReadInputTokens + inUsage.CacheReadInputTokens,
	}
}

// ReflectionReducer appends []string slices for accumulated reflections.
func ReflectionReducer(existing, incoming any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	inStrs, ok := incoming.([]string)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	exStrs, ok := existing.([]string)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: inStrs")
		return inStrs
	}
	result := make([]string, len(exStrs)+len(inStrs))
	copy(result, exStrs)
	copy(result[len(exStrs):], inStrs)
	observe.GlobalTrace("return: result")
	return result
}
