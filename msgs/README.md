# rclgo Message Bindings

`cmd/ros2_topic_capture` currently needs only a minimal subset of ROS 2 standard message bindings:

- `builtin_interfaces/msg/Time`
- `std_msgs/msg/Header`
- `sensor_msgs/msg/Image`
- `sensor_msgs/msg/CompressedImage`

These bindings are compiled only with `-tags ros2_rclgo`.

The checked-in `*.gen.go` files are a bootstrap snapshot so the helper can move to `rclgo` without forcing the default developer environment to install ROS 2 first. On a ROS2-enabled machine, regenerate them from the repository root with:

```bash
go generate ./...
```

That uses the command declared in:

- [rclgo_generate.go](/home/sealessland/inference/eino-vlm-agent-demo/rclgo_generate.go)
