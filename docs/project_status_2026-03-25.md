# 项目状态总览（2026-03-25）

这份文档不是单次测试记录，而是给后续接手的人看的“当前项目状态总览”。

适用对象：

- 第一次接手这个项目的人
- 需要知道 Jetson 现场现在跑到哪一步的人
- 需要继续接 ROS2 设备、继续接本地推理或继续改 agent 行为的人

## 1. 项目现在是什么

这个项目是一个运行在 Jetson 场景下的多模态 Agent 服务。

它当前同时具备两类能力：

- 图像链路：
  从相机或 ROS2 图像 topic 抓一帧，保存到本地，再交给模型读图
- 对话链路：
  用户在网页里输入自然语言，后端通过 Eino agent + tool call 决定是否抓图、是否比较历史图片、是否带上外设状态，然后返回自然语言回答

前端有两套页面：

- 正常对话页面：
  面向“和 agent 对话”
- debug 页面：
  面向“看健康状态、外设状态、最新图片”

## 2. 当前代码主结构

如果你第一次看代码，建议按这一层层往下看：

1. `cmd/jetson_camera_agent/server.go`
   负责 HTTP 路由注册与应用启动
2. `cmd/jetson_camera_agent/handlers.go`
   负责把 HTTP 请求转成 service 调用
3. `cmd/jetson_camera_agent/service.go`
   负责核心业务编排，是当前系统最关键的文件
4. `cmd/jetson_camera_agent/chat_tools.go`
   负责把能力封装成 agent 可调用的 tool
5. `internal/agent/vision_agent.go`
   负责 Eino agent、模型、多图输入、tool calling 与读图后综合回答
6. `cmd/jetson_camera_agent/session_store.go`
   负责 session 历史与最近几张图片记忆
7. `internal/peripherals/*.go`
   负责外设抽象与 driver
8. `internal/camera/*.go`
   负责具体抓图 helper
9. `scripts/*.py`
   负责最底层的 Python 抓图脚本
10. `prompts/system.txt`
   负责约束 agent 的行为

## 3. 当前系统交互链路

### 3.1 正常对话链路

浏览器请求：

- `/api/agent/chat`
- `/api/agent/chat/stream`

处理顺序：

1. `server.go` 注册路由
2. `handlers.go` 解析请求
3. `service.go` 调 `ObservationService.AgentChat(...)`
4. `service.go` 根据用户问题做动作启发：
   - 是否抓新图
   - 是否复用最新图
   - 是否要比较前后两张图
   - 是否附带外设快照
5. `service.go` 调 `VisionAgent.Chat(...)`
6. `vision_agent.go` 内部创建 Eino `ChatModelAgent`
7. agent 可以调用在 `chat_tools.go` 中注册的工具
8. 工具调用外设层和抓图脚本
9. 图片抓到后再回到 `vision_agent.go`
10. 模型读图后生成最终自然语言回答

### 3.2 ROS2 图像链路

当前 ROS2 图像链路的调用顺序是：

1. `ros2_topic_read` 或 `camera_read`
2. `peripherals.Manager`
3. `ros2_topic_device.go`
4. `ros2_topic.go`
5. `scripts/capture_ros2_topic_image.py`
6. `rclpy` 订阅一次 topic
7. 拿到一帧后落盘
8. 图片路径回到 Go
9. 模型继续读图回答

### 3.3 图片比较链路

当前系统已经支持“上一张图 vs 当前图”的比较。

这一能力依赖：

- `session_store.go`
  保存最近几张图片路径
- `service.go`
  识别“对比/区别/变化/上一张图/前一帧”这类问题
- `vision_agent.go`
  支持多图输入，并在 prompt 中明确“第一张是旧图，最后一张是新图”

## 4. 当前已经做成的工程能力

截至当前，本地仓库里已经完成的关键能力有：

- 单模型多模态对话
- tool calling
- 工具后继续读图回答
- 普通观察请求自动抓新帧
- 防止模型只说“需要调用工具”而不执行
- session 记忆最近几张图
- 同一 session 下前后图比较
- ROS2 图像 topic 抓图
- `ros2_topic_read` 作为稳定多设备工具

