package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/agent"
	"eino-vlm-agent-demo/internal/peripherals"

	einotool "github.com/cloudwego/eino/components/tool"
)

type VisionAnalyzer interface {
	AnalyzeImage(ctx context.Context, imagePath string, prompt string) (string, error)
	Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error)
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
	SessionID       string                   `json:"session_id,omitempty"`
	Message         string                   `json:"message"`
	History         []agent.ConversationTurn `json:"history,omitempty"`
	CaptureFresh    bool                     `json:"capture_fresh,omitempty"`
	UseLatestImage  bool                     `json:"use_latest_image,omitempty"`
	IncludeSnapshot bool                     `json:"include_snapshot,omitempty"`
}

type agentAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type agentSource struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type agentTrace struct {
	Intent       string           `json:"intent"`
	Actions      []agentAction    `json:"actions"`
	ToolCalls    []toolCallRecord `json:"tool_calls,omitempty"`
	RetrievedIDs []string         `json:"retrieved_ids"`
}

type agentChatResponse struct {
	SessionID   string                     `json:"session_id,omitempty"`
	Reply       string                     `json:"reply,omitempty"`
	Capture     *peripherals.CaptureResult `json:"capture,omitempty"`
	Peripherals *peripherals.FleetSnapshot `json:"peripherals,omitempty"`
	Sources     []agentSource              `json:"sources,omitempty"`
	Trace       *agentTrace                `json:"trace,omitempty"`
	Error       string                     `json:"error,omitempty"`
}

type knowledgeDocument struct {
	ID      string
	Title   string
	Content string
}

type ObservationService struct {
	workdir          string
	defaultPrompt    string
	enableImageInput bool
	peripherals      *peripherals.Manager
	cameraTool       *cameraReadTool
	dialogueTools    []einotool.InvokableTool
	analyzer         VisionAnalyzer
	sessions         *sessionStore
}

func NewObservationService(workdir string, defaultPrompt string, enableImageInput bool, peripheralsManager *peripherals.Manager, analyzer VisionAnalyzer) (*ObservationService, error) {
	sessions, err := newSessionStore(workdir)
	if err != nil {
		return nil, err
	}
	dialogueTools, err := newDialogueTools(workdir, peripheralsManager)
	if err != nil {
		return nil, err
	}
	return &ObservationService{
		workdir:          workdir,
		defaultPrompt:    defaultPrompt,
		enableImageInput: enableImageInput,
		peripherals:      peripheralsManager,
		cameraTool:       newCameraReadTool(workdir, peripheralsManager),
		dialogueTools:    dialogueTools,
		analyzer:         analyzer,
		sessions:         sessions,
	}, nil
}

func (s *ObservationService) InspectPeripherals(ctx context.Context) peripherals.FleetSnapshot {
	return s.peripherals.InspectAll(ctx)
}

func (s *ObservationService) InspectPrimary(ctx context.Context) (peripherals.DeviceSnapshot, error) {
	return s.peripherals.InspectPrimary(ctx)
}

func (s *ObservationService) CapturePrimary(ctx context.Context) (*peripherals.CaptureResult, error) {
	result, _, err := s.cameraTool.Capture(ctx)
	return result, err
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
	if !s.enableImageInput {
		return analyzeResponse{
			Capture:     capture,
			Peripherals: s.snapshotPtr(ctx),
			Error:       "configured model does not support image input; set JETSON_ENABLE_IMAGE_INPUT=true only for a vision-capable model",
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
			{ID: "rag", Name: "Retrieval", Description: "Retrieves relevant operational notes and current device context before answering."},
			{ID: "react", Name: "Action Planning", Description: "Plans whether to capture a fresh frame, reuse the latest frame, and attach peripheral status."},
			{ID: "tools", Name: "Tool Calling", Description: "Can call registered tools such as camera reads and smoke-test verification during dialogue."},
		},
	}
}

