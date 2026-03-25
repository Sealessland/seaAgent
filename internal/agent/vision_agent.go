package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	BaseURL          string
	APIKey           string
	Model            string
	EnableImageInput bool
	SystemPrompt     string
}

type ConversationTurn struct {
	Role    string
	Content string
}

type ChatRequest struct {
	History          []ConversationTurn
	Prompt           string
	ImagePath        string
	ImagePaths       []string
	Tools            []tool.InvokableTool
	ForcedToolNames  []string
	MaxToolCallLoops int
}

type ToolCallTrace struct {
	Name      string
	Arguments string
	Result    string
}

type ChatResponse struct {
	Content   string
	ToolCalls []ToolCallTrace
}

type VisionAgent struct {
	chatModel                model.BaseChatModel
	systemPrompt             string
	enableImageInput         bool
	supportsForcedToolChoice bool
}

func NewVisionAgent(ctx context.Context, cfg Config) (*VisionAgent, error) {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return &VisionAgent{
		chatModel:                chatModel,
		systemPrompt:             cfg.SystemPrompt,
		enableImageInput:         cfg.EnableImageInput,
		supportsForcedToolChoice: !strings.Contains(strings.ToLower(cfg.BaseURL), "dashscope.aliyuncs.com"),
	}, nil
}

func (a *VisionAgent) AnalyzeImage(ctx context.Context, imagePath string, prompt string) (string, error) {
	if !a.enableImageInput {
		return "", fmt.Errorf("configured model does not support image input")
	}
	return a.generateWithModel(ctx, a.chatModel, nil, prompt, []string{imagePath})
}

func (a *VisionAgent) AnalyzeDataURL(ctx context.Context, dataURL string, mimeType string, prompt string) (string, error) {
	if strings.TrimSpace(dataURL) == "" {
		return "", fmt.Errorf("image data url is empty")
	}
	resp, err := a.generate(ctx, nil, prompt, []schema.MessageInputPart{{
		Type: schema.ChatMessagePartTypeImageURL,
		Image: &schema.MessageInputImage{
			MessagePartCommon: schema.MessagePartCommon{
				URL:      &dataURL,
				MIMEType: mimeType,
			},
			Detail: schema.ImageURLDetailHigh,
		},
	}}, nil, nil, 0)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *VisionAgent) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	imagePaths := combineImagePaths(req.ImagePaths, req.ImagePath)
	if len(imagePaths) > 0 && !a.enableImageInput {
		return ChatResponse{}, fmt.Errorf("configured model does not support image input")
	}

	imageParts, err := imagePartsFromPaths(imagePaths)
	if err != nil {
		return ChatResponse{}, err
	}

	resp, err := a.generate(ctx, req.History, req.Prompt, imageParts, req.Tools, req.ForcedToolNames, req.MaxToolCallLoops)
	if err != nil {
		return ChatResponse{}, err
	}

	if !a.enableImageInput {
		return resp, nil
	}

	capturedImagePath := latestImagePathFromToolCalls(resp.ToolCalls)
	if capturedImagePath == "" {
		return resp, nil
	}
	synthesisImagePaths := combineImagePaths(imagePaths, capturedImagePath)

	synthesizedContent, err := a.generateWithModel(
		ctx,
		a.chatModel,
		req.History,
		buildPostToolAnswerPrompt(req.Prompt, resp.Content, resp.ToolCalls, len(synthesisImagePaths)),
		synthesisImagePaths,
	)
	if err != nil {
		return ChatResponse{}, err
	}

	resp.Content = synthesizedContent
	return resp, nil
}