## 5. 当前可用的核心工具

工具定义在：

- `cmd/jetson_camera_agent/chat_tools.go`

目前最重要的几个 tool：

- `camera_read`
  面向“主视角当前图像”
- `ros2_topic_read`
  面向“按设备名访问 ROS2 图像设备”
- `tool_call_smoke_test`
  面向工具调用链路验证

### 5.1 `camera_read`

适合：

- 当前主相机观察
- 当前画面快速抓新帧
- 复用最近一张主视角图片

输入模式：

- `capture_fresh`
- `use_latest_image`

### 5.2 `ros2_topic_read`

适合：

- 列出当前系统配置了哪些 ROS2 图像设备
- 检查某个 ROS2 设备状态
- 从某个指定设备抓一张图

输入模式：

- `list`
- `inspect`
- `capture`

设计原则：

- 模型不需要知道 topic 名
- 模型只需要知道设备名
- topic / message type / ROS setup 全部由配置和 driver 负责

## 6. prompt 与服务端兜底的分工

这个项目不是单靠 prompt 运行，也不是全靠硬编码。

当前做法是两层配合：

- `prompts/system.txt`
  负责约束 agent 的行为
- `service.go`
  负责业务判断和工程兜底

当前 prompt 约束包括：

- 当前画面请求要尽快调用相机能力
- 工具返回图片后要继续读图并回答
- 不要只返回路径、JSON 或“我需要调用工具”

当前服务端兜底包括：

- 识别“观察当前画面”并优先 fresh capture
- 识别“比较前后两张图”
- 当模型只输出工具规划话术时，自动补抓图并重答

## 7. 本地仓库当前 git 状态

本地当前分支：

- `master`

截至本文开始整理这套文档之前，最近几个重要提交包括：

- `9f35284 Add Jetson ROS2 validation note`
- `60e640a Add ZED ROS2 capture config and setup path fix`
- `b0b462c Improve vision tool flow and session image memory`

这些提交分别对应：

- ROS2 图片链路验证记录
- ZED ROS2 配置与 `ros_setup` 路径展开修复
- tool call 后继续读图、session 图像记忆与比较链路

需要注意：

- 这份文档本身后续也可能继续更新并形成新提交
- 因此不要把本文里的某个 commit hash 当成长期固定的“最新状态”
- 真正接手时，请现场执行 `git status`、`git log --oneline -n 5` 查看实时状态

当前建议始终把本地 `/home/sealessland/inference/eino-vlm-agent-demo` 视为权威仓库。

## 8. Jetson 当前运行状态

以下状态是在 2026-03-25 晚间现场确认的。

### 8.1 当前端口

Jetson 当前可见的监听端口：

- `127.0.0.1:18080`
- `127.0.0.1:18081`
- `127.0.0.1:18084`
- `127.0.0.1:18085`

### 8.2 当前运行实例

Jetson 上当前至少有两套明确可用的服务：

- `18080 / 18081`
  旧的默认实例
- `18084 / 18085`
  当前已验证通过的 ROS2 独立实例

另外还观察到一个额外的 `jetson_camera_agent` 进程：

- `pid=303176`

它当前不对应本文档里确认过的端口，暂时只做记录，不做破坏性处理。

### 8.3 Jetson 当前更像“同步工作目录”，不是干净 git clone

Jetson 上在 `~/inference/eino-vlm-agent-demo` 内执行：

- `git rev-parse --short HEAD`

返回了“需要一个单独的版本”，说明它当前不是一个可直接依赖的标准 git 工作树状态。

同时 `git status --short` 也显示了大量未跟踪路径。

所以当前建议把本地 `/home/sealessland/inference/eino-vlm-agent-demo` 当作主仓库，把 Jetson 目录视为“运行副本 / 同步副本”，而不是权威 git 源。

## 9. Jetson 当前模型与配置状态

本地 `.env` 当前关键值是：

- `OPENAI_MODEL_NAME=qwen3-vl-flash-2026-01-22`
- `JETSON_ENABLE_IMAGE_INPUT=true`
- `JETSON_PERIPHERAL_CONFIG=./configs/peripherals.json`

