package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ROS2TopicCaptureConfig struct {
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
		builder.WriteString("source ")
		builder.WriteString(shellQuote(trimmed))
		builder.WriteString(" >/dev/null 2>&1; ")
	}
	builder.WriteString("exec python3 ")
	builder.WriteString(shellQuote(cfg.ScriptPath))
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func ValidateROS2TopicCaptureConfig(cfg ROS2TopicCaptureConfig) error {
	switch {
	case strings.TrimSpace(cfg.ScriptPath) == "":
		return fmt.Errorf("ros2 capture script path is empty")
	case strings.TrimSpace(cfg.Topic) == "":
		return fmt.Errorf("ros2 capture topic is empty")
	case strings.TrimSpace(cfg.MessageType) == "":
		return fmt.Errorf("ros2 capture message type is empty")
	default:
		return nil
	}
}
