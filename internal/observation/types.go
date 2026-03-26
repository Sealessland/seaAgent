package observation

import (
	"context"

	"eino-vlm-agent-demo/internal/agent"
	"eino-vlm-agent-demo/internal/peripherals"
)

type Analyzer interface {
	AnalyzeImage(ctx context.Context, imagePath string, prompt string) (string, error)
	Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error)
}

type Capability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CapabilitiesResponse struct {
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Capabilities []Capability `json:"capabilities"`
}

type ChatRequest struct {
	SessionID       string                   `json:"session_id,omitempty"`
	Message         string                   `json:"message"`
	History         []agent.ConversationTurn `json:"history,omitempty"`
	CaptureFresh    bool                     `json:"capture_fresh,omitempty"`
	UseLatestImage  bool                     `json:"use_latest_image,omitempty"`
	IncludeSnapshot bool                     `json:"include_snapshot,omitempty"`
}

type Action struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type Source struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type ToolCallRecord struct {
	Name   string            `json:"name"`
	Input  map[string]string `json:"input,omitempty"`
	Output map[string]string `json:"output,omitempty"`
}

type Trace struct {
	Intent       string           `json:"intent"`
	Actions      []Action         `json:"actions"`
	ToolCalls    []ToolCallRecord `json:"tool_calls,omitempty"`
	RetrievedIDs []string         `json:"retrieved_ids"`
}

type ChatResponse struct {
	SessionID   string                     `json:"session_id,omitempty"`
	Reply       string                     `json:"reply,omitempty"`
	Capture     *peripherals.CaptureResult `json:"capture,omitempty"`
	Peripherals *peripherals.FleetSnapshot `json:"peripherals,omitempty"`
	Sources     []Source                   `json:"sources,omitempty"`
	Trace       *Trace                     `json:"trace,omitempty"`
	Error       string                     `json:"error,omitempty"`
}

type AnalyzeResponse struct {
	Result      string                     `json:"result,omitempty"`
	Capture     *peripherals.CaptureResult `json:"capture,omitempty"`
	Peripherals *peripherals.FleetSnapshot `json:"peripherals,omitempty"`
	Error       string                     `json:"error,omitempty"`
}
