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
