# Jetson ROS2 与正常对话链路端到端验证（2026-03-25）

验证时间：2026-03-25 21:20 至 21:22（Asia/Shanghai）

## 结论

两条链路都已在 Jetson 现场打通：

- ROS2 视频 / 抓帧链路：可用
- 正常对话链路：可用

本次验证使用的 ROS2 独立服务实例：

- Agent UI: `http://127.0.0.1:18084/`
- Debug UI: `http://127.0.0.1:18085/`

说明：

- 现有主服务 `18080` 未改动
- 本次验证全部在 ROS2 独立实例上完成，避免影响当前默认直连链路

## 0. 先给初学者的整体理解

如果你第一次看这个项目，可以先把它理解成一条“从网页提问，到 ROS2 抓图，再到模型回答”的流水线。

### 0.1 这套系统到底在做什么

用户在浏览器里发一句话，例如：

- “看看前方有什么”
- “比较上一张图和当前画面”
- “调用 rear-camera-ros2 抓一张图并描述”

系统不会直接让模型去猜底层 ROS2 topic，而是按下面这条链路工作：

1. 前端把请求发到后端 HTTP 接口。
2. 后端先判断这句话是不是需要抓图、是否需要比较上一张图、是否要带上外设快照。
3. 后端把这些上下文整理好，再交给 Eino agent。
4. Eino agent 决定是否调用 `camera_read` 或 `ros2_topic_read` 这类工具。
5. 工具不会直接和模型耦合，而是去调用外设管理层。
6. 外设管理层再去调用 `ros2_topic` driver。
7. `ros2_topic` driver 最后通过一个最小 Python subscriber 从 ROS2 topic 订阅一帧并落盘。
8. 图片路径回到 agent 后，模型继续读图，生成自然语言回答。
9. 如果是多轮会话，系统还会把最近几张图片记到 session 中，供后续比较使用。

### 0.2 关键代码入口在哪

初学者建议按下面顺序看代码：

1. `internal/jetsonagent/server.go`
2. `internal/jetsonagent/handlers.go`
3. `internal/observation/service.go`
4. `internal/observation/chat_tools.go`
5. `internal/agent/vision_agent.go`
6. `internal/observation/session_store.go`
7. `internal/peripherals/ros2_topic_device.go`
8. `internal/camera/ros2_topic.go`
9. `scripts/capture_ros2_topic_image.py`
10. `prompts/system.txt`

看法建议：

- `server.go` 看“系统有哪些 HTTP 路由”
- `handlers.go` 看“前端请求怎么被转进 service”
- `service.go` 看“系统什么时候抓图、什么时候比较、什么时候兜底”
- `chat_tools.go` 看“模型到底能调哪些工具”
- `vision_agent.go` 看“Eino agent 和模型如何协作”
- `session_store.go` 看“为什么它能记住上一张图”
- `ros2_topic_device.go` 和 `ros2_topic.go` 看“ROS2 图到底怎么进系统”
- `capture_ros2_topic_image.py` 看“实际 subscriber 怎么写”
- `system.txt` 看“prompt 是怎么约束 agent 行为的”

### 0.3 一条完整交互链路

以“请直接观察当前画面，告诉我前方有什么”为例，真实调用链如下：

1. 浏览器请求 `/api/agent/chat` 或 `/api/agent/chat/stream`
2. 路由定义在 `internal/jetsonagent/server.go`
3. 请求处理在 `internal/jetsonagent/handlers.go`
4. 处理函数继续调用 `ObservationService.AgentChat(...)`
5. `internal/observation/service.go` 会先做动作启发：
   - 是否需要 fresh capture
   - 是否要复用 latest image
   - 是否要做 image comparison
   - 是否附带 peripherals snapshot
6. 然后它调用 `VisionAgent.Chat(...)`
7. `internal/agent/vision_agent.go` 内部用 Eino ADK 的 `ChatModelAgent`
8. agent 可调用在 `internal/observation/chat_tools.go` 注册的工具
9. 若调用 `camera_read` 或 `ros2_topic_read(mode=capture)`，会进入 peripherals manager
10. peripherals manager 再把请求转发到 `ros2_topic` device
11. `internal/peripherals/ros2_topic_device.go` 组织 capture 参数
12. `internal/camera/ros2_topic.go` 组装 bash 命令与 ROS setup
13. `scripts/capture_ros2_topic_image.py` 启动 `rclpy` 节点，订阅一次 topic，拿到一帧，保存图片
14. 图片路径回到 Go 服务
15. `internal/agent/vision_agent.go` 再把图片作为多模态输入发给模型，让它真正“读图后回答”
16. `internal/observation/session_store.go` 会记住这张图，以便下一轮比较“上一张 vs 当前张”