func (a *VisionAgent) generate(
	ctx context.Context,
	history []ConversationTurn,
	prompt string,
	imageParts []schema.MessageInputPart,
	tools []tool.InvokableTool,
	forcedToolNames []string,
	maxToolCallLoops int,
) (ChatResponse, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Describe the visible scene and mention anything relevant for robotics or safety."
	}
	if len(forcedToolNames) > 0 && !a.supportsForcedToolChoice {
		prompt = strings.TrimSpace(prompt) + "\n\nTool requirement:\nYou must call the required tool before answering."
	}
	if maxToolCallLoops <= 0 {
		maxToolCallLoops = 4
	}

	messages := make([]*schema.Message, 0, len(history)+1)
	for _, turn := range history {
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(turn.Role)) {
		case "assistant":
			messages = append(messages, schema.AssistantMessage(content, nil))
		default:
			messages = append(messages, schema.UserMessage(content))
		}
	}

	userMessage := &schema.Message{
		Role: schema.User,
		UserInputMultiContent: []schema.MessageInputPart{
			{
				Type: schema.ChatMessagePartTypeText,
				Text: prompt,
			},
		},
	}
	if len(imageParts) > 0 {
		userMessage.UserInputMultiContent = append(userMessage.UserInputMultiContent, imageParts...)
	}
	messages = append(messages, userMessage)

	if len(tools) == 0 {
		modelMessages := append([]*schema.Message{schema.SystemMessage(a.systemPrompt)}, messages...)
		resp, err := a.chatModel.Generate(ctx, modelMessages)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("generate multimodal response: %w", err)
		}
		if resp == nil {
			return ChatResponse{}, fmt.Errorf("empty response")
		}
		content := strings.TrimSpace(resp.Content)
		if content == "" {
			return ChatResponse{}, fmt.Errorf("response content is empty")
		}
		return ChatResponse{Content: content}, nil
	}

	chatAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "vision-dialogue-agent",
		Description: "Vision and peripheral assistant with callable tools.",
		Instruction: a.systemPrompt,
		Model:       a.chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toBaseTools(tools),
			},
			ReturnDirectly: map[string]bool{
				"tool_call_smoke_test": true,
			},
		},
		MaxIterations: maxToolCallLoops,
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create chat model agent: %w", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           chatAgent,
		EnableStreaming: false,
	})

	runOpts := make([]adk.AgentRunOption, 0, 1)
	if len(forcedToolNames) > 0 && a.supportsForcedToolChoice {
		runOpts = append(runOpts, adk.WithChatModelOptions([]model.Option{
			model.WithToolChoice(schema.ToolChoiceForced, forcedToolNames...),
		}))
	}

	iter := runner.Run(ctx, messages, runOpts...)
	toolArgsByID := make(map[string]schema.ToolCall)
	trace := make([]ToolCallTrace, 0, len(tools))
	var finalContent string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return ChatResponse{}, event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return ChatResponse{}, fmt.Errorf("read agent event message: %w", err)
		}
		if msg == nil {
			continue
		}

		switch event.Output.MessageOutput.Role {
		case schema.Assistant:
			for _, toolCall := range msg.ToolCalls {
				toolArgsByID[toolCall.ID] = toolCall
			}
			if len(msg.ToolCalls) == 0 && strings.TrimSpace(msg.Content) != "" {
				finalContent = strings.TrimSpace(msg.Content)
			}
		case schema.Tool:
			call := toolArgsByID[msg.ToolCallID]
			trace = append(trace, ToolCallTrace{
				Name:      event.Output.MessageOutput.ToolName,
				Arguments: call.Function.Arguments,
				Result:    msg.Content,
			})
			if finalContent == "" && event.Output.MessageOutput.ToolName == "tool_call_smoke_test" {
				finalContent = strings.TrimSpace(msg.Content)
			}
		}
	}

	if finalContent == "" {
		return ChatResponse{}, fmt.Errorf("response content is empty")
	}
	return ChatResponse{
		Content:   finalContent,
		ToolCalls: trace,
	}, nil
}

func toBaseTools(tools []tool.InvokableTool) []tool.BaseTool {
	baseTools := make([]tool.BaseTool, 0, len(tools))
	for _, currentTool := range tools {
		baseTools = append(baseTools, currentTool)
	}
	return baseTools
}

func (a *VisionAgent) generateWithModel(
	ctx context.Context,
	chatModel model.BaseChatModel,
	history []ConversationTurn,
	prompt string,
	imagePaths []string,
) (string, error) {
	if chatModel == nil {
		return "", fmt.Errorf("chat model is nil")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Describe the visible scene and mention anything relevant for robotics or safety."
	}
	imageParts, err := imagePartsFromPaths(imagePaths)
	if err != nil {
		return "", err
	}

	messages := make([]*schema.Message, 0, len(history)+2)
	for _, turn := range history {
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(turn.Role)) {
		case "assistant":
			messages = append(messages, schema.AssistantMessage(content, nil))
		default:
			messages = append(messages, schema.UserMessage(content))
		}
	}

	userMessage := &schema.Message{
		Role: schema.User,
		UserInputMultiContent: []schema.MessageInputPart{
			{
				Type: schema.ChatMessagePartTypeText,
				Text: prompt,
			},
		},
	}
	if len(imageParts) > 0 {
		userMessage.UserInputMultiContent = append(userMessage.UserInputMultiContent, imageParts...)
	}
	messages = append(messages, userMessage)

	modelMessages := append([]*schema.Message{schema.SystemMessage(a.systemPrompt)}, messages...)
	resp, err := chatModel.Generate(ctx, modelMessages)
	if err != nil {
		return "", fmt.Errorf("generate multimodal response: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("empty response")
	}
	content := strings.TrimSpace(resp.Content)
	if content == "" {
		return "", fmt.Errorf("response content is empty")
	}
	return content, nil
}