func (s *ObservationService) AgentChat(ctx context.Context, req agentChatRequest) (agentChatResponse, error) {
	session, err := s.loadSession(req)
	if err != nil {
		return agentChatResponse{}, err
	}

	intent := inferIntent(req.Message)
	compareRequest := requestsImageComparison(strings.ToLower(req.Message))
	previousSessionImagePath := session.latestImagePath()
	req = s.applyActionHeuristics(req)
	visionInputDisabled := !s.enableImageInput && (req.CaptureFresh || req.UseLatestImage)
	if visionInputDisabled {
		req.CaptureFresh = false
		req.UseLatestImage = false
	}
	trace := &agentTrace{
		Intent:  intent,
		Actions: actionTrace(req),
	}

	prompt := strings.TrimSpace(req.Message)
	if prompt == "" {
		prompt = s.defaultPrompt
	}

	var capture *peripherals.CaptureResult
	var imagePath string
	var imagePaths []string

	switch {
	case req.CaptureFresh:
		var call toolCallRecord
		capture, call, err = s.cameraTool.Capture(ctx)
		trace.ToolCalls = append(trace.ToolCalls, call)
		if err != nil {
			return agentChatResponse{}, err
		}
		if capture.Error != "" || !capture.OK {
			return agentChatResponse{
				SessionID:   session.ID,
				Capture:     capture,
				Peripherals: s.snapshotPtr(ctx),
				Trace:       trace,
				Error:       "camera capture failed before agent chat",
			}, nil
		}
		imagePath = capture.Output
	case req.UseLatestImage:
		var call toolCallRecord
		imagePath, call, err = s.cameraTool.LatestImagePath()
		trace.ToolCalls = append(trace.ToolCalls, call)
		if err != nil {
			imagePath = ""
		}
	}
	if imagePath != "" {
		imagePaths = append(imagePaths, imagePath)
	}
	if compareRequest && previousSessionImagePath != "" && previousSessionImagePath != imagePath {
		if imagePath == "" {
			var call toolCallRecord
			capture, call, err = s.cameraTool.Capture(ctx)
			trace.ToolCalls = append(trace.ToolCalls, call)
			if err != nil {
				return agentChatResponse{}, err
			}
			if capture != nil && capture.OK {
				imagePath = capture.Output
				imagePaths = []string{previousSessionImagePath, imagePath}
			}
		} else {
			imagePaths = []string{previousSessionImagePath, imagePath}
		}
	}

	var snapshot *peripherals.FleetSnapshot
	if req.IncludeSnapshot || req.CaptureFresh {
		snapshot = s.snapshotPtr(ctx)
	}

	sources := s.retrieveSources(prompt, req.History, snapshot)
	trace.RetrievedIDs = sourceIDs(sources)

	chatPrompt := prompt
	if snapshot != nil {
		snapshotJSON, err := json.MarshalIndent(snapshot, "", "  ")
		if err == nil {
			chatPrompt = fmt.Sprintf("%s\n\nPeripheral snapshot:\n%s", prompt, string(snapshotJSON))
		}
	}
	if len(sources) > 0 {
		var builder strings.Builder
		builder.WriteString(chatPrompt)
		builder.WriteString("\n\nRetrieved context:\n")
		for _, source := range sources {
			builder.WriteString("- ")
			builder.WriteString(source.Title)
			builder.WriteString(": ")
			builder.WriteString(source.Snippet)
			builder.WriteString("\n")
		}
		builder.WriteString("\nAnswer directly, cite relevant operational context when useful, and mention uncertainty explicitly.")
		chatPrompt = builder.String()
	}
	if compareRequest && len(imagePaths) >= 2 {
		chatPrompt += "\n\nImage comparison mode:\nThe first attached image is the earlier frame from this session. The last attached image is the newest frame. Compare them and describe the differences directly."
	}
	if visionInputDisabled {
		chatPrompt += "\n\nModel constraint:\nThe current model configuration does not support direct image input. Do not claim to see or analyze the camera frame unless a tool output explicitly describes it."
	}

	chatCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	chatResult, err := s.analyzer.Chat(chatCtx, agent.ChatRequest{
		History:          s.chatHistory(session),
		Prompt:           chatPrompt,
		ImagePath:        imagePath,
		ImagePaths:       imagePaths,
		Tools:            s.dialogueTools,
		ForcedToolNames:  forcedToolNames(prompt),
		MaxToolCallLoops: 4,
	})
	if err != nil {
		return agentChatResponse{
			SessionID:   session.ID,
			Capture:     capture,
			Peripherals: snapshot,
			Sources:     sources,
			Trace:       trace,
			Error:       err.Error(),
		}, nil
	}
	for _, toolTrace := range chatResult.ToolCalls {
		trace.ToolCalls = append(trace.ToolCalls, toolTraceRecord(toolTrace))
	}
	reply := chatResult.Content
	reply, capture, recoveryTrace, recovered := s.autoRecoverVisualReply(ctx, session, prompt, chatPrompt, reply, capture, imagePath, snapshot)
	if recovered {
		trace.ToolCalls = append(trace.ToolCalls, recoveryTrace...)
	}
	if imagePath != "" {
		session.rememberImage(imagePath, "prechat_capture")
	}
	for _, path := range imagePathsFromToolRecords(trace.ToolCalls) {
		session.rememberImage(path, "tool_capture")
	}

	session.Messages = append(session.Messages,
		agent.ConversationTurn{Role: "user", Content: prompt},
		agent.ConversationTurn{Role: "assistant", Content: reply},
	)
	session.Summary, session.Messages = summarizeHistory(session.Summary, session.Messages)
	if err := s.sessions.Save(session); err != nil {
		return agentChatResponse{}, err
	}

	return agentChatResponse{
		SessionID:   session.ID,
		Reply:       reply,
		Capture:     capture,
		Peripherals: snapshot,
		Sources:     sources,
		Trace:       trace,
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

func (s *ObservationService) loadSession(req agentChatRequest) (*conversationSession, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		session := s.sessions.New()
		if len(req.History) > 0 {
			session.Messages = append(session.Messages, req.History...)
			session.Summary, session.Messages = summarizeHistory(session.Summary, session.Messages)
		}
		return session, nil
	}

	session, err := s.sessions.Load(req.SessionID)
	if err != nil {
		if os.IsNotExist(err) {
			session = s.sessions.New()
			session.ID = req.SessionID
			session.Messages = append(session.Messages, req.History...)
			session.Summary, session.Messages = summarizeHistory(session.Summary, session.Messages)
			return session, nil
		}
		return nil, err
	}
	return session, nil
}

func (s *ObservationService) chatHistory(session *conversationSession) []agent.ConversationTurn {
	history := make([]agent.ConversationTurn, 0, len(session.Messages)+1)
	if strings.TrimSpace(session.Summary) != "" {
		history = append(history, agent.ConversationTurn{
			Role:    "user",
			Content: session.Summary,
		})
	}
	history = append(history, session.Messages...)
	return history
}

func (s *ObservationService) applyActionHeuristics(req agentChatRequest) agentChatRequest {
	lower := strings.ToLower(req.Message)
	if containsAny(lower, "camera_read", "调用camera_read", "call camera_read", "ros2_topic_read", "调用ros2_topic_read", "call ros2_topic_read") {
		if containsAny(lower, "sensor", "peripheral", "radar", "status", "depth", "外设", "雷达", "状态", "相机") {
			req.IncludeSnapshot = true
		}
		return req
	}
	if requestsFreshVisualObservation(lower) {
		req.CaptureFresh = true
		req.UseLatestImage = false
	}
	if requestsLatestImageReuse(lower) && !req.CaptureFresh {
		req.UseLatestImage = true
	}
	if containsAny(lower, "sensor", "peripheral", "radar", "status", "depth", "外设", "雷达", "状态", "相机") {
		req.IncludeSnapshot = true
	}
	return req
}

func inferIntent(message string) string {
	lower := strings.ToLower(message)
	switch {
	case containsAny(lower, "status", "health", "外设", "状态", "radar", "sensor"):
		return "peripheral_status"
	case containsAny(lower, "what", "describe", "scene", "画面", "前方", "obstacle", "障碍"):
		return "scene_understanding"
	case containsAny(lower, "compare", "difference", "一致", "矛盾", "disagree"):
		return "cross_sensor_reasoning"
	default:
		return "general_assistant"
	}
}

func actionTrace(req agentChatRequest) []agentAction {
	return []agentAction{
		{ID: "capture_fresh", Label: "Fresh Capture", Description: "Capture a new frame before answering.", Enabled: req.CaptureFresh},
		{ID: "use_latest_image", Label: "Latest Image", Description: "Reuse the most recent captured image.", Enabled: req.UseLatestImage},
		{ID: "include_snapshot", Label: "Peripheral Snapshot", Description: "Attach the current peripheral fleet snapshot.", Enabled: req.IncludeSnapshot},
	}
}

func (s *ObservationService) retrieveSources(prompt string, history []agent.ConversationTurn, snapshot *peripherals.FleetSnapshot) []agentSource {
	queryTerms := tokenize(prompt)
	for _, turn := range history {
		queryTerms = append(queryTerms, tokenize(turn.Content)...)
	}

	docs := s.knowledgeDocs(snapshot)
	sources := make([]agentSource, 0, len(docs))
	for _, doc := range docs {
		score := overlapScore(queryTerms, tokenize(doc.Content+" "+doc.Title))
		if score == 0 {
			continue
		}
		sources = append(sources, agentSource{
			ID:      doc.ID,
			Title:   doc.Title,
			Snippet: trimSnippet(doc.Content, 180),
			Score:   score,
		})
	}

	sort.Slice(sources, func(i int, j int) bool {
		if sources[i].Score == sources[j].Score {
			return sources[i].ID < sources[j].ID
		}
		return sources[i].Score > sources[j].Score
	})
	if len(sources) > 4 {
		sources = sources[:4]
	}
	return sources
}

func (s *ObservationService) knowledgeDocs(snapshot *peripherals.FleetSnapshot) []knowledgeDocument {
	docs := []knowledgeDocument{
		{ID: "cap-chat", Title: "Dialogue", Content: "The agent supports multi-turn dialogue using prior user and assistant turns."},
		{ID: "cap-vision", Title: "Vision Context", Content: "The agent can answer with the latest image or a freshly captured frame from the primary device."},
		{ID: "cap-peripherals", Title: "Peripheral Snapshot", Content: "The agent can include radar, depth camera, and other peripheral status in the answer context."},
		{ID: "playbook-uncertainty", Title: "Operational Playbook", Content: "When sensor evidence is incomplete or conflicting, the agent should mention uncertainty and avoid overclaiming."},
		{ID: "cap-tools", Title: "Tool Calling", Content: "The agent can call registered tools during dialogue, including camera_read for visual input and tool_call_smoke_test for deterministic validation."},
	}
	if snapshot != nil {
		for index, device := range snapshot.Devices {
			content := device.Summary
			if len(device.Metadata) > 0 {
				parts := make([]string, 0, len(device.Metadata))
				for key, value := range device.Metadata {
					parts = append(parts, key+"="+value)
				}
				sort.Strings(parts)
				content += ". metadata: " + strings.Join(parts, ", ")
			}
			for _, check := range device.Checks {
				content += ". " + check.Name + ": " + trimSnippet(check.Output, 120)
			}
			docs = append(docs, knowledgeDocument{
				ID:      "device-" + strconv.Itoa(index),
				Title:   device.Name + " (" + device.Kind + ")",
				Content: content,
			})
		}
	}
	return docs
}

func sourceIDs(sources []agentSource) []string {
	ids := make([]string, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.ID)
	}
	return ids
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func tokenize(text string) []string {
	fields := strings.Fields(strings.NewReplacer(",", " ", ".", " ", "\n", " ", ":", " ", ";", " ").Replace(strings.ToLower(text)))
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if len(trimmed) >= 2 {
			terms = append(terms, trimmed)
		}
	}
	return terms
}