### 0.4 prompt 和服务端为什么都要做

只靠 prompt，不够稳。

只靠服务端硬编码，也不够灵活。

当前项目采用的是两层配合：

- `prompts/system.txt` 负责约束模型行为
- `internal/observation/service.go` 负责工程兜底

例如系统 prompt 明确要求：

- 当前画面请求要尽快调用相机能力
- 工具返回图片后，要继续读图回答
- 不要只返回路径或 JSON

而 `service.go` 还额外负责：

- 识别“观察当前画面”这类请求并优先抓新帧
- 识别“比较上一张图和当前图”
- 如果模型只是说“需要调用工具”，自动补抓图并重答

这就是为什么它比单纯让模型自由发挥更稳定。

## 1. ROS2 视频 / 抓帧链路

当前稳定可用的 ROS2 图像话题：

- Topic: `/zed/zed_node/rgb/color/rect/image`
- Message Type: `sensor_msgs/msg/Image`

### 1.1 持续发布验证

执行：

```bash
source /opt/ros/humble/setup.bash
source ~/zed_ws/install/setup.bash
timeout 8 ros2 topic hz /zed/zed_node/rgb/color/rect/image
```

结果摘要：

```text
average rate: 15.005 Hz
```

结论：

- ROS2 图像 topic 正在持续出帧
- 当前实测帧率约为 `15 Hz`

### 1.2 直接 ROS2 subscriber 抓图验证

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

- 项目内 ROS2 subscriber 已能稳定收到并落盘图像
- 当前抓图分辨率为 `640x360`

### 1.3 服务侧连续抓图验证

执行：

```bash
for i in 1 2 3; do
  curl -sS http://127.0.0.1:18084/api/camera/capture
  sleep 1
done
```

结果摘要：

- `capture-20260325-212035.245275787.jpg`
- `capture-20260325-212038.374595817.jpg`
- `capture-20260325-212041.461680938.jpg`

三次均返回：

- `ok=true`
- `width=640`
- `height=360`
- `camera_sn=/zed/zed_node/rgb/color/rect/image`

结论：

- 服务端 `driver=ros2_topic` 连续抓图可用
- ROS2 图像链路不是一次性成功，而是可持续工作

### 1.4 最新画面预览验证

执行：

```bash
curl -sS http://127.0.0.1:18084/api/camera/capture
curl -sS http://127.0.0.1:18084/api/camera/latest.jpg -o /tmp/ros2-latest-preview-check.jpg
sha256sum <最新 capture 文件> /tmp/ros2-latest-preview-check.jpg
```

结果：

```text
d4f7c9c5a70c10cf125051afd993f1f73611607bde57c70ae4a5013872376c2f  /tmp/jetson-camera-agent/capture-20260325-212044.513707852.jpg
d4f7c9c5a70c10cf125051afd993f1f73611607bde57c70ae4a5013872376c2f  /tmp/ros2-latest-preview-check.jpg
```

结论：

- `/api/camera/latest.jpg` 正确指向最新一帧
- 前端轮询预览使用的“最新图片”链路可用

## 2. 正常对话链路

### 2.1 主页面可访问

执行：

```bash
curl -I -sS http://127.0.0.1:18084/
```

结果：

```text
HTTP/1.1 200 OK
```

结论：

- 正常对话页面可打开

### 2.2 普通对话接口可用

执行：

```bash
curl -sS -X POST http://127.0.0.1:18084/api/agent/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"请直接观察当前画面，简短告诉我前方有什么。"}'
```

结果摘要：

- 返回 `session_id`
- 返回自然语言场景描述
- 返回 `capture.ok=true`
- `trace.tool_calls` 中可见实际抓图行为

本次返回内容明确描述了：

- 左侧打开的黑色泡沫箱
- 右侧履带机器人
- 地面标线
- 背景蓝白墙、显示器与安全帽

结论：

- 正常对话接口可用
- 对话请求会真实调用抓图，而不是只回复“需要调用工具”
- 当前对话链路已通过 ROS2 图像源完成观察与回答

### 2.3 流式对话接口可用

执行：

```bash
curl -sS -N -X POST http://127.0.0.1:18084/api/agent/chat/stream \
  -H 'Content-Type: application/json' \
  -d '{"message":"请直接观察当前画面，用一句话回答，不要先解释你要调用工具。"}'
```

结果摘要：

流式事件顺序正常：

- `event: status`
- `event: meta`
- `event: delta`
- `event: done`

最终返回为一句直接场景描述，没有先输出“我需要调用工具”。

结论：

- 正常对话页面使用的 SSE 流式接口可用
- 前端实时显示回答的链路可用