func buildPostToolAnswerPrompt(userPrompt string, draftReply string, toolCalls []ToolCallTrace, imageCount int) string {
	var builder strings.Builder
	switch {
	case imageCount >= 2:
		builder.WriteString("You have multiple attached images.\n")
		builder.WriteString("The earlier attached image is the previous frame, and the last attached image is the newest frame.\n")
		builder.WriteString("Compare them and answer the user's request directly.\n")
	default:
		builder.WriteString("The tool call has already captured an image for you.\n")
		builder.WriteString("Read the image and answer the user's request directly.\n")
	}
	builder.WriteString("Do not just repeat raw tool JSON or file paths.\n")
	if strings.TrimSpace(userPrompt) != "" {
		builder.WriteString("\nUser request:\n")
		builder.WriteString(strings.TrimSpace(userPrompt))
		builder.WriteString("\n")
	}
	if len(toolCalls) > 0 {
		builder.WriteString("\nTool outputs:\n")
		builder.WriteString(summarizeToolCalls(toolCalls))
		builder.WriteString("\n")
	}
	if strings.TrimSpace(draftReply) != "" {
		builder.WriteString("\nDraft reply before image reading:\n")
		builder.WriteString(strings.TrimSpace(draftReply))
		builder.WriteString("\n")
	}
	if imageCount >= 2 {
		builder.WriteString("\nFinal answer requirements:\nGround the answer in the attached images, and explicitly mention differences or confirm if there is no meaningful change.")
	} else {
		builder.WriteString("\nFinal answer requirements:\nGround the answer in the image you just received. Mention uncertainty if the image is unclear.")
	}
	return builder.String()
}

func latestImagePathFromToolCalls(traces []ToolCallTrace) string {
	for i := len(traces) - 1; i >= 0; i-- {
		path := imagePathFromToolResult(traces[i])
		if path != "" {
			return path
		}
	}
	return ""
}

func imagePathFromToolResult(trace ToolCallTrace) string {
	if trace.Name != "camera_read" && trace.Name != "ros2_topic_read" {
		return ""
	}
	var payload struct {
		ImagePath string `json:"image_path"`
	}
	if err := json.Unmarshal([]byte(trace.Result), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.ImagePath)
}

func summarizeToolCalls(traces []ToolCallTrace) string {
	var builder strings.Builder
	for _, trace := range traces {
		builder.WriteString("- ")
		builder.WriteString(trace.Name)
		if strings.TrimSpace(trace.Arguments) != "" {
			builder.WriteString(" args=")
			builder.WriteString(strings.TrimSpace(trace.Arguments))
		}
		if strings.TrimSpace(trace.Result) != "" {
			builder.WriteString(" result=")
			builder.WriteString(trimForPrompt(trace.Result, 600))
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func trimForPrompt(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func combineImagePaths(paths []string, extra ...string) []string {
	combined := make([]string, 0, len(paths)+len(extra))
	seen := make(map[string]struct{}, len(paths)+len(extra))
	for _, path := range append(append([]string{}, paths...), extra...) {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		combined = append(combined, path)
	}
	return combined
}

func imagePartsFromPaths(paths []string) ([]schema.MessageInputPart, error) {
	parts := make([]schema.MessageInputPart, 0, len(paths))
	for _, path := range combineImagePaths(paths) {
		dataURL, mimeType, err := imageFileToDataURL(path)
		if err != nil {
			return nil, err
		}
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					URL:      &dataURL,
					MIMEType: mimeType,
				},
				Detail: schema.ImageURLDetailHigh,
			},
		})
	}
	return parts, nil
}

func imageFileToDataURL(imagePath string) (string, string, error) {
	fileContent, err := os.ReadFile(imagePath)
	if err != nil {
		return "", "", fmt.Errorf("read image %s: %w", imagePath, err)
	}

	mimeType := http.DetectContentType(fileContent)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", "", fmt.Errorf("unsupported mime type %q for %s", mimeType, filepath.Base(imagePath))
	}

	encoded := base64.StdEncoding.EncodeToString(fileContent)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
	return dataURL, mimeType, nil
}

func mimeTypeFromDataURL(dataURL string) (string, error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return "", fmt.Errorf("invalid data url prefix")
	}
	first := strings.SplitN(strings.TrimPrefix(dataURL, "data:"), ";", 2)
	if len(first) < 2 || strings.TrimSpace(first[0]) == "" {
		return "", fmt.Errorf("invalid data url mime type")
	}
	return first[0], nil
}
