# Jetson ROS2 图片链路验证记录（2026-03-25）

验证时间：2026-03-25 21:14（Asia/Shanghai）

## 结论

ROS2 图片链路当前可用，已经在 Jetson 现场验证通过。

当前稳定可用的图像话题：

- Topic: `/zed/zed_node/rgb/color/rect/image`
- Message Type: `sensor_msgs/msg/Image`

当前 ROS2 验证服务实例：

- Agent UI: `http://127.0.0.1:18084/`
- Debug UI: `http://127.0.0.1:18085/`

说明：

- 现有主服务 `18080` 未改动
- ROS2 链路在独立实例上验证，避免影响现有直连相机流程

## 1. ROS2 话题可见性

执行：

```bash
source /opt/ros/humble/setup.bash
source ~/zed_ws/install/setup.bash
ros2 topic info /zed/zed_node/rgb/color/rect/image
```

结果：

```text
Type: sensor_msgs/msg/Image
Publisher count: 1
Subscription count: 0
```

结论：

- ROS graph 中该图像话题可见
- Jetson 上已有 publisher 正在提供该图像 topic

## 2. 直接 ROS2 subscriber 抓图验证

执行：

```bash
python3 ./scripts/capture_ros2_topic_image.py \
  --output /tmp/ros2-verify-20260325-211443.jpg \
  --topic /zed/zed_node/rgb/color/rect/image \
  --message-type sensor_msgs/msg/Image \
  --encoding bgr8 \
  --timeout-seconds 10
```

结果：

```json
{"ok": true, "output": "/tmp/ros2-verify-20260325-211443.jpg", "width": 640, "height": 360, "camera_sn": "/zed/zed_node/rgb/color/rect/image"}
```

结论：

- 项目内的一次性 ROS2 subscriber 可以成功收到图像帧
- 图片已成功落盘
- 当前图像分辨率为 `640x360`

## 3. 服务侧 `ros2_topic` 抓图验证

执行：

```bash
curl -sS http://127.0.0.1:18084/api/camera/capture
```

结果：

```json
{"ok":true,"output":"/tmp/jetson-camera-agent/capture-20260325-211444.416771452.jpg","width":640,"height":360,"camera_sn":"/zed/zed_node/rgb/color/rect/image"}
```

结论：

- 服务内的 `driver=ros2_topic` 链路可正常抓图
- 从 HTTP 接口到 ROS2 subscriber 到本地落盘的链路已打通

## 4. 当前配置说明

本次验证使用的独立配置文件：

- `configs/peripherals.ros2.zed.json`

关键配置：

- `primary_capture_device=front-camera-ros2-zed`
- topic 使用 `/zed/zed_node/rgb/color/rect/image`
- message type 使用 `sensor_msgs/msg/Image`

## 5. 补充说明

本轮验证中，raw image 话题稳定可用；因此当前推荐优先使用：

- `/zed/zed_node/rgb/color/rect/image`
- `sensor_msgs/msg/Image`

如后续要把 ROS2 链路提升为默认主链路，可以直接把默认外设配置切到这份 ROS2 配置。
