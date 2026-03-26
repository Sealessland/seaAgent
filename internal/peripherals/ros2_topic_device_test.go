package peripherals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eino-vlm-agent-demo/internal/camera"
)

func TestROS2ShellCommandExpandsSetupPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	command := ros2ShellCommand([]string{"~/zed_ws/install/setup.bash"}, "ros2 topic list")
	if len(command) != 3 {
		t.Fatalf("unexpected command length: %d", len(command))
	}

	want := "source '" + filepath.Join(home, "zed_ws/install/setup.bash") + "' >/dev/null 2>&1;"
	if !strings.Contains(command[2], want) {
		t.Fatalf("command %q does not contain expanded setup path %q", command[2], want)
	}
}

func TestDefaultROS2TopicChecksUseHelperProbe(t *testing.T) {
	checks := defaultROS2TopicChecks(&CaptureConfig{
		Topic:       "/front_camera/image_raw",
		MessageType: "sensor_msgs/msg/Image",
		ROSSetup:    []string{"/opt/ros/humble/setup.bash"},
	})

	if len(checks) != 3 {
		t.Fatalf("unexpected check count: %d", len(checks))
	}

	var hasHelper bool
	for _, check := range checks {
		if check.Name == "python_deps" {
			t.Fatalf("unexpected legacy python dependency check: %+v", check)
		}
		if check.Name == "capture_helper" {
			hasHelper = true
			if len(check.Command) != 3 {
				t.Fatalf("unexpected helper command length: %d", len(check.Command))
			}
			if !strings.Contains(check.Command[2], "'./ros2_topic_capture' --probe") {
				t.Fatalf("helper probe command %q does not target the rclgo helper", check.Command[2])
			}
		}
	}

	if !hasHelper {
		t.Fatalf("capture_helper check was not added")
	}
}

func TestDefaultROS2TopicChecksUseLegacyPythonDeps(t *testing.T) {
	checks := defaultROS2TopicChecks(&CaptureConfig{
		Script:      "./scripts/capture_ros2_topic_image.py",
		Topic:       "/front_camera/image_raw/compressed",
		MessageType: "sensor_msgs/msg/CompressedImage",
	})

	var hasPythonDeps bool
	for _, check := range checks {
		if check.Name == "capture_helper" {
			t.Fatalf("unexpected helper probe in legacy python mode: %+v", check)
		}
		if check.Name == "python_deps" {
			hasPythonDeps = true
		}
	}

	if !hasPythonDeps {
		t.Fatalf("python_deps check was not added for the legacy subscriber path")
	}
}

func TestSummarizeROS2TopicChecksHelperMode(t *testing.T) {
	summary := summarizeROS2TopicChecks(&CaptureConfig{
		Topic:       "/front_camera/image_raw",
		MessageType: "sensor_msgs/msg/Image",
	}, map[string]string{
		"capture_helper": "rclgo_ready",
		"topic_list":     "/front_camera/image_raw",
		"topic_info":     "Type: sensor_msgs/msg/Image",
	})

	if !strings.Contains(summary, "rclgo subscriber") {
		t.Fatalf("summary %q does not mention the rclgo subscriber path", summary)
	}
}

func TestNewROS2TopicDeviceDefaultsHelperBinary(t *testing.T) {
	device, err := newROS2TopicDevice(DeviceConfig{
		Name:   "front-camera-ros2",
		Kind:   "rgb_camera",
		Driver: "ros2_topic",
		Capture: &CaptureConfig{
			Topic:       "/front_camera/image_raw",
			MessageType: "sensor_msgs/msg/Image",
		},
	})
	if err != nil {
		t.Fatalf("newROS2TopicDevice() error = %v", err)
	}

	rosDevice := device.(*ros2TopicDevice)
	if got := rosDevice.cfg.Capture.Binary; got != camera.DefaultROS2CaptureBinaryPath() {
		t.Fatalf("default helper binary = %q, want %q", got, camera.DefaultROS2CaptureBinaryPath())
	}

	metadata := ros2TopicMetadata(rosDevice.cfg)
	if got := metadata["capture_helper"]; got != camera.DefaultROS2CaptureBinaryPath() {
		t.Fatalf("metadata capture_helper = %q, want %q", got, camera.DefaultROS2CaptureBinaryPath())
	}
}
