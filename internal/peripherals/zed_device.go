package peripherals

import (
	"context"
	"fmt"
	"strings"

	"eino-vlm-agent-demo/internal/camera"
)

type zedDevice struct {
	cfg DeviceConfig
}

func newZEDDevice(cfg DeviceConfig) (Device, error) {
	device, err := newExecDevice(cfg)
	if err != nil {
		return nil, err
	}
	execDevice := device.(*execDevice)
	if execDevice.cfg.Capture == nil || strings.TrimSpace(execDevice.cfg.Capture.Script) == "" {
		return nil, fmt.Errorf("zed device %q requires capture.script", cfg.Name)
	}
	if len(execDevice.cfg.Checks) == 0 {
		execDevice.cfg.Checks = defaultZEDChecks()
	}
	return &zedDevice{cfg: execDevice.cfg}, nil
}

func (d *zedDevice) Descriptor() DeviceDescriptor {
	return DeviceDescriptor{
		Name:            d.cfg.Name,
		Kind:            d.cfg.Kind,
		Driver:          d.cfg.Driver,
		SupportsCapture: true,
	}
}

func (d *zedDevice) Inspect(ctx context.Context) DeviceSnapshot {
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
		Summary:         summarizeZEDChecks(checkOutputs),
		Checks:          checks,
		Metadata:        d.cfg.Metadata,
	}
}

func (d *zedDevice) Capture(ctx context.Context, outputPath string) (*CaptureResult, error) {
	return camera.CaptureWithPython(ctx, d.cfg.Capture.Script, outputPath)
}

func defaultZEDChecks() []CheckConfig {
	return []CheckConfig{
		{
			Name:    "usb",
			Command: []string{"bash", "-lc", "lsusb | grep -i zed || true"},
		},
		{
			Name:    "ros_topics",
			Command: []string{"bash", "-lc", "source /opt/ros/humble/setup.bash >/dev/null 2>&1; source ~/zed_ws/install/setup.bash >/dev/null 2>&1; ros2 topic list 2>/dev/null | grep '^/zed' || true"},
		},
		{
			Name:    "zed_log_tail",
			Command: []string{"bash", "-lc", "tail -n 40 /tmp/zed_launch.log 2>/dev/null || true"},
		},
		{
			Name:    "pyzed_probe",
			Command: []string{"python3", "-c", "import pyzed.sl as sl; cam=sl.Camera(); init=sl.InitParameters(); init.camera_resolution=sl.RESOLUTION.HD720; init.camera_fps=15; init.depth_mode=sl.DEPTH_MODE.NONE; print(cam.open(init))"},
		},
	}
}

func summarizeZEDChecks(outputs map[string]string) string {
	lowerLog := strings.ToLower(outputs["zed_log_tail"] + " " + outputs["pyzed_probe"])
	switch {
	case strings.TrimSpace(outputs["usb"]) == "" || outputs["usb"] == "(empty)":
		return "ZED device is not visible on USB."
	case strings.Contains(lowerLog, "camera stream failed to start"):
		return "ZED is visible on USB, but the SDK stream fails to start before any frame reaches the agent."
	case strings.TrimSpace(outputs["ros_topics"]) != "" && outputs["ros_topics"] != "(empty)":
		return "ZED ROS2 topics are visible. Frame transport may be available."
	default:
		return "ZED is visible, but no active ROS2 topics or successful SDK grab were detected."
	}
}
