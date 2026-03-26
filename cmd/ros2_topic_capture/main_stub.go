//go:build !ros2_rclgo

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"eino-vlm-agent-demo/internal/camera"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--probe" {
		fmt.Println("rclgo_unavailable")
		return
	}

	_ = json.NewEncoder(os.Stdout).Encode(camera.CaptureResult{
		OK:    false,
		Error: "ros2_topic_capture was built without ros2_rclgo; rebuild with `go build -tags ros2_rclgo -o ros2_topic_capture ./cmd/ros2_topic_capture` or `./manage_jetson_camera_agent.sh build-ros2-helper` on a ROS2-enabled machine",
	})
}