当前已经额外准备好的 ROS2 配置文件：

- `configs/peripherals.ros2.zed.json`

它对应的核心设备是：

- `front-camera-ros2-zed`

对应 topic：

- `/zed/zed_node/rgb/color/rect/image`

类型：

- `sensor_msgs/msg/Image`

## 10. Jetson 当前已验证通过的能力

### 10.1 ROS2 视频 / 抓帧链路

已验证：

- `ros2 topic hz` 稳定约 `15 Hz`
- 直接 subscriber 抓图成功
- `/api/camera/capture` 连续抓图成功
- `/api/camera/latest.jpg` 与最新 capture 文件一致

### 10.2 正常对话链路

已验证：

- `http://127.0.0.1:18084/` 可打开
- `/api/agent/chat` 可用
- `/api/agent/chat/stream` 可用
- 普通观察请求会直接抓图后回答
- 不会只停在“需要调用工具”

### 10.3 多轮图片比较链路

已验证：

- 同一 `session_id` 下可以比较“上一张图 vs 当前图”
- 当前场景无明显变化时，会明确回答“无明显变化”

## 11. 当前已经留下的文档

本地已经存在的相关文档：

- `docs/ros2_topic_interface_example.md`
- `docs/jetson_ros2_image_link_validation_2026-03-25.md`
- `docs/jetson_ros2_and_chat_e2e_validation_2026-03-25.md`
- `docs/project_status_2026-03-25.md`

建议阅读顺序：

1. `project_status_2026-03-25.md`
2. `jetson_ros2_and_chat_e2e_validation_2026-03-25.md`
3. `jetson_ros2_image_link_validation_2026-03-25.md`
4. `ros2_topic_interface_example.md`

文档同步约定：

- 本地 `/home/sealessland/inference/eino-vlm-agent-demo/docs` 作为权威文档目录
- Jetson `~/inference/eino-vlm-agent-demo/docs` 保留同路径同步副本，方便现场排查
- 若两端内容不一致，优先以本地仓库版本为准，再重新同步到 Jetson

## 12. 当前已知风险与注意事项

### 12.1 Jetson 默认端口还没有切到 ROS2

当前验证通过的是：

- `18084 / 18085`

不是默认的：

- `18080 / 18081`

所以如果要把 ROS2 作为正式主链路，还需要单独做一次默认实例切换。

### 12.2 Jetson 目录当前不适合当主 git 仓库

Jetson 当前目录不是干净的 git 工作树。

建议：

- 本地仓库做主
- Jetson 做同步运行副本

### 12.3 Jetson 上存在多个 agent 进程

目前观察到多个 `jetson_camera_agent` 进程并存。

在没有明确清理方案前，建议：

- 先按端口识别实例
- 避免直接做破坏性 kill
- 每次验证时写清楚是在哪个端口测的

## 13. 后续接手时的推荐动作

如果下一个人要继续推进，最合理的顺序是：

1. 先读本文件，了解当前状态。
2. 打开 `docs/jetson_ros2_and_chat_e2e_validation_2026-03-25.md` 看验收细节。
3. 看 `cmd/jetson_camera_agent/service.go` 理解业务编排。
4. 看 `cmd/jetson_camera_agent/chat_tools.go` 和 `internal/agent/vision_agent.go` 理解 tool call 与读图逻辑。
5. 看 `configs/peripherals.ros2.zed.json` 理解当前 ROS2 配置。
6. 若要新增 ROS2 图像设备，优先走“新增配置 + 复用 `ros2_topic_read`”。
7. 若要切换默认主链路，再单独处理 `18080` 默认实例。
8. 若要接本地推理服务，再按单模型多模态配置替换 `OPENAI_BASE_URL` 与 `OPENAI_MODEL_NAME`。

## 14. 一句话总结

截至 2026-03-25，项目本地代码主线已经具备“工具调用后读图回答、会话图像记忆、前后图比较、ROS2 图像设备接入”这几项关键能力；Jetson 现场已经把 ROS2 图像链路和正常对话链路都打通，但 ROS2 还没有正式替换默认 `18080` 实例。
