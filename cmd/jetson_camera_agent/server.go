package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
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
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static assets unavailable", http.StatusInternalServerError)
		})
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticContent)))
	mux.HandleFunc("/api/config", app.handleUIConfig)
	mux.HandleFunc("/api/health", app.handleHealth)
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
}

func loadPeripheralManager(cfg appConfig) (*peripherals.Manager, error) {
	fleetCfg, err := peripherals.LoadConfig(cfg.PeripheralConfig)
	if err != nil {
		return nil, fmt.Errorf("load peripheral config %s: %w", cfg.PeripheralConfig, err)
	}
	return peripherals.NewManager(fleetCfg)
}
