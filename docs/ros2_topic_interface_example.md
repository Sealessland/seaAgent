# ROS2 Topic 外设接口示例

这份文档描述当前项目里新增的 `ros2_topic` 外设接口形式。

目标：

- 不改 ZED 现有 `pyzed` 抓图链路
- 给 ROS2 图像 topic 一条独立、可配置、可被 agent 调用的接入方式
- 让后续接入其他 ROS2 相机时，不需要再改 handler / service / agent 主链路

## 1. 设计结论

调研后，这里采用的是：

- Go 侧统一抽象成 `driver=ros2_topic`
- 真正抓图使用一个最小 Python subscriber
- subscriber 通过 `rclpy` 订阅一次 topic，拿到一帧后落盘

这里没有选 `ros2 topic echo` 当主抓图方案，原因是：

- `ros2 topic echo` 更适合调试，不适合稳定抓图
- 图像消息需要类型化解析
- `sensor_msgs/msg/Image` 和 `CompressedImage` 都要走不同解码路径
- 用 `rclpy + sensor_msgs + cv_bridge` 的 subscriber 更稳定，也更接近真实工程路径

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
    "script": "./scripts/capture_ros2_topic_image.py",
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
- `capture.topic`
  - 要订阅的 ROS2 图像 topic
- `capture.message_type`
  - 当前支持：
    - `sensor_msgs/msg/Image`
    - `sensor_msgs/msg/CompressedImage`
- `capture.encoding`
  - 对 `Image` 类型会传给 `cv_bridge`
- `capture.timeout_seconds`
  - 等待第一帧的超时
- `capture.ros_setup`
  - 启动 subscriber 前要 source 的 ROS 环境

## 3. Go 侧接口位置

### 3.1 driver

新增 driver：

- [ros2_topic_device.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/ros2_topic_device.go)

这个 driver 负责：

- 校验 `topic/message_type`
- 注入默认检查项
- 调用 Python subscriber 抓图
- 把 topic 和 message type 写回 metadata

### 3.2 manager 层

为了让 agent tool 能按设备名操作，我补了：

- `InspectDevice(ctx, name)`
- `CaptureDevice(ctx, name, outputPath)`

对应文件：

- [manager.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/manager.go)

这样以后不止主设备可以抓图，任意具备 capture 能力的 ROS2 设备都能被统一调用。

### 3.3 camera helper

新增 ROS2 topic 抓图 helper：

- [ros2_topic.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/camera/ros2_topic.go)

这里做的事是：

- 拼接 `bash -lc`
- 按配置 source ROS 环境
- 调 `python3 capture_ros2_topic_image.py ...`
- 解析最后一段 JSON 结果

## 4. Python subscriber 实现

新增脚本：

- [capture_ros2_topic_image.py](/home/sealessland/inference/eino-vlm-agent-demo/scripts/capture_ros2_topic_image.py)

它的策略是：

1. 启动一个最小 `rclpy` node
2. 订阅指定 topic
3. 等到第一帧
4. 如果是 `sensor_msgs/msg/Image`
   - 用 `cv_bridge.CvBridge.imgmsg_to_cv2(...)`
5. 如果是 `sensor_msgs/msg/CompressedImage`
   - 用 `numpy + cv2.imdecode(...)`
6. 落盘
7. 输出 JSON 给 Go

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

这意味着后续模型不需要知道具体 topic 名、脚本路径、ROS setup 细节。

它只需要调用：

```json
{"mode":"capture","device":"front-camera-ros2"}
```

## 6. 可用性前提

这条路径在架构上是可用的，但运行时前提必须满足：

- `rclpy`
- `sensor_msgs`
- `cv_bridge`
- `numpy`
- `opencv-python` 或系统 OpenCV Python 绑定
- 对应 ROS2 topic 正在发布

当前这套接口已经把依赖检查放进了设备检查项里：

- `python_deps`
- `topic_list`
- `topic_info`

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
