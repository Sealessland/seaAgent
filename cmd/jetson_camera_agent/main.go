package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/agent"
	"eino-vlm-agent-demo/internal/camera"
)

//go:embed static/index.html
var staticFS embed.FS

type analyzeResponse struct {
	Result  string               `json:"result,omitempty"`
	Capture *camera.CaptureResult `json:"capture,omitempty"`
	Error   string               `json:"error,omitempty"`
}

type healthResponse struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

func main() {
	addr := envOrDefault("JETSON_AGENT_LISTEN_ADDR", "127.0.0.1:18080")
	baseURL := envOrDefault("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1")
	apiKey := envOrDefault("OPENAI_API_KEY", "EMPTY")
	modelName := envOrDefault("OPENAI_MODEL_NAME", "Qwen3.5-2B-local")
	systemPrompt := envOrDefault("VISION_SYSTEM_PROMPT", "You are a concise Jetson vision agent. Describe visible facts first, then mention robotics or safety relevance in one short line.")
	workdir := envOrDefault("JETSON_AGENT_WORKDIR", filepath.Join(os.TempDir(), "jetson-camera-agent"))
	scriptPath := envOrDefault("JETSON_CAMERA_SCRIPT", "./scripts/capture_zed_frame.py")

	if err := os.MkdirAll(workdir, 0o755); err != nil {
		log.Fatalf("create workdir: %v", err)
	}

	visionAgent, err := agent.NewVisionAgent(context.Background(), agent.Config{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Model:        modelName,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		log.Fatalf("init vision agent failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/health", makeHealthHandler(baseURL))
	mux.HandleFunc("/api/camera/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, camera.Inspect(r.Context()))
	})
	mux.HandleFunc("/api/camera/capture", func(w http.ResponseWriter, r *http.Request) {
		result := captureFrame(r.Context(), scriptPath, workdir)
		if result.Error != "" || !result.OK {
			writeJSON(w, http.StatusBadGateway, result)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/api/camera/latest.jpg", func(w http.ResponseWriter, r *http.Request) {
		latestPath, err := latestCapturePath(workdir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, latestPath)
	})
	mux.HandleFunc("/api/capture-and-analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		prompt := "Describe the current camera view and mention anything relevant for robotics or safety."
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if strings.TrimSpace(string(body)) != "" {
			var req struct {
				Prompt string `json:"prompt"`
			}
			if err := json.Unmarshal(body, &req); err == nil && strings.TrimSpace(req.Prompt) != "" {
				prompt = req.Prompt
			}
		}

		capture := captureFrame(r.Context(), scriptPath, workdir)
		if capture.Error != "" || !capture.OK {
			writeJSON(w, http.StatusBadGateway, analyzeResponse{
				Capture: capture,
				Error:   "camera capture failed before agent inference",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		result, err := visionAgent.AnalyzeImage(ctx, capture.Output, prompt)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, analyzeResponse{
				Capture: capture,
				Error:   err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, analyzeResponse{
			Capture: capture,
			Result:  result,
		})
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("jetson camera agent listening on http://%s", addr)
	log.Printf("target VLM endpoint: %s, model: %s", baseURL, modelName)
	log.Printf("camera capture script: %s", scriptPath)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func makeHealthHandler(baseURL string) http.HandlerFunc {
	client := &http.Client{Timeout: 15 * time.Second}
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"

	return func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, modelsURL, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, healthResponse{OK: false, Body: err.Error()})
			return
		}
		resp, err := client.Do(req)
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
}

func captureFrame(ctx context.Context, scriptPath string, workdir string) *camera.CaptureResult {
	filename := fmt.Sprintf("zed-%d.jpg", time.Now().UnixNano())
	outputPath := filepath.Join(workdir, filename)

	capture, err := camera.CaptureWithPython(ctx, scriptPath, outputPath)
	if err != nil {
		return &camera.CaptureResult{Error: err.Error()}
	}
	return capture
}

func latestCapturePath(workdir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(workdir, "zed-*.jpg"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no captured image is available yet")
	}

	sort.Slice(matches, func(i int, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] > matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})

	return matches[0], nil
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

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