func overlapScore(left []string, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(right))
	for _, item := range right {
		set[item] = struct{}{}
	}
	score := 0
	for _, item := range left {
		if _, ok := set[item]; ok {
			score++
		}
	}
	return score
}

func trimSnippet(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func requestsFreshVisualObservation(text string) bool {
	if strings.HasPrefix(strings.TrimSpace(text), "/camera") {
		return true
	}
	if !containsAny(text, "camera", "camera view", "camera feed", "frame", "scene", "image", "look", "observe", "what do you see", "see", "watch", "前方", "画面", "图像", "图片", "镜头", "相机", "摄像头", "观察", "看看", "看一下", "看下", "看一眼", "描述一下", "当前场景", "当前画面", "现场") {
		return false
	}
	if containsAny(text, "status", "health", "配置", "参数", "外设状态", "camera status", "device status", "连接状态") {
		return false
	}
	return containsAny(text,
		"observe", "look", "watch", "see", "what do you see", "describe", "inspect",
		"now", "current", "live", "fresh", "实时", "现在", "当前", "观察", "看看", "看一下", "看下", "看一眼", "描述", "前方", "周围", "现场",
	)
}

func requestsLatestImageReuse(text string) bool {
	return containsAny(text,
		"latest image", "latest frame", "last frame", "reuse latest", "use latest image",
		"最新图片", "最新图像", "最新一帧", "上一帧", "最近抓拍", "复用最新",
	)
}

func requestsImageComparison(text string) bool {
	if strings.HasPrefix(strings.TrimSpace(text), "/compare") {
		return true
	}
	return containsAny(text,
		"compare", "difference", "different", "change", "changed", "delta", "vs", "versus",
		"对比", "比较", "区别", "差异", "变化", "变了", "前后", "两张", "上一张", "上一帧", "前一张", "前一帧",
	)
}

func looksLikeToolDeferral(reply string) bool {
	lower := strings.ToLower(strings.TrimSpace(reply))
	if lower == "" {
		return false
	}
	return containsAny(lower,
		"need to call", "need to use", "i need to use", "i need to call", "must call", "have to call",
		"需要调用", "需要先调用", "需要使用", "我需要调用", "我需要先调用", "我将调用", "先调用工具", "先获取画面", "无法直接查看",
	)
}

func (s *ObservationService) autoRecoverVisualReply(
	ctx context.Context,
	session *conversationSession,
	userPrompt string,
	chatPrompt string,
	reply string,
	capture *peripherals.CaptureResult,
	imagePath string,
	snapshot *peripherals.FleetSnapshot,
) (string, *peripherals.CaptureResult, []toolCallRecord, bool) {
	if !requestsFreshVisualObservation(strings.ToLower(userPrompt)) || !looksLikeToolDeferral(reply) {
		return reply, capture, nil, false
	}

	recoveryTrace := make([]toolCallRecord, 0, 1)
	recoveryImagePath := strings.TrimSpace(imagePath)
	recoveryCapture := capture

	if recoveryImagePath == "" && recoveryCapture != nil && recoveryCapture.OK {
		recoveryImagePath = strings.TrimSpace(recoveryCapture.Output)
	}

	if recoveryImagePath == "" {
		var call toolCallRecord
		path, latestCall, err := s.cameraTool.LatestImagePath()
		if err == nil && strings.TrimSpace(path) != "" {
			recoveryImagePath = path
			latestCall.Name = "camera_read"
			recoveryTrace = append(recoveryTrace, latestCall)
		} else {
			recoveryCapture, call, err = s.cameraTool.Capture(ctx)
			call.Name = "camera_read"
			if call.Input == nil {
				call.Input = map[string]string{}
			}
			call.Input["reason"] = "auto_retry_visual_observation"
			recoveryTrace = append(recoveryTrace, call)
			if err != nil || recoveryCapture == nil || !recoveryCapture.OK || strings.TrimSpace(recoveryCapture.Output) == "" {
				return reply, capture, recoveryTrace, false
			}
			recoveryImagePath = recoveryCapture.Output
		}
	}

	recoveryPrompt := buildDirectVisualAnswerPrompt(chatPrompt, snapshot)
	chatCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	recoveredResult, err := s.analyzer.Chat(chatCtx, agent.ChatRequest{
		History:          s.chatHistory(session),
		Prompt:           recoveryPrompt,
		ImagePath:        recoveryImagePath,
		Tools:            nil,
		ForcedToolNames:  nil,
		MaxToolCallLoops: 1,
	})
	if err != nil || strings.TrimSpace(recoveredResult.Content) == "" {
		return reply, capture, recoveryTrace, false
	}

	return recoveredResult.Content, recoveryCapture, recoveryTrace, true
}

func buildDirectVisualAnswerPrompt(chatPrompt string, snapshot *peripherals.FleetSnapshot) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(chatPrompt))
	builder.WriteString("\n\nA current camera frame is already attached.")
	builder.WriteString("\nAnswer directly from the image now.")
	builder.WriteString("\nDo not say that you need to call a tool or fetch another image.")
	builder.WriteString("\nDo not mention tool invocation unless it is directly relevant to the answer.")
	if snapshot != nil {
		builder.WriteString("\nUse peripheral status only to qualify the answer when helpful.")
	}
	return builder.String()
}

