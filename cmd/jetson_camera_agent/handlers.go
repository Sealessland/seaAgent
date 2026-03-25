package main

import (
	"encoding/json"
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
