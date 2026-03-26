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

func TestBuildROS2CaptureCommandUsesHelperBinaryByDefault(t *testing.T) {
	command := buildROS2CaptureCommand(ROS2TopicCaptureConfig{
		Topic:       "/front_camera/image_raw",
		MessageType: "sensor_msgs/msg/Image",
	}, "/tmp/out.jpg")

	if !strings.Contains(command, "exec './ros2_topic_capture'") {
		t.Fatalf("command %q does not execute the default helper binary", command)
	}
	if strings.Contains(command, "python3") {
		t.Fatalf("command %q should not use the legacy python helper", command)
	}
}

func TestResolveROS2TopicCapturePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	tests := []struct {
		name string
		cfg  ROS2TopicCaptureConfig
		want string
	}{
		{
			name: "binary path wins",
			cfg: ROS2TopicCaptureConfig{
				BinaryPath: "~/bin/ros2-topic-capture",
				ScriptPath: "./scripts/capture_ros2_topic_image.py",
			},
			want: filepath.Join(home, "bin/ros2-topic-capture"),
		},
		{
			name: "non python script is treated as helper binary",
			cfg: ROS2TopicCaptureConfig{
				ScriptPath: "~/bin/ros2-topic-capture",
			},
			want: filepath.Join(home, "bin/ros2-topic-capture"),
		},
		{
			name: "default helper path",
			cfg:  ROS2TopicCaptureConfig{},
			want: "./ros2_topic_capture",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveROS2TopicCapturePath(tt.cfg); got != tt.want {
				t.Fatalf("ResolveROS2TopicCapturePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsesLegacyROS2TopicScript(t *testing.T) {
	tests := []struct {
		name string
		cfg  ROS2TopicCaptureConfig
		want bool
	}{
		{
			name: "python script is legacy helper",
			cfg: ROS2TopicCaptureConfig{
				ScriptPath: "./scripts/capture_ros2_topic_image.py",
			},
			want: true,
		},
		{
			name: "helper binary is not legacy helper",
			cfg: ROS2TopicCaptureConfig{
				BinaryPath: "./ros2_topic_capture",
			},
			want: false,
		},
		{
			name: "non python script path is treated as helper binary",
			cfg: ROS2TopicCaptureConfig{
				ScriptPath: "./ros2_topic_capture",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := UsesLegacyROS2TopicScript(tt.cfg); got != tt.want {
				t.Fatalf("UsesLegacyROS2TopicScript() = %v, want %v", got, tt.want)
			}
		})
	}
}
