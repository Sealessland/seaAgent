package jetsonagent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type appConfig struct {
	ListenAddr       string
	DebugListenAddr  string
	BaseURL          string
	APIKey           string
	ModelName        string
	EnableImageInput bool
	SystemPrompt     string
	SystemPromptFile string
	DefaultPrompt    string
	UITitle          string
	UIDescription    string
	Workdir          string
	PeripheralConfig string
	FrontendDistDir  string
	DebugDistDir     string
}

func loadConfig() (appConfig, error) {
	if err := loadDotEnv(".env"); err != nil {
		return appConfig{}, err
	}

	cfg := appConfig{
		ListenAddr:       requiredEnv("JETSON_AGENT_LISTEN_ADDR"),
		DebugListenAddr:  envOrDefault("JETSON_DEBUG_LISTEN_ADDR", "127.0.0.1:18081"),
		BaseURL:          requiredEnv("OPENAI_BASE_URL"),
		APIKey:           requiredEnv("OPENAI_API_KEY"),
		ModelName:        requiredEnv("OPENAI_MODEL_NAME"),
		EnableImageInput: envBoolOrDefault("JETSON_ENABLE_IMAGE_INPUT", true),
		SystemPrompt:     strings.TrimSpace(os.Getenv("VISION_SYSTEM_PROMPT")),
		SystemPromptFile: envOrDefault("VISION_SYSTEM_PROMPT_FILE", "./prompts/system.txt"),
		DefaultPrompt:    requiredEnv("JETSON_DEFAULT_PROMPT"),
		UITitle:          requiredEnv("JETSON_UI_TITLE"),
		UIDescription:    requiredEnv("JETSON_UI_DESCRIPTION"),
		Workdir:          envOrDefault("JETSON_AGENT_WORKDIR", filepath.Join(os.TempDir(), "jetson-camera-agent")),
		PeripheralConfig: envOrDefault("JETSON_PERIPHERAL_CONFIG", "./configs/peripherals.json"),
		FrontendDistDir:  envOrDefault("JETSON_FRONTEND_DIST_DIR", "./front-end/dist"),
		DebugDistDir:     envOrDefault("JETSON_DEBUG_DIST_DIR", "./debug-front-end/dist"),
	}

	if missing := cfg.missingKeys(); len(missing) > 0 {
		return appConfig{}, fmt.Errorf("missing required config keys: %s", strings.Join(missing, ", "))
	}

	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		content, err := os.ReadFile(cfg.SystemPromptFile)
		if err != nil {
			return appConfig{}, fmt.Errorf("read system prompt file %s: %w", cfg.SystemPromptFile, err)
		}
		cfg.SystemPrompt = strings.TrimSpace(string(content))
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		return appConfig{}, fmt.Errorf("system prompt is empty")
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

func envBoolOrDefault(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
