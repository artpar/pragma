package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/artpar/gogent/internal/observe"
)

// applyCacheBreakpoints adds cache_control markers to the wire params:
//  1. System blocks: already handled in systemToWire (Cacheable → CacheControl).
//  2. Tool definitions: marker on the last tool.
//  3. Messages: marker on the last content block of the last message.
//  4. Skip message caching if only 1 message (nothing to cache yet).
func applyCacheBreakpoints(params *sdk.MessageNewParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	applyCacheToLastTool(params.Tools)
	applyCacheToLastMessage(params.Messages)
}

// applyCacheToLastTool sets cache_control on the last tool definition.
// Tools rarely change mid-session, so caching the full tool list is effective.
func applyCacheToLastTool(tools []sdk.ToolUnionParam) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(tools) == 0 {
		observe.GlobalTrace("if: len(tools) == 0")
		return
	}
	cc := sdk.NewCacheControlEphemeralParam()
	last := &tools[len(tools)-1]
	if p := last.GetCacheControl(); p != nil {
		observe.GlobalTrace("if: p != nil")
		*p = cc
	}
}

// applyCacheToLastMessage sets cache_control on the last content block of
// the last message. This maximizes the cached prefix (system + tools + all
// prior messages). Skipped if there's only 1 message.
func applyCacheToLastMessage(msgs []sdk.MessageParam) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(msgs) <= 1 {
		observe.GlobalTrace("if: len(msgs) <= 1")
		return
	}
	lastMsg := &msgs[len(msgs)-1]
	if len(lastMsg.Content) == 0 {
		observe.GlobalTrace("if: len(lastMsg.Content) == 0")
		return
	}
	cc := sdk.NewCacheControlEphemeralParam()
	lastBlock := &lastMsg.Content[len(lastMsg.Content)-1]
	if p := lastBlock.GetCacheControl(); p != nil {
		observe.GlobalTrace("if: p != nil")
		*p = cc
	}
}
