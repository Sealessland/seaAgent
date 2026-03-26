# ROS2 Topic 外设接口示例

这份文档描述当前项目里 `ros2_topic` 外设接口的现状。

目标：

- 不改 ZED 现有 `pyzed` 抓图链路
- 给 ROS2 图像 topic 一条独立、可配置、可被 agent 调用的接入方式
- 让后续接入其他 ROS2 相机时，不需要再改 handler / service / agent 主链路

## 1. 设计结论

当前方案是：

- Go 侧统一抽象成 `driver=ros2_topic`
- 真正抓图默认使用一个最小 `rclgo` helper 二进制
- helper 通过 ROS 2 的 `rcl` C API 订阅一次 topic，拿到一帧后落盘
- 旧的 Python subscriber 只作为兼容回退保留

这里没有选 `ros2 topic echo` 当主抓图方案，原因是：

- `ros2 topic echo` 更适合调试，不适合稳定抓图
- 图像消息需要类型化解析
- `sensor_msgs/msg/Image` 和 `sensor_msgs/msg/CompressedImage` 都要走不同解码路径
- 用 `rclgo` 的订阅器更贴近当前服务的 Go 主链路，也减少了生产链路对 Python 依赖的耦合

## 2. 新增配置形式

`configs/peripherals.example.json` 里已经补了一个示例设备：

- [peripherals.example.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.example.json)

示例字段：

```json
{
  "name": "front-camera-ros2",
  "kind": "rgb_camera",
  "driver": "ros2_topic",
  "capture": {
    "binary": "./ros2_topic_capture",
    "topic": "/front_camera/image_raw/compressed",
    "message_type": "sensor_msgs/msg/CompressedImage",
    "encoding": "bgr8",
    "timeout_seconds": 5,
    "ros_setup": [
      "/opt/ros/humble/setup.bash",
      "~/robot_ws/install/setup.bash"
    ]
  }
}
```

其中关键字段是：

- `driver`
  - 固定为 `ros2_topic`
- `capture.binary`
  - 默认 helper 路径是 `./ros2_topic_capture`
  - 推荐显式写出来，方便部署时排查
- `capture.topic`
  - 要订阅的 ROS2 图像 topic
- `capture.message_type`
  - 当前支持：
    - `sensor_msgs/msg/Image`
    - `sensor_msgs/msg/CompressedImage`
- `capture.encoding`
  - 对 `Image` 类型会作为原始像素编码提示
- `capture.timeout_seconds`
  - 等待第一帧的超时
- `capture.ros_setup`
  - 启动 helper 前要 source 的 ROS 环境

兼容模式下，仍然可以填：

- `capture.script`
  - 指向 `*.py` 时，会走旧 `rclpy` subscriber
  - 指向非 `.py` 路径时，会被当成 helper 二进制路径

## 3. Go 侧接口位置

### 3.1 driver

新增 driver：

- [ros2_topic_device.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/ros2_topic_device.go)

这个 driver 负责：

- 校验 `topic/message_type`
- 注入默认检查项
- 调用 `rclgo` helper 抓图
- 把 topic 和 message type 写回 metadata

### 3.2 manager 层

为了让 agent tool 能按设备名操作，manager 侧暴露了：

- `InspectDevice(ctx, name)`
- `CaptureDevice(ctx, name, outputPath)`

对应文件：

- [manager.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/manager.go)

这样以后不止主设备可以抓图，任意具备 capture 能力的 ROS2 设备都能被统一调用。

### 3.3 camera helper

ROS2 topic 抓图 helper 由这几部分组成：

- [ros2_topic.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/camera/ros2_topic.go)
- [main.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/ros2_topic_capture/main.go)
- [main_stub.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/ros2_topic_capture/main_stub.go)

这里做的事是：

- 拼接 `bash -lc`
- 按配置 source ROS 环境
- 调 `ros2_topic_capture ...`
- 解析最后一段 JSON 结果