## 3. 最终判断

截至 2026-03-25 21:22（Asia/Shanghai），Jetson 上这两条链路均已验证通过：

- ROS2 视频 / 抓帧 / 最新画面预览链路：通过
- 正常对话 / 流式对话 / 图像理解回答链路：通过

如果后续要把 ROS2 作为默认主链路，可以直接将默认服务切换到这套 ROS2 外设配置。

## 4. 其他设备如何从 ROS2 链路接入

这一套方案的设计目标不是只服务当前 ZED，而是把“ROS2 图像 topic 设备”抽象成统一外设。后续接其他设备时，优先复用现有 `driver=ros2_topic` 与 `ros2_topic_read`，不要为每个相机单独改 handler 或 agent 主链路。

### 4.1 最小接入步骤

对于其他 ROS2 图像设备，推荐按下面流程接入：

1. 先确认设备已经在 ROS graph 中持续发布图像 topic。
2. 明确 topic 名称、消息类型、ROS 环境路径。
3. 在外设配置里新增一个 `driver=ros2_topic` 设备条目。
4. 先用 `scripts/capture_ros2_topic_image.py` 单独抓图验证。
5. 再通过 `ros2_topic_read` tool 做 `list / inspect / capture` 验证。
6. 最后再把它挂进正常对话链路或提升为 primary device。

### 4.2 推荐配置方式

新增设备时，直接在 `configs/peripherals*.json` 中追加设备，不改主链路代码。典型配置如下：

```json
{
  "name": "rear-camera-ros2",
  "kind": "rgb_camera",
  "driver": "ros2_topic",
  "metadata": {
    "mount": "rear",
    "transport": "ros2",
    "source": "image_pipeline"
  },
  "capture": {
    "script": "./scripts/capture_ros2_topic_image.py",
    "topic": "/rear_camera/image_raw",
    "message_type": "sensor_msgs/msg/Image",
    "encoding": "bgr8",
    "timeout_seconds": 5,
    "ros_setup": [
      "/opt/ros/humble/setup.bash",
      "~/robot_ws/install/setup.bash"
    ]
  }
}
```

如果设备发布的是压缩图像，可改为：

- `message_type="sensor_msgs/msg/CompressedImage"`
- `topic=/xxx/compressed`

### 4.3 什么时候可以直接复用 `ros2_topic_read`

以下场景，直接复用现有 `ros2_topic_read` 就够了：

- 列出有哪些 ROS2 图像设备
- 检查某个设备当前是否在线
- 从某个设备抓一张当前图片
- 让 agent 在多设备之间按设备名切换观察

当前工具接口已经够稳定：

- `mode=list`
- `mode=inspect`
- `mode=capture`

示例：

```json
{"mode":"capture","device":"rear-camera-ros2"}
```

### 4.4 什么时候不应该继续复用 `ros2_topic_read`

如果设备不再是“读一张图”这个语义，就不建议继续往 `ros2_topic_read` 里硬塞。

更合理的做法是按能力拆新 tool：

- 机械臂状态读取：`arm_state_read`
- 导航状态读取：`nav_status_read`
- 激光雷达摘要：`lidar_scan_read`
- 底盘控制：`base_motion_command`
- 云台控制：`gimbal_control`

原则是：

- 一个 tool 对应一种稳定能力
- 输入 schema 要窄，避免让模型直接操作底层 topic 细节
- 输出结构要稳定，方便 agent 后续推理和服务端兜底

### 4.5 Tool Call 的推荐封装方式

比较正经的封装方式是：

1. 配置层负责 topic、message type、ROS setup。
2. driver 层负责实际 ROS2 读取与基础校验。
3. manager 层负责按设备名统一调用。
4. tool 层只暴露“list / inspect / capture”这类稳定业务能力。
5. agent/prompt 层只面对工具语义，不直接接触 topic 名。

这样做的好处是：

- 更换 topic 不需要改 prompt
- 更换工作区路径不需要改模型指令
- 模型不会生成脆弱的底层 ROS 命令
- 更符合 Eino agent 的工具边界设计

这里和代码的对应关系是：

- 配置层：
  `configs/peripherals*.json`
- driver 层：
  `internal/peripherals/ros2_topic_device.go`
- ROS2 capture helper：
  `internal/camera/ros2_topic.go`
- 真正 subscriber：
  `scripts/capture_ros2_topic_image.py`
- tool 定义：
  `internal/observation/chat_tools.go`
- agent 调用工具与读图：
  `internal/agent/vision_agent.go`
- 服务编排与兜底：
  `internal/observation/service.go`

### 4.6 对 Eino agent 的推荐做法

