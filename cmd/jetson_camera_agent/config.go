package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type appConfig struct {
	ListenAddr       string
	BaseURL          string
	APIKey           string
	ModelName        string
	SystemPrompt     string
	DefaultPrompt    string
	UITitle          string
	UIDescription    string
	Workdir          string
	CameraScript     string
	PeripheralConfig string
}

func loadConfig() (appConfig, error) {
	if err := loadDotEnv(".env"); err != nil {
		return appConfig{}, err
	}

	cfg := appConfig{
		ListenAddr:       requiredEnv("JETSON_AGENT_LISTEN_ADDR"),
		BaseURL:          requiredEnv("OPENAI_BASE_URL"),
		APIKey:           requiredEnv("OPENAI_API_KEY"),
		ModelName:        requiredEnv("OPENAI_MODEL_NAME"),
		SystemPrompt:     requiredEnv("VISION_SYSTEM_PROMPT"),
		DefaultPrompt:    requiredEnv("JETSON_DEFAULT_PROMPT"),
		UITitle:          requiredEnv("JETSON_UI_TITLE"),
		UIDescription:    requiredEnv("JETSON_UI_DESCRIPTION"),
		Workdir:          envOrDefault("JETSON_AGENT_WORKDIR", filepath.Join(os.TempDir(), "jetson-camera-agent")),
		CameraScript:     envOrDefault("JETSON_CAMERA_SCRIPT", "./scripts/capture_zed_frame.py"),
		PeripheralConfig: envOrDefault("JETSON_PERIPHERAL_CONFIG", "./configs/peripherals.json"),
	}

	if missing := cfg.missingKeys(); len(missing) > 0 {
		return appConfig{}, fmt.Errorf("missing required config keys: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

func (c appConfig) missingKeys() []string {
	var missing []string
	checks := map[string]string{
		"JETSON_AGENT_LISTEN_ADDR": c.ListenAddr,
		"OPENAI_BASE_URL":          c.BaseURL,
		"OPENAI_API_KEY":           c.APIKey,
		"OPENAI_MODEL_NAME":        c.ModelName,
		"VISION_SYSTEM_PROMPT":     c.SystemPrompt,
		"JETSON_DEFAULT_PROMPT":    c.DefaultPrompt,
		"JETSON_UI_TITLE":          c.UITitle,
		"JETSON_UI_DESCRIPTION":    c.UIDescription,
	}
	for key, value := range checks {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid .env line: %q", line)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			return fmt.Errorf("invalid .env key in line: %q", line)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}

func requiredEnv(key string) string {
	value, _ := os.LookupEnv(key)
	return strings.TrimSpace(value)
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
