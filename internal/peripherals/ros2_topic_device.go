package peripherals

import (
	"context"
	"fmt"
	"strings"

	"eino-vlm-agent-demo/internal/camera"
)

const defaultROS2CaptureScript = "./scripts/capture_ros2_topic_image.py"

type ros2TopicDevice struct {
	cfg DeviceConfig
}

func newROS2TopicDevice(cfg DeviceConfig) (Device, error) {
	device, err := newExecDevice(cfg)
	if err != nil {
		return nil, err
	}
	execDevice := device.(*execDevice)
	if execDevice.cfg.Capture == nil {
		return nil, fmt.Errorf("ros2_topic device %q requires capture config", cfg.Name)
	}
	if strings.TrimSpace(execDevice.cfg.Capture.Topic) == "" {
		return nil, fmt.Errorf("ros2_topic device %q requires capture.topic", cfg.Name)
	}
	if strings.TrimSpace(execDevice.cfg.Capture.MessageType) == "" {
		return nil, fmt.Errorf("ros2_topic device %q requires capture.message_type", cfg.Name)
	}
	if strings.TrimSpace(execDevice.cfg.Capture.Script) == "" {
		execDevice.cfg.Capture.Script = defaultROS2CaptureScript
	}
	if err := camera.ValidateROS2TopicCaptureConfig(camera.ROS2TopicCaptureConfig{
		ScriptPath:     execDevice.cfg.Capture.Script,
		Topic:          execDevice.cfg.Capture.Topic,
		MessageType:    execDevice.cfg.Capture.MessageType,
		Encoding:       execDevice.cfg.Capture.Encoding,
		TimeoutSeconds: execDevice.cfg.Capture.TimeoutSeconds,
		ROSSetup:       execDevice.cfg.Capture.ROSSetup,
	}); err != nil {
		return nil, fmt.Errorf("ros2_topic device %q invalid capture config: %w", cfg.Name, err)
	}
	if len(execDevice.cfg.Checks) == 0 {
		execDevice.cfg.Checks = defaultROS2TopicChecks(execDevice.cfg.Capture)
	}
	return &ros2TopicDevice{cfg: execDevice.cfg}, nil
}

func (d *ros2TopicDevice) Descriptor() DeviceDescriptor {
	return DeviceDescriptor{
		Name:            d.cfg.Name,
		Kind:            d.cfg.Kind,
		Driver:          d.cfg.Driver,
		SupportsCapture: true,
	}
}

func (d *ros2TopicDevice) Inspect(ctx context.Context) DeviceSnapshot {
	checkOutputs := make(map[string]string, len(d.cfg.Checks))
	checks := make([]CheckResult, 0, len(d.cfg.Checks))
	for _, check := range d.cfg.Checks {
		output := runCommand(ctx, check.Command)
		checkOutputs[check.Name] = output
		checks = append(checks, CheckResult{
			Name:   check.Name,
			Output: output,
		})
	}

	return DeviceSnapshot{
		Name:            d.cfg.Name,
		Kind:            d.cfg.Kind,
		Driver:          d.cfg.Driver,
		SupportsCapture: true,
		Summary:         summarizeROS2TopicChecks(d.cfg.Capture, checkOutputs),
		Checks:          checks,
		Metadata:        ros2TopicMetadata(d.cfg),
	}
}

func (d *ros2TopicDevice) Capture(ctx context.Context, outputPath string) (*CaptureResult, error) {
	return camera.CaptureROS2Topic(ctx, camera.ROS2TopicCaptureConfig{
		ScriptPath:     d.cfg.Capture.Script,
		Topic:          d.cfg.Capture.Topic,
		MessageType:    d.cfg.Capture.MessageType,
		Encoding:       d.cfg.Capture.Encoding,
		TimeoutSeconds: d.cfg.Capture.TimeoutSeconds,
		ROSSetup:       d.cfg.Capture.ROSSetup,
	}, outputPath)
}

func defaultROS2TopicChecks(capture *CaptureConfig) []CheckConfig {
	if capture == nil {
		return nil
	}
	return []CheckConfig{
		{
			Name:    "topic_list",
			Command: ros2ShellCommand(capture.ROSSetup, "ros2 topic list 2>/dev/null | grep '^"+shellSingleQuoteForGrep(capture.Topic)+"$' || true"),
		},
		{
			Name:    "topic_info",
			Command: ros2ShellCommand(capture.ROSSetup, "ros2 topic info "+shellQuote(capture.Topic)+" 2>/dev/null || true"),
		},
		{
			Name:    "python_deps",
			Command: ros2ShellCommand(capture.ROSSetup, "python3 -c \"import rclpy, cv_bridge, sensor_msgs.msg, numpy, cv2; print('ros2_python_ok')\" 2>/dev/null || true"),
		},
	}
}

func summarizeROS2TopicChecks(capture *CaptureConfig, outputs map[string]string) string {
	if capture == nil {
		return "ROS2 topic capture is not configured."
	}
	switch {
	case strings.TrimSpace(outputs["python_deps"]) == "" || outputs["python_deps"] == "(empty)":
		return "ROS2 topic path is configured, but Python ROS2 image dependencies are unavailable."
	case strings.TrimSpace(outputs["topic_list"]) == "" || outputs["topic_list"] == "(empty)":
		return "ROS2 image topic is configured, but the topic is not currently visible in the ROS graph."
	case strings.TrimSpace(outputs["topic_info"]) != "" && outputs["topic_info"] != "(empty)":
		return "ROS2 image topic is visible and capture can be attempted through a subscriber."
	default:
		return "ROS2 topic capture is configured, but topic metadata is incomplete."
	}
}

func ros2TopicMetadata(cfg DeviceConfig) map[string]string {
	metadata := make(map[string]string, len(cfg.Metadata)+4)
	for key, value := range cfg.Metadata {
		metadata[key] = value
	}
	if cfg.Capture != nil {
		metadata["topic"] = cfg.Capture.Topic
		metadata["message_type"] = cfg.Capture.MessageType
		if strings.TrimSpace(cfg.Capture.Encoding) != "" {
			metadata["encoding"] = cfg.Capture.Encoding
		}
		if cfg.Capture.TimeoutSeconds > 0 {
			metadata["timeout_seconds"] = fmt.Sprintf("%d", cfg.Capture.TimeoutSeconds)
		}
	}
	return metadata
}

func ros2ShellCommand(setup []string, body string) []string {
	var builder strings.Builder
	builder.WriteString("set -e; ")
	for _, entry := range setup {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		builder.WriteString("source ")
		builder.WriteString(shellQuote(trimmed))
		builder.WriteString(" >/dev/null 2>&1; ")
	}
	builder.WriteString(body)
	return []string{"bash", "-lc", builder.String()}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func shellSingleQuoteForGrep(value string) string {
	return strings.ReplaceAll(value, `'`, `'\''`)
}
