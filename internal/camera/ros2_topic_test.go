package camera

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	got := ExpandHomePath("~/zed_ws/install/setup.bash")
	want := filepath.Join(home, "zed_ws/install/setup.bash")
	if got != want {
		t.Fatalf("ExpandHomePath() = %q, want %q", got, want)
	}
}

func TestBuildROS2CaptureCommandExpandsSetupPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	command := buildROS2CaptureCommand(ROS2TopicCaptureConfig{
		ScriptPath:     "./scripts/capture_ros2_topic_image.py",
		Topic:          "/zed/zed_node/rgb/color/rect/image/compressed",
		MessageType:    "sensor_msgs/msg/CompressedImage",
		Encoding:       "bgr8",
		TimeoutSeconds: 5,
		ROSSetup:       []string{"/opt/ros/humble/setup.bash", "~/zed_ws/install/setup.bash"},
	}, "/tmp/out.jpg")

	want := "source '" + filepath.Join(home, "zed_ws/install/setup.bash") + "' >/dev/null 2>&1;"
	if !strings.Contains(command, want) {
		t.Fatalf("command %q does not contain expanded setup path %q", command, want)
	}
}
