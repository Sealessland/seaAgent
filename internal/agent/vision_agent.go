package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
}

type VisionAgent struct {
	chatModel    model.ToolCallingChatModel
	systemPrompt string
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
		chatModel:    chatModel,
		systemPrompt: cfg.SystemPrompt,
	}, nil
}

func (a *VisionAgent) AnalyzeImage(ctx context.Context, imagePath string, prompt string) (string, error) {
	dataURL, mimeType, err := imageFileToDataURL(imagePath)
	if err != nil {
		return "", err
	}
	return a.AnalyzeDataURL(ctx, dataURL, mimeType, prompt)
}

func (a *VisionAgent) AnalyzeDataURL(ctx context.Context, dataURL string, mimeType string, prompt string) (string, error) {
	if strings.TrimSpace(dataURL) == "" {
		return "", fmt.Errorf("image data url is empty")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Describe the visible scene and mention anything relevant for robotics or safety."
	}
	if strings.TrimSpace(mimeType) == "" {
		parsed, err := mimeTypeFromDataURL(dataURL)
		if err != nil {
			return "", err
		}
		mimeType = parsed
	}

	messages := []*schema.Message{
		schema.SystemMessage(a.systemPrompt),
		{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{
					Type: schema.ChatMessagePartTypeText,
					Text: prompt,
				},
				{
					Type: schema.ChatMessagePartTypeImageURL,
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{
							URL:      &dataURL,
							MIMEType: mimeType,
						},
						Detail: schema.ImageURLDetailHigh,
					},
				},
			},
		},
	}

	resp, err := a.chatModel.Generate(ctx, messages)
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
