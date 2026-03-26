package rclgogen

// Run on a ROS2-enabled machine to regenerate the checked-in message bindings.
//go:generate go run github.com/tiiuae/rclgo/cmd/rclgo-gen generate -d msgs --include-go-package-deps ./...
