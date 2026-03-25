package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type fakeToolCallingModel struct {
	callCount int
}

func (m *fakeToolCallingModel) Generate(_ context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.callCount++
	common := model.GetCommonOptions(nil, opts...)
	if m.callCount == 1 {
		if common.ToolChoice == nil || *common.ToolChoice != schema.ToolChoiceForced {
			return nil, fmt.Errorf("expected forced tool choice on first generate")
		}
		if len(common.AllowedToolNames) != 1 || common.AllowedToolNames[0] != "tool_call_smoke_test" {
			return nil, fmt.Errorf("unexpected allowed tool names: %#v", common.AllowedToolNames)
		}
		return schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "tool_call_smoke_test",
					Arguments: `{"token":"smoke-123"}`,
				},
			},
		}), nil
	}

	last := input[len(input)-1]
	if last.Role != schema.Tool || last.ToolName != "tool_call_smoke_test" {
		return nil, fmt.Errorf("expected tool message, got role=%s tool=%s", last.Role, last.ToolName)
	}
	return schema.AssistantMessage("tool verified", nil), nil
}

func (m *fakeToolCallingModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, io.EOF
}

func TestVisionAgentChatParsesToolCallsFromADKEvents(t *testing.T) {
	ctx := context.Background()

	smokeTool, err := utils.InferTool(
		"tool_call_smoke_test",
		"deterministic smoke test tool",
		func(_ context.Context, input struct {
			Token string `json:"token"`
		}) (map[string]string, error) {
			return map[string]string{
				"echo": "tool-call-ok:" + input.Token,
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("infer tool: %v", err)
	}

	agent := &VisionAgent{
		chatModel:                &fakeToolCallingModel{},
		systemPrompt:             "You are a test agent.",
		supportsForcedToolChoice: true,
	}

	resp, err := agent.Chat(ctx, ChatRequest{
		Prompt:           "/tooltest run smoke test",
		Tools:            []tool.InvokableTool{smokeTool},
		ForcedToolNames:  []string{"tool_call_smoke_test"},
		MaxToolCallLoops: 4,
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	if resp.Content != `{"echo":"tool-call-ok:smoke-123"}` {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("unexpected tool call count: %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "tool_call_smoke_test" {
		t.Fatalf("unexpected tool name: %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments != `{"token":"smoke-123"}` {
		t.Fatalf("unexpected tool args: %q", resp.ToolCalls[0].Arguments)
	}
	if resp.ToolCalls[0].Result == "" {
		t.Fatal("expected tool result to be captured")
	}
}

type fakeCameraReasoningModel struct {
	callCount int
}

func (m *fakeCameraReasoningModel) Generate(_ context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.callCount++
	switch m.callCount {
	case 1:
		common := model.GetCommonOptions(nil, opts...)
		if common.ToolChoice == nil || *common.ToolChoice != schema.ToolChoiceForced {
			return nil, fmt.Errorf("expected forced tool choice on first generate")
		}
		return schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "camera-call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "camera_read",
					Arguments: `{"mode":"capture_fresh"}`,
				},
			},
		}), nil
	case 2:
		last := input[len(input)-1]
		if last.Role != schema.Tool || last.ToolName != "camera_read" {
			return nil, fmt.Errorf("expected camera_read tool message, got role=%s tool=%s", last.Role, last.ToolName)
		}
		return schema.AssistantMessage("capture finished", nil), nil
	case 3:
		last := input[len(input)-1]
		if len(last.UserInputMultiContent) < 2 || last.UserInputMultiContent[1].Image == nil {
			return nil, fmt.Errorf("expected follow-up multimodal prompt")
		}
		text := joinedMessageText(last)
		if !strings.Contains(text, "Read the image") {
			return nil, fmt.Errorf("expected image-reading follow-up prompt, got %q", text)
		}
		return schema.AssistantMessage("画面右侧有一台白色履带机器人，左侧是一个黑色设备箱。", nil), nil
	default:
		return nil, fmt.Errorf("unexpected generate call count: %d", m.callCount)
	}
}

func (m *fakeCameraReasoningModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, io.EOF
}

func TestVisionAgentChatSynthesizesImageAfterCameraToolCall(t *testing.T) {
	ctx := context.Background()
	imagePath := writeTestPNG(t)

	cameraTool, err := utils.InferTool(
		"camera_read",
		"capture one camera frame",
		func(_ context.Context, input struct {
			Mode string `json:"mode"`
		}) (map[string]any, error) {
			return map[string]any{
				"ok":         true,
				"mode":       input.Mode,
				"image_path": imagePath,
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("infer camera tool: %v", err)
	}

	agent := &VisionAgent{
		chatModel:                &fakeCameraReasoningModel{},
		systemPrompt:             "You are a test agent.",
		enableImageInput:         true,
		supportsForcedToolChoice: true,
	}

	resp, err := agent.Chat(ctx, ChatRequest{
		Prompt:           "请调用 camera_read 抓一张图，然后根据图像内容回答。",
		Tools:            []tool.InvokableTool{cameraTool},
		ForcedToolNames:  []string{"camera_read"},
		MaxToolCallLoops: 4,
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	if !strings.Contains(resp.Content, "白色履带机器人") {
		t.Fatalf("unexpected synthesized content: %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("unexpected tool call count: %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "camera_read" {
		t.Fatalf("unexpected tool name: %q", resp.ToolCalls[0].Name)
	}
}

func writeTestPNG(t *testing.T) string {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO5WJ3kAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	return path
}

func joinedMessageText(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Content) != "" {
		return msg.Content
	}
	parts := make([]string, 0, len(msg.UserInputMultiContent))
	for _, part := range msg.UserInputMultiContent {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, strings.TrimSpace(part.Text))
		}
	}
	return strings.Join(parts, "\n")
}