func imagePathsFromToolRecords(records []toolCallRecord) []string {
	paths := make([]string, 0, len(records))
	for _, record := range records {
		if record.Name != "camera_read" && record.Name != "ros2_topic_read" {
			continue
		}
		if record.Output == nil {
			continue
		}
		if path := strings.TrimSpace(record.Output["output"]); path != "" {
			paths = append(paths, path)
		}
		if raw := strings.TrimSpace(record.Output["result"]); raw != "" {
			var payload struct {
				ImagePath string `json:"image_path"`
			}
			if err := json.Unmarshal([]byte(raw), &payload); err == nil && strings.TrimSpace(payload.ImagePath) != "" {
				paths = append(paths, strings.TrimSpace(payload.ImagePath))
			}
		}
	}
	return paths
}

func forcedToolNames(prompt string) []string {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	switch {
	case strings.HasPrefix(lower, "/tooltest"):
		return []string{"tool_call_smoke_test"}
	case strings.HasPrefix(lower, "/camera"):
		return []string{"camera_read"}
	case strings.HasPrefix(lower, "/ros2"):
		return []string{"ros2_topic_read"}
	case strings.Contains(lower, "tool_call_smoke_test"):
		return []string{"tool_call_smoke_test"}
	case strings.Contains(lower, "tool call smoke test"):
		return []string{"tool_call_smoke_test"}
	case strings.Contains(lower, "camera_read"):
		return []string{"camera_read"}
	case strings.Contains(lower, "call camera_read"):
		return []string{"camera_read"}
	case strings.Contains(lower, "调用camera_read"):
		return []string{"camera_read"}
	case strings.Contains(lower, "ros2_topic_read"):
		return []string{"ros2_topic_read"}
	case strings.Contains(lower, "call ros2_topic_read"):
		return []string{"ros2_topic_read"}
	case strings.Contains(lower, "调用ros2_topic_read"):
		return []string{"ros2_topic_read"}
	default:
		return nil
	}
}

func toolTraceRecord(trace agent.ToolCallTrace) toolCallRecord {
	record := toolCallRecord{
		Name: trace.Name,
	}
	if strings.TrimSpace(trace.Arguments) != "" {
		record.Input = map[string]string{
			"arguments": trace.Arguments,
		}
	}
	if strings.TrimSpace(trace.Result) != "" {
		record.Output = map[string]string{
			"result": trace.Result,
		}
	}
	return record
}
