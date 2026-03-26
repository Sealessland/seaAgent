# Configs Guide

这个目录的目标很简单：让没有完整软件开发背景的嵌入式同学，也能通过改配置把设备接进 agent。

## 1. 先看哪个文件

- 运行时主配置：
  - [peripherals.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.json)
- 可参考的完整示例：
  - [peripherals.example.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.example.json)
  - [peripherals.ros2.zed.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.ros2.zed.json)
- 可直接复制的设备片段：
  - [ros2_topic_device.example.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/snippets/ros2_topic_device.example.json)

## 2. ROS2 topic 设备怎么注册

最短步骤：

1. 打开 [peripherals.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.json)
2. 把 [ros2_topic_device.example.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/snippets/ros2_topic_device.example.json) 复制到 `devices` 数组里
3. 只填这几个核心字段：
   - `name`
   - `topic`
   - `message_type`
   - `ros_setup`
   - `binary`
4. 如果它是主视觉输入，把 `primary_capture_device` 指到这个设备名
5. 在 ROS2 机器上执行：

```bash
./manage_jetson_camera_agent.sh build-ros2-helper
./manage_jetson_camera_agent.sh restart
```

## 3. 注册完成后怎么验证

推荐按这个顺序：

1. `GET /api/peripherals`
2. `ros2_topic_read` 的 `list`
3. `ros2_topic_read` 的 `inspect`
4. `ros2_topic_read` 的 `capture`

如果 `inspect` 里已经能看到：

- `topic_list`
- `topic_info`
- `capture_helper`

说明配置路径基本是对的，剩下主要看 ROS graph 和 helper 构建环境。

## 4. 什么时候需要改代码

大多数新 ROS2 图像设备，不需要改代码。

优先只改：

1. `configs/peripherals.json`
2. `configs/snippets/...`
3. 必要时 `docs/ros2_topic_interface_example.md`

只有当设备语义已经不是“订阅一个图像 topic 抓一帧”时，才需要进入：

- `internal/peripherals`
- `internal/camera`
- `internal/observation`