### 3.4 rclgo message bindings

当前 helper 只需要一小部分标准消息绑定：

- [Time.gen.go](/home/sealessland/inference/eino-vlm-agent-demo/msgs/builtin_interfaces/msg/Time.gen.go)
- [Header.gen.go](/home/sealessland/inference/eino-vlm-agent-demo/msgs/std_msgs/msg/Header.gen.go)
- [Image.gen.go](/home/sealessland/inference/eino-vlm-agent-demo/msgs/sensor_msgs/msg/Image.gen.go)
- [CompressedImage.gen.go](/home/sealessland/inference/eino-vlm-agent-demo/msgs/sensor_msgs/msg/CompressedImage.gen.go)

这些文件只在 `-tags ros2_rclgo` 时参与编译。

如果后面要在 ROS2 机器上重新生成绑定，项目根目录已经预留了：

- [rclgo_generate.go](/home/sealessland/inference/eino-vlm-agent-demo/rclgo_generate.go)
- [README.md](/home/sealessland/inference/eino-vlm-agent-demo/msgs/README.md)

## 4. rclgo helper 实现

helper 的策略是：

1. 启动一个最小 `rclgo` node
2. 订阅指定 topic
3. 等到第一帧
4. 如果是 `sensor_msgs/msg/Image`
   - 直接按编码把 payload 转成 Go `image.Image`
5. 如果是 `sensor_msgs/msg/CompressedImage`
   - 用 Go 标准库 `image.Decode(...)`
6. 落盘
7. 输出 JSON 给 Go

默认构建没有 ROS 依赖时，`main_stub.go` 会返回：

- `rclgo_unavailable`

这样当前开发机仍然可以跑 `go test ./...`，而 Jetson/ROS2 环境再单独构建 helper：

```bash
go build -tags ros2_rclgo -o ros2_topic_capture ./cmd/ros2_topic_capture
```

兼容回退脚本仍在这里：

- [capture_ros2_topic_image.py](/home/sealessland/inference/eino-vlm-agent-demo/scripts/capture_ros2_topic_image.py)

## 5. Agent tool call 接口

新增了一个给 agent 用的稳定 tool：

- `ros2_topic_read`

定义位置：

- [chat_tools.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/jetson_camera_agent/chat_tools.go)

支持三种模式：

- `mode=list`
  - 列出所有配置好的 `ros2_topic` 设备
- `mode=inspect`
  - 查看单个 ROS2 设备的状态摘要
- `mode=capture`
  - 从指定 ROS2 图像设备抓一帧

这意味着后续模型不需要知道具体 topic 名、helper 路径、ROS setup 细节。

它只需要调用：

```json
{"mode":"capture","device":"front-camera-ros2"}
```

## 6. 可用性前提

这条路径在架构上是可用的，但运行时前提必须满足：

- 已安装 ROS2 C 运行时和标准消息头文件
- helper 已通过 `go build -tags ros2_rclgo ...` 构建
- 对应 ROS2 topic 正在发布

当前这套接口已经把依赖检查放进了设备检查项里：

- `topic_list`
- `topic_info`
- `capture_helper`

只有在显式配置 `capture.script=*.py` 的兼容模式下，才会回到：

- `python_deps`

所以不是“接口写了但跑不起来看不出来”，而是会在外设状态和 tool 输出里暴露可用性。

## 7. 为什么这套接口方便扩展

因为它不是按某一台相机写死的，而是按“ROS2 图像 topic 设备”建模：

- 配置决定 topic
- 配置决定 message type
- 配置决定 ROS 环境
- Go 主链路不需要知道 topic 细节
- agent 只面对稳定 tool 名

所以后面接：

- USB 摄像头经 image_transport 发布的 topic
- 网络摄像头桥接出的 topic
- 工业相机节点
- 深度相机的 RGB topic

都可以复用这一套接口。
