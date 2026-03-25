package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/agent"
	"eino-vlm-agent-demo/internal/peripherals"
)

type VisionAnalyzer interface {
	AnalyzeImage(ctx context.Context, imagePath string, prompt string) (string, error)
	Chat(ctx context.Context, req agent.ChatRequest) (string, error)
}

type agentCapability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type agentCapabilitiesResponse struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Capabilities []agentCapability `json:"capabilities"`
}

type agentChatRequest struct {
	Message         string                   `json:"message"`
	History         []agent.ConversationTurn `json:"history,omitempty"`
	CaptureFresh    bool                     `json:"capture_fresh,omitempty"`
	UseLatestImage  bool                     `json:"use_latest_image,omitempty"`
	IncludeSnapshot bool                     `json:"include_snapshot,omitempty"`
}

type agentChatResponse struct {
	Reply       string                     `json:"reply,omitempty"`
	Capture     *peripherals.CaptureResult `json:"capture,omitempty"`
	Peripherals *peripherals.FleetSnapshot `json:"peripherals,omitempty"`
	Error       string                     `json:"error,omitempty"`
}

type ObservationService struct {
	workdir       string
	defaultPrompt string
	peripherals   *peripherals.Manager
	analyzer      VisionAnalyzer
}

func NewObservationService(workdir string, defaultPrompt string, peripheralsManager *peripherals.Manager, analyzer VisionAnalyzer) *ObservationService {
	return &ObservationService{
		workdir:       workdir,
		defaultPrompt: defaultPrompt,
		peripherals:   peripheralsManager,
		analyzer:      analyzer,
	}
}

func (s *ObservationService) InspectPeripherals(ctx context.Context) peripherals.FleetSnapshot {
	return s.peripherals.InspectAll(ctx)
}

func (s *ObservationService) InspectPrimary(ctx context.Context) (peripherals.DeviceSnapshot, error) {
	return s.peripherals.InspectPrimary(ctx)
}

func (s *ObservationService) CapturePrimary(ctx context.Context) (*peripherals.CaptureResult, error) {
	filename := fmt.Sprintf("capture-%d.jpg", time.Now().UnixNano())
	outputPath := filepath.Join(s.workdir, filename)
	return s.peripherals.CapturePrimary(ctx, outputPath)
}

func (s *ObservationService) AnalyzePrimary(ctx context.Context, prompt string) (analyzeResponse, error) {
	if prompt == "" {
		prompt = s.defaultPrompt
	}

	capture, err := s.CapturePrimary(ctx)
	if err != nil {
		return analyzeResponse{}, err
	}
	if capture.Error != "" || !capture.OK {
		return analyzeResponse{
			Capture:     capture,
			Peripherals: s.snapshotPtr(ctx),
			Error:       "camera capture failed before agent inference",
		}, nil
	}

	analyzeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	result, err := s.analyzer.AnalyzeImage(analyzeCtx, capture.Output, prompt)
	if err != nil {
		return analyzeResponse{
			Capture:     capture,
			Peripherals: s.snapshotPtr(ctx),
			Error:       err.Error(),
		}, nil
	}

	return analyzeResponse{
		Capture:     capture,
		Peripherals: s.snapshotPtr(ctx),
		Result:      result,
	}, nil
}

func (s *ObservationService) AgentCapabilities() agentCapabilitiesResponse {
	return agentCapabilitiesResponse{
		Name:        "Jetson Peripheral Agent",
		Description: "Multimodal chat over live camera frames and embedded peripheral snapshots.",
		Capabilities: []agentCapability{
			{ID: "chat", Name: "Dialogue", Description: "Maintains short-turn conversation with user prompts and assistant responses."},
			{ID: "vision", Name: "Vision Context", Description: "Can analyze a fresh capture or the most recent local image when available."},
			{ID: "peripherals", Name: "Peripheral Snapshot", Description: "Can include the current peripheral fleet state in the reasoning context."},
		},
	}
}

func (s *ObservationService) AgentChat(ctx context.Context, req agentChatRequest) (agentChatResponse, error) {
	prompt := strings.TrimSpace(req.Message)
	if prompt == "" {
		prompt = s.defaultPrompt
	}

	var capture *peripherals.CaptureResult
	var imagePath string
	var err error

	switch {
	case req.CaptureFresh:
		capture, err = s.CapturePrimary(ctx)
		if err != nil {
			return agentChatResponse{}, err
		}
		if capture.Error != "" || !capture.OK {
			return agentChatResponse{
				Capture:     capture,
				Peripherals: s.snapshotPtr(ctx),
				Error:       "camera capture failed before agent chat",
			}, nil
		}
		imagePath = capture.Output
	case req.UseLatestImage:
		imagePath, err = s.LatestCapturePath()
		if err != nil {
			imagePath = ""
		}
	}

	var snapshot *peripherals.FleetSnapshot
	if req.IncludeSnapshot || req.CaptureFresh {
		snapshot = s.snapshotPtr(ctx)
	}

	chatPrompt := prompt
	if snapshot != nil {
		snapshotJSON, err := json.MarshalIndent(snapshot, "", "  ")
		if err == nil {
			chatPrompt = fmt.Sprintf("%s\n\nPeripheral snapshot:\n%s", prompt, string(snapshotJSON))
		}
	}

	chatCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	reply, err := s.analyzer.Chat(chatCtx, agent.ChatRequest{
		History:   req.History,
		Prompt:    chatPrompt,
		ImagePath: imagePath,
	})
	if err != nil {
		return agentChatResponse{
			Capture:     capture,
			Peripherals: snapshot,
			Error:       err.Error(),
		}, nil
	}

	return agentChatResponse{
		Reply:       reply,
		Capture:     capture,
		Peripherals: snapshot,
	}, nil
}

func (s *ObservationService) LatestCapturePath() (string, error) {
	matches, err := filepath.Glob(filepath.Join(s.workdir, "capture-*.jpg"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no captured image is available yet")
	}

	sort.Slice(matches, func(i int, j int) bool {
		return matches[i] > matches[j]
	})

	return matches[0], nil
}

func (s *ObservationService) snapshotPtr(ctx context.Context) *peripherals.FleetSnapshot {
	snapshot := s.InspectPeripherals(ctx)
	return &snapshot
}