从 Eino agent 的工程实践看，建议遵循这几个约束：

- 让模型调用的是高层工具名，不是底层 ROS2 topic
- 保持 tool schema 简短、枚举清晰、字段少而稳定
- 把设备发现能力做成 `list/inspect`，不要让模型盲猜设备名
- 对“观察当前画面”这种高频需求，继续走 `camera_read` 这种主能力工具
- 对“指定设备观察”这种需求，再走 `ros2_topic_read`
- 对高风险写操作单独建 tool，并保留更强校验或人工确认

也就是说：

- `camera_read` 解决“主视角当前画面”
- `ros2_topic_read` 解决“多 ROS2 图像设备按名访问”
- 其他能力单独做专用 tool，而不是让一个大而杂的万能 tool 承担全部职责

如果是初学者，最重要的一条可以记成一句话：

- 模型只负责“决定用哪个能力”和“拿到结果后怎么回答”
- 系统代码负责“真正去和 ROS2、文件、外设打交道”

不要让模型自己生成 `ros2 topic echo`、`python3 xxx.py`、`source ~/xxx/setup.bash` 这类底层命令；这些都应该由服务端和 tool 层封装掉。

### 4.7 一个推荐接入流程

后面如果再接一个新设备，建议固定按这条流程做：

1. `ros2 topic list` / `ros2 topic info` 确认 topic 存在。
2. 用项目自带 subscriber 脚本直接抓图。
3. 把设备加进 `configs/peripherals*.json`。
4. 用 `/api/peripherals` 或 `ros2_topic_read(mode=list/inspect)` 验证配置生效。
5. 用 `ros2_topic_read(mode=capture)` 验证单设备抓图。
6. 再决定它是否应该成为 `primary_capture_device`。
7. 若只是辅助设备，保留为命名设备，不强行改主链路。

如果只是“再接一个 ROS2 相机”，通常到第 5 步就够了，很多时候不需要改 Go 代码。

只有下面这些情况才建议改代码：

- 设备不是图像 topic，而是别的消息类型
- 设备需要控制命令而不是只读
- 设备输出不是“一张图”这种语义，而是状态、轨迹、点云、目标列表等
- 你希望给模型暴露一个更高层、更业务化的工具，而不是原始设备名

### 4.8 如果要新建一个专用 tool，建议怎么做

如果以后想把某个设备能力封装成更好懂的 tool，建议按当前项目的模式来：

1. 先定义清晰的输入输出 struct。
2. 在 `internal/observation/chat_tools.go` 里用 `utils.InferTool(...)` 注册。
3. tool 内部只做高层动作，不暴露 ROS2 底层细节。
4. 真正的底层读取或控制，继续下沉到 peripherals / camera / driver 层。
5. 在 `prompts/system.txt` 中补充什么时候应该使用这个 tool。
6. 如果这是高风险操作，不要和只读工具混在一起。

一个好的 tool，应该满足：

- 名字能看懂
- 参数少
- 参数枚举尽量稳定
- 输出 JSON 结构稳定
- 出错时返回明确错误
- 模型不需要知道 ROS2 topic 名称

反过来说，一个坏的 tool 往往长这样：

- 让模型直接传 topic 名
- 让模型自己传 bash 命令
- 输入字段太自由
- 一个 tool 既读状态又发控制
- 输出结构每次都不一样

### 4.9 当前这套方案为什么适合初学者继续扩展

因为它把复杂性拆开了：

- 前端只负责发请求和显示结果
- handler 只负责 HTTP 进出
- service 只负责业务编排
- tool 只负责给 agent 暴露稳定能力
- driver 只负责设备访问
- subscriber 只负责从 ROS2 真正取一帧

你后续扩展时，只需要先判断自己在改哪一层，而不是一上来就在 handler、prompt、模型、ROS2 命令之间乱跳。

### 4.10 当前项目里已经具备的复用点

当前项目已经有这些现成能力，可以直接复用：

- ROS2 图像 subscriber 脚本：
  [capture_ros2_topic_image.py](/home/sealessland/inference/eino-vlm-agent-demo/scripts/capture_ros2_topic_image.py)
- ROS2 capture helper：
  [ros2_topic.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/camera/ros2_topic.go)
- ROS2 外设 driver：
  [ros2_topic_device.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/ros2_topic_device.go)
- Agent tool 定义：
  [chat_tools.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/observation/chat_tools.go)
- 配置示例：
  [peripherals.ros2.zed.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.ros2.zed.json)

结论上，后面接其他 ROS2 图像设备，优先走“新增配置 + 复用 `ros2_topic_read`”这条路；只有当设备能力已经超出“读图像”语义时，才值得新增专用 tool。
