package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/peripherals"
)

type analyzeResponse struct {
	Result      string                     `json:"result,omitempty"`
	Capture     *peripherals.CaptureResult `json:"capture,omitempty"`
	Peripherals *peripherals.FleetSnapshot `json:"peripherals,omitempty"`
	Error       string                     `json:"error,omitempty"`
}

type uiConfigResponse struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	DefaultPrompt string `json:"default_prompt"`
}

type healthResponse struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

func (app *application) handleUIConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, uiConfigResponse{
		Title:         app.cfg.UITitle,
		Description:   app.cfg.UIDescription,
		DefaultPrompt: app.cfg.DefaultPrompt,
	})
}

func (app *application) handleHealth(w http.ResponseWriter, r *http.Request) {
	modelsURL := strings.TrimRight(app.cfg.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, modelsURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, healthResponse{OK: false, Body: err.Error()})
		return
	}
	req.Header.Set("Authorization", "Bearer "+app.cfg.APIKey)

	resp, err := app.healthCheck.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, healthResponse{OK: false, Body: err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	writeJSON(w, http.StatusOK, healthResponse{
		OK:         resp.StatusCode == http.StatusOK,
		StatusCode: resp.StatusCode,
		Body:       string(body),
	})
}

func (app *application) handlePeripherals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.observation.InspectPeripherals(r.Context()))
}

func (app *application) handlePeripheralStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	sendSnapshot := func() bool {
		snapshot := app.observation.InspectPeripherals(r.Context())
		payload, err := json.Marshal(snapshot)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !sendSnapshot() {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !sendSnapshot() {
				return
			}
		}
	}
}

func (app *application) handleAgentCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, app.observation.AgentCapabilities())
}

func (app *application) handleAgentChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req agentChatRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, agentChatResponse{Error: "invalid request body"})
			return
		}
	}
	if app.cfg.EnableImageInput && !req.CaptureFresh && !req.UseLatestImage {
		req.UseLatestImage = true
	}
	if !req.IncludeSnapshot {
		req.IncludeSnapshot = true
	}

	result, err := app.observation.AgentChat(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, agentChatResponse{Error: err.Error()})
		return
	}
	if result.Error != "" {
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *application) handleAgentChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var req agentChatRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	if app.cfg.EnableImageInput && !req.CaptureFresh && !req.UseLatestImage {
		req.UseLatestImage = true
	}
	if !req.IncludeSnapshot {
		req.IncludeSnapshot = true
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeStreamEvent := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !writeStreamEvent("status", map[string]string{"state": "started"}) {
		return
	}

	result, err := app.observation.AgentChat(r.Context(), req)
	if err != nil {
		_ = writeStreamEvent("error", map[string]string{"error": err.Error()})
		return
	}
	if !writeStreamEvent("meta", map[string]string{"session_id": result.SessionID}) {
		return
	}
	if result.Error != "" {
		_ = writeStreamEvent("error", map[string]string{"error": result.Error})
		return
	}

	for _, chunk := range chunkText(result.Reply, 96) {
		if !writeStreamEvent("delta", map[string]string{"content": chunk}) {
			return
		}
	}
	_ = writeStreamEvent("done", result)
}

func (app *application) handleCameraStatus(w http.ResponseWriter, r *http.Request) {
	result, err := app.observation.InspectPrimary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *application) handleCameraCapture(w http.ResponseWriter, r *http.Request) {
	result, err := app.observation.CapturePrimary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, &peripherals.CaptureResult{Error: err.Error()})
		return
	}
	if result.Error != "" || !result.OK {
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *application) handleLatestCapture(w http.ResponseWriter, r *http.Request) {
	latestPath, err := app.observation.LatestCapturePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, latestPath)
}

func (app *application) handleCaptureAndAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	prompt := app.resolvePrompt(r)
	result, err := app.observation.AnalyzePrimary(r.Context(), prompt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, analyzeResponse{Error: err.Error()})
		return
	}
	if result.Error != "" {
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *application) resolvePrompt(r *http.Request) string {
	prompt := app.cfg.DefaultPrompt
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if strings.TrimSpace(string(body)) == "" {
		return prompt
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return prompt
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return prompt
	}
	return req.Prompt
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s in %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func chunkText(text string, size int) []string {
	if size <= 0 || len(text) <= size {
		return []string{text}
	}
	chunks := make([]string, 0, (len(text)/size)+1)
	for len(text) > size {
		chunks = append(chunks, text[:size])
		text = text[size:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}
