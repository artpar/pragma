package anthropic

import (
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

func TestApplyCacheToLastTool(t *testing.T) {
	tools := []sdk.ToolUnionParam{
		{OfTool: &sdk.ToolParam{Name: "Bash"}},
		{OfTool: &sdk.ToolParam{Name: "Read"}},
	}
	applyCacheToLastTool(tools)

	// Last tool should have cache set
	cc := tools[1].GetCacheControl()
	if cc == nil {
		t.Fatal("last tool should have cache control")
	}
	if string(cc.Type) == "" {
		t.Error("last tool cache control type should be set")
	}
}

func TestApplyCacheToLastToolEmpty(t *testing.T) {
	// Should not panic on empty slice
	applyCacheToLastTool(nil)
	applyCacheToLastTool([]sdk.ToolUnionParam{})
}

func TestApplyCacheToLastMessageMultiple(t *testing.T) {
	msgs := []sdk.MessageParam{
		{
			Role: sdk.MessageParamRoleUser,
			Content: []sdk.ContentBlockParamUnion{
				sdk.NewTextBlock("first"),
			},
		},
		{
			Role: sdk.MessageParamRoleAssistant,
			Content: []sdk.ContentBlockParamUnion{
				sdk.NewTextBlock("second"),
			},
		},
		{
			Role: sdk.MessageParamRoleUser,
			Content: []sdk.ContentBlockParamUnion{
				sdk.NewTextBlock("third"),
			},
		},
	}
	applyCacheToLastMessage(msgs)

	// Last message's last block should have cache
	cc := msgs[2].Content[0].GetCacheControl()
	if cc == nil {
		t.Fatal("last message should have cache control")
	}
	if string(cc.Type) == "" {
		t.Error("last message cache control type should be set")
	}
}

func TestApplyCacheToLastMessageSingle(t *testing.T) {
	msgs := []sdk.MessageParam{
		{
			Role: sdk.MessageParamRoleUser,
			Content: []sdk.ContentBlockParamUnion{
				sdk.NewTextBlock("only one"),
			},
		},
	}
	applyCacheToLastMessage(msgs)

	// Single message should NOT get cache
	cc := msgs[0].Content[0].GetCacheControl()
	if cc != nil && string(cc.Type) != "" {
		t.Error("single message should not get cache control")
	}
}

func TestApplyCacheToLastMessageEmpty(t *testing.T) {
	applyCacheToLastMessage(nil)
	applyCacheToLastMessage([]sdk.MessageParam{})
}
