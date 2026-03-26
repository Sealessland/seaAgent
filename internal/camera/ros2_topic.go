package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultROS2CaptureBinary = "./ros2_topic_capture"

type ROS2TopicCaptureConfig struct {
	BinaryPath     string
	ScriptPath     string
	Topic          string
	MessageType    string
	Encoding       string
	TimeoutSeconds int
	ROSSetup       []string
}

func CaptureROS2Topic(ctx context.Context, cfg ROS2TopicCaptureConfig, outputPath string) (*CaptureResult, error) {
	command := buildROS2CaptureCommand(cfg, outputPath)
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))

	var result CaptureResult
	if text != "" {
		jsonText := extractLastJSON(text)
		if jsonText != "" {
			_ = json.Unmarshal([]byte(jsonText), &result)
		}
		result.RawOutput = text
	}

	if err != nil {
		if result.Error == "" {
			result.Error = err.Error()
		}
		if result.RawOutput == "" {
			result.RawOutput = text
		}
		return &result, nil
	}

	return &result, nil
}

func buildROS2CaptureCommand(cfg ROS2TopicCaptureConfig, outputPath string) string {
	var builder strings.Builder
	builder.WriteString("set -e; ")
	for _, entry := range cfg.ROSSetup {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		trimmed = ExpandHomePath(trimmed)
		builder.WriteString("source ")
		builder.WriteString(shellQuote(trimmed))
		builder.WriteString(" >/dev/null 2>&1; ")
	}
	if UsesLegacyROS2TopicScript(cfg) {
		builder.WriteString("exec python3 ")
		builder.WriteString(shellQuote(ExpandHomePath(cfg.ScriptPath)))
	} else {
		builder.WriteString("exec ")
		builder.WriteString(shellQuote(ResolveROS2TopicCapturePath(cfg)))
	}
	builder.WriteString(" --output ")
	builder.WriteString(shellQuote(outputPath))
	builder.WriteString(" --topic ")
	builder.WriteString(shellQuote(cfg.Topic))
	builder.WriteString(" --message-type ")
	builder.WriteString(shellQuote(cfg.MessageType))
	if strings.TrimSpace(cfg.Encoding) != "" {
		builder.WriteString(" --encoding ")
		builder.WriteString(shellQuote(cfg.Encoding))
	}
	if cfg.TimeoutSeconds > 0 {
		builder.WriteString(" --timeout-seconds ")
		builder.WriteString(strconv.Itoa(cfg.TimeoutSeconds))
	}
	return builder.String()
}

func DefaultROS2CaptureBinaryPath() string {
	return defaultROS2CaptureBinary
}

func UsesLegacyROS2TopicScript(cfg ROS2TopicCaptureConfig) bool {
	path := strings.TrimSpace(cfg.ScriptPath)
	return path != "" && strings.HasSuffix(strings.ToLower(path), ".py")
}

func ResolveROS2TopicCapturePath(cfg ROS2TopicCaptureConfig) string {
	if path := strings.TrimSpace(cfg.BinaryPath); path != "" {
		return ExpandHomePath(path)
	}
	if path := strings.TrimSpace(cfg.ScriptPath); path != "" && !strings.HasSuffix(strings.ToLower(path), ".py") {
		return ExpandHomePath(path)
	}
	return defaultROS2CaptureBinary
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func ExpandHomePath(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

func ValidateROS2TopicCaptureConfig(cfg ROS2TopicCaptureConfig) error {
	switch {
	case strings.TrimSpace(cfg.Topic) == "":
		return fmt.Errorf("ros2 capture topic is empty")
	case strings.TrimSpace(cfg.MessageType) == "":
		return fmt.Errorf("ros2 capture message type is empty")
	default:
		return nil
	}
}
