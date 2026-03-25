package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/agent"
	"eino-vlm-agent-demo/internal/peripherals"
)

//go:embed static
var staticFS embed.FS

type application struct {
	cfg         appConfig
	observation *ObservationService
	healthCheck *http.Client
}

func newServer(cfg appConfig) (*http.Server, error) {
	if err := os.MkdirAll(cfg.Workdir, 0o755); err != nil {
		return nil, err
	}

	peripheralsManager, err := loadPeripheralManager(cfg)
	if err != nil {
		return nil, err
	}

	visionAgent, err := agent.NewVisionAgent(context.Background(), agent.Config{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.ModelName,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err != nil {
		return nil, err
	}

	app := &application{
		cfg:         cfg,
		observation: NewObservationService(cfg.Workdir, cfg.DefaultPrompt, peripheralsManager, visionAgent),
		healthCheck: &http.Client{Timeout: 15 * time.Second},
	}

	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           loggingMiddleware(app.routes()),
		ReadHeaderTimeout: 10 * time.Second,
	}, nil
}

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", app.frontendHandler())
	mux.HandleFunc("/api/config", app.handleUIConfig)
	mux.HandleFunc("/api/health", app.handleHealth)
	mux.HandleFunc("/api/agent/capabilities", app.handleAgentCapabilities)
	mux.HandleFunc("/api/agent/chat", app.handleAgentChat)
	mux.HandleFunc("/api/peripherals", app.handlePeripherals)
	mux.HandleFunc("/api/camera/status", app.handleCameraStatus)
	mux.HandleFunc("/api/camera/capture", app.handleCameraCapture)
	mux.HandleFunc("/api/camera/latest.jpg", app.handleLatestCapture)
	mux.HandleFunc("/api/capture-and-analyze", app.handleCaptureAndAnalyze)
	return mux
}

func logStartup(cfg appConfig) {
	log.Printf("jetson camera agent listening on http://%s", cfg.ListenAddr)
	log.Printf("target VLM endpoint: %s, model: %s", cfg.BaseURL, cfg.ModelName)
	log.Printf("peripheral config: %s", cfg.PeripheralConfig)
	log.Printf("frontend dist dir: %s", cfg.FrontendDistDir)
}

func loadPeripheralManager(cfg appConfig) (*peripherals.Manager, error) {
	fleetCfg, err := peripherals.LoadConfig(cfg.PeripheralConfig)
	if err != nil {
		return nil, fmt.Errorf("load peripheral config %s: %w", cfg.PeripheralConfig, err)
	}
	return peripherals.NewManager(fleetCfg)
}

func (app *application) frontendHandler() http.Handler {
	if info, err := os.Stat(app.cfg.FrontendDistDir); err == nil && info.IsDir() {
		return spaFileServer(os.DirFS(app.cfg.FrontendDistDir))
	}

	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static assets unavailable", http.StatusInternalServerError)
		})
	}
	return spaFileServer(staticContent)
}

func spaFileServer(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		requestPath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if requestPath == "." || requestPath == "/" {
			requestPath = "index.html"
		}

		if _, err := fs.Stat(root, requestPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		indexFile, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "frontend entrypoint unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexFile)
	})
}
