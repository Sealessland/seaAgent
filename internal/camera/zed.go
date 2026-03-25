package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type CaptureResult struct {
	OK        bool   `json:"ok"`
	Output    string `json:"output,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	CameraSN  string `json:"camera_sn,omitempty"`
	Error     string `json:"error,omitempty"`
	RawOutput string `json:"raw_output,omitempty"`
}

type Status struct {
	ZedUSB       string `json:"zed_usb"`
	ZedTopics    string `json:"zed_topics"`
	ZedLogTail   string `json:"zed_log_tail"`
	PyzedProbe   string `json:"pyzed_probe"`
	Summary      string `json:"summary"`
}

func CaptureWithPython(ctx context.Context, scriptPath string, outputPath string) (*CaptureResult, error) {
	cmd := exec.CommandContext(ctx, "python3", scriptPath, "--output", outputPath)
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

func extractLastJSON(text string) string {
	start := strings.LastIndex(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return text[start : end+1]
}

func Inspect(ctx context.Context) Status {
	status := Status{
		ZedUSB:     run(ctx, "bash", "-lc", "lsusb | grep -i zed || true"),
		ZedTopics:  run(ctx, "bash", "-lc", "source /opt/ros/humble/setup.bash >/dev/null 2>&1; source ~/zed_ws/install/setup.bash >/dev/null 2>&1; ros2 topic list 2>/dev/null | grep '^/zed' || true"),
		ZedLogTail: run(ctx, "bash", "-lc", "tail -n 40 /tmp/zed_launch.log 2>/dev/null || true"),
		PyzedProbe: run(ctx, "python3", "-c", "import pyzed.sl as sl; cam=sl.Camera(); init=sl.InitParameters(); init.camera_resolution=sl.RESOLUTION.HD720; init.camera_fps=15; init.depth_mode=sl.DEPTH_MODE.NONE; print(cam.open(init))"),
	}

	lowerLog := strings.ToLower(status.ZedLogTail + " " + status.PyzedProbe)
	switch {
	case strings.TrimSpace(status.ZedUSB) == "":
		status.Summary = "ZED device is not visible on USB."
	case strings.Contains(lowerLog, "camera stream failed to start"):
		status.Summary = "ZED is visible on USB, but the SDK stream fails to start before any frame reaches the agent."
	case strings.TrimSpace(status.ZedTopics) != "":
		status.Summary = "ZED ROS2 topics are visible. Frame transport may be available."
	default:
		status.Summary = "ZED is visible, but no active ROS2 topics or successful SDK grab were detected."
	}

	return status
}

func run(parent context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return fmt.Sprintf("command failed: %v", err)
	}
	if text == "" {
		return "(empty)"
	}
	return text
}
