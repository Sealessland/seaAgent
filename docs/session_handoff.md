# Session Handoff

这份文档是给“下一个会话”用的快速接手说明。

如果你刚进入这个仓库，优先读这份文档，再决定继续改哪一块。

## 1. 项目现在是什么

这是一个跑在 Jetson 场景里的 Go + Eino agent demo，已经被整理成“用户前端 / debug 前端 / agent 编排 / 外设接入”分层结构。

当前目标不是做一个单纯 demo 页面，而是做一个便于 AI 和人继续演进的嵌入式 agent 工程骨架。

## 2. 当前架构边界

- 用户端口：`127.0.0.1:18080`
  - 主前端
  - 主 API
  - 用户只感知对话
- Debug 端口：`127.0.0.1:18081`
  - 外设状态
  - 调试页
  - SSE 状态流

代码职责：

- `cmd/jetson_camera_agent`
  - 服务入口、HTTP、配置、service、tool 注册
- `internal/agent`
  - Eino 对话模型封装
  - tool-calling 对话执行
  - 多轮对话请求/响应结构
- `internal/peripherals`
  - 外设抽象
  - 设备配置
  - 设备管理器
  - 设备 driver
- `internal/camera`
  - 具体采图 helper
- `front-end`
  - 用户对话前端，已裁成 chat-only
- `debug-front-end`
  - 调试前端
- `configs/peripherals.json`
  - 当前实际外设配置
- `configs/peripherals.example.json`
  - 扩展示例配置

## 3. 已经完成了什么

### 3.1 main / 后端分层

单文件 `main` 已拆分：

- `main.go`
- `config.go`
- `server.go`
- `handlers.go`
- `service.go`
- `session_store.go`
- `tools.go`
- `chat_tools.go`

`.env` 和 system prompt 已剥离：

- `.env`
- `.env.example`
- `prompts/system.txt`

### 3.2 前后端分离

已经拆成：

- `front-end`
  - Preact + Vite
  - 用户态只保留对话体验
- `debug-front-end`
  - Preact + Vite
  - 独立调试页

主前端已经去掉和对话无关的说明性内容。

### 3.3 多轮对话

已经实现：

- `session_id`
- 服务端会话持久化
- 历史摘要压缩
- 流式对话接口 `/api/agent/chat/stream`

会话存储位置：

- `JETSON_AGENT_WORKDIR/sessions`

### 3.4 Tool calling

已经不是“服务层自己决定调函数”的假 tool 了，现在是真正接到了 Eino ADK：

- `internal/agent/vision_agent.go`
  - 使用 `adk.NewChatModelAgent`
  - 使用 `adk.NewRunner`
  - 从 `AsyncIterator[*AgentEvent]` 解析：
    - assistant 事件中的 `ToolCalls`
    - tool 事件中的 `ToolMessage`

当前已注册的对话工具：

- `camera_read`
- `tool_call_smoke_test`
- `ros2_topic_read`

### 3.5 tool smoke test 已打通

已实测：

- `GET /api/agent/capabilities`
- `POST /api/agent/chat`
- `POST /api/agent/chat/stream`

都可用。

固定测试提示词：

```text
/tooltest 请调用 tool_call_smoke_test，token 使用 smoke-001，并根据工具结果简短回复。
```

成功时返回会包含：

- `reply`
- `trace.tool_calls`
- `tool_call_smoke_test`

### 3.6 前端并发发送已阻塞

主前端已经做了“上一条没处理完，下一条不允许发”：

- 禁止再次发送
- 禁止会话切换
- 禁止新建对话
- 冻结一次请求的 `history`

避免流式返回时把历史弄乱。

### 3.7 ZED 老链路保留

ZED 仍然走老路径：

- `scripts/capture_zed_frame.py`
- `pyzed.sl`

没有被 ROS2 topic 新接口替换。

### 3.8 ROS2 topic 扩展接口已落地

新增了独立的 `ros2_topic` 接口面：

- `internal/peripherals/ros2_topic_device.go`
- `internal/camera/ros2_topic.go`
- `scripts/capture_ros2_topic_image.py`

并且 agent 可通过：

- `ros2_topic_read`

去：

- `list`
- `inspect`
- `capture`

ROS2 设备。

## 4. 关键设计结论

### 4.1 ZED 为什么还走老路径

这是刻意保留的：

- ZED 当前真实链路已经跑通到 `pyzed` 这一层
- 不应该在引入 ROS2 接口时顺手把 ZED 采图路径一起推翻

所以当前策略是：

- `zed`
  - 保持原有 `pyzed` 链路
- `ros2_topic`
  - 作为新增 driver 独立存在

### 4.2 ROS2 取图为什么不是 `ros2 topic echo`

调研后的结论是：

- 调试可以用 `ros2 topic list/info`
- 但真正抓图不该靠 `ros2 topic echo`

当前实现选的是：

- 一个最小 `rclpy` subscriber
- 支持：
  - `sensor_msgs/msg/Image`
  - `sensor_msgs/msg/CompressedImage`

原因：

- topic 是类型化消息，不是普通文本
- 图像要经过 `cv_bridge` 或 `cv2.imdecode`
- subscriber 比 CLI echo 更稳定，也更接近真实工程实践

## 5. 已验证的事实

### 5.1 DeepSeek API 是通的

已直接请求过：

- `https://api.deepseek.com/models`

返回了：

- `deepseek-chat`
- `deepseek-reasoner`

当前 `.env` 配置的是：

- `OPENAI_BASE_URL=https://api.deepseek.com`
- `OPENAI_MODEL_NAME=deepseek-chat`

### 5.2 后端本地接口是通的

已实测成功：

- `/api/agent/capabilities`
- `/api/agent/chat`
- `/api/agent/chat/stream`

### 5.3 编译 / 测试状态

已通过：

```bash
go test ./internal/agent ./internal/peripherals ./cmd/jetson_camera_agent
```

主前端也通过：

```bash
cd front-end
npm run build
```

## 6. 当前没有真正跑通的地方

### 6.1 ZED Python 依赖

当前环境里 ZED 路径仍然不通，原因是：

- 缺 `pyzed`

这会在状态里表现成：

- `ModuleNotFoundError: No module named 'pyzed'`

### 6.2 ROS2 Python 依赖

当前环境里也没有这些 Python 包：

- `rclpy`
- `cv_bridge`
- `sensor_msgs`
- `numpy`
- `cv2`

所以结论要说清楚：

- ROS2 接口结构已经就位
- 运行时依赖当前机器还没装
- 也就是说“接口可接入”，但“现场立即可跑”还差环境准备

## 7. 最值得优先做的下一步

按优先级排序，建议下一个会话优先做下面几件事。

### 7.1 补 readiness / diagnostics

优先级最高。

现在最缺的是一个明确的 readiness 接口，告诉用户：

- DeepSeek 可用不可用
- ZED SDK 可用不可用
- ROS2 Python 依赖可用不可用
- 某个 topic 可见不可见

建议做法：

- 新增一个统一 readiness API
- 或新增一个 `environment_readiness` tool

### 7.2 把 tool 返回格式做成人类可读

现在 `/tooltest` 返回的是工具结果 JSON。

这没错，但产品体验一般。

下一步可以：

- 保留 `trace.tool_calls`
- 同时把 `reply` 包一层自然语言总结

### 7.3 真正装齐 ROS2 / ZED 环境

不是代码问题，是运行环境问题。

如果下一会话要打通真实 ROS2 topic 抓图，就要补：

- ROS2 Python 环境
- `cv_bridge`
- 图像消息包
- OpenCV/numpy

如果下一会话要打通真实 ZED 抓图，就要补：

- `pyzed`
- ZED SDK

## 8. 重要文件清单

下一个会话大概率会反复用到这些文件：

- `cmd/jetson_camera_agent/service.go`
- `cmd/jetson_camera_agent/handlers.go`
- `cmd/jetson_camera_agent/chat_tools.go`
- `cmd/jetson_camera_agent/tools.go`
- `cmd/jetson_camera_agent/session_store.go`
- `internal/agent/vision_agent.go`
- `internal/agent/vision_agent_test.go`
- `internal/peripherals/config.go`
- `internal/peripherals/manager.go`
- `internal/peripherals/zed_device.go`
- `internal/peripherals/ros2_topic_device.go`
- `internal/camera/zed.go`
- `internal/camera/ros2_topic.go`
- `scripts/capture_zed_frame.py`
- `scripts/capture_ros2_topic_image.py`
- `front-end/src/App.tsx`
- `front-end/src/api.ts`
- `configs/peripherals.json`
- `configs/peripherals.example.json`
- `docs/zed_capture_flow_example.md`
- `docs/ros2_topic_interface_example.md`

## 9. 给下一个会话的建议起手式

建议先做这几步：

1. 读这份文档
2. 再读：
   - `README.md`
   - `docs/zed_capture_flow_example.md`
   - `docs/ros2_topic_interface_example.md`
3. 再看：
   - `cmd/jetson_camera_agent/service.go`
   - `internal/agent/vision_agent.go`
   - `internal/peripherals/manager.go`
4. 再决定本轮需求属于：
   - 用户前端
   - debug 侧
   - agent/tool calling
   - 外设接入
   - 环境 readiness

## 10. 最短结论

这个项目现在已经不是“一个单文件 demo”了，而是一套可继续演进的嵌入式 agent 骨架：

- 用户前端和 debug 前端分离
- 主对话链路已具备多轮、流式、tool calling
- DeepSeek 官方 API 已验证连通
- ZED 老路径保留
- ROS2 topic 接口已经按可扩展方式落地

当前真正的主要阻塞不在架构，而在运行环境依赖。

## 11. 不要回退的约束

下一个会话继续改时，优先保住这些约束，不要无意中回退：

- 用户前端只负责对话，不要把 debug 信息重新塞回主页面
- debug 能力继续留在 `18081`
- ZED 继续走老链路，不要把它强行改成 ROS2 topic
- ROS2 topic 是新增路径，不是替换路径
- tool calling 继续走 Eino ADK
  - 不要退回“服务层手写假 tool loop”
- 主前端要保持“上一条未完成时，下一条不能发”
- system prompt 继续走文件化配置
  - 不要重新硬编码回 Go 代码

## 12. 当前已知坑

这些是已经踩过的坑，下一个会话不用再重复踩一遍。

### 12.1 `tool_choice=forced` 不能一直挂整个循环

之前 `/tooltest` 的问题就是：

- `tool_choice=forced`
- DeepSeek 会持续要求继续调工具
- 最后触发 `exceeds max iterations`

现在修法是：

- `tool_call_smoke_test` 走 `ReturnDirectly`

所以如果后面再加类似“验证型工具”，优先考虑：

- `ReturnDirectly`
- 或单轮限定工具

不要把 forced tool choice 粗暴挂满整个 ReAct 循环。

### 12.2 配置里 `VISION_SYSTEM_PROMPT` 曾经读错过

之前有个配置 bug：

- 代码错误强依赖 `VISION_SYSTEM_PROMPT`
- 但 `.env` 实际已经改成 `VISION_SYSTEM_PROMPT_FILE`

现在已经修掉。

如果后面再动配置加载，优先检查：

- `cmd/jetson_camera_agent/config.go`
- `.env`
- `.env.example`
- `prompts/system.txt`

### 12.3 旧进程会占端口

之前确实出现过：

- `18080`
- `18081`

被残留的 `jetson_camera_agent` 进程占住。

如果服务起不来，先查端口占用，不要先怀疑业务代码。

### 12.4 在当前 Codex sandbox 里直接起服务可能失败

这个环境里，直接在 sandbox 内监听端口可能会失败。

之前见到过：

- `listen tcp 127.0.0.1:18081: socket: operation not permitted`

所以如果下一个会话需要再次做端到端验证，记住：

- 编译/测试可以在 sandbox 内做
- 真正起服务和打本地端口，可能需要沙箱外执行

## 13. 最有用的验证命令

如果下一个会话要快速判断“现在到底坏没坏”，优先跑这些命令。

### 13.1 Go 测试

```bash
GOCACHE=/home/sealessland/inference/eino-vlm-agent-demo/.cache/go-build \
GOMODCACHE=/home/sealessland/inference/eino-vlm-agent-demo/.cache/go-mod \
go test ./internal/agent ./internal/peripherals ./cmd/jetson_camera_agent
```

### 13.2 主前端构建

```bash
cd front-end
npm run build
```

### 13.3 agent 能力接口

```bash
curl -sS http://127.0.0.1:18080/api/agent/capabilities
```

### 13.4 smoke test

```bash
curl -sS http://127.0.0.1:18080/api/agent/chat \
  -H 'Content-Type: application/json' \
  --data-binary '{"message":"/tooltest 请调用 tool_call_smoke_test，token 使用 smoke-001，并根据工具结果简短回复。","use_latest_image":false,"include_snapshot":false}'
```

期望看到：

- `reply`
- `trace.tool_calls`
- `tool_call_smoke_test`

### 13.5 流式接口

```bash
curl -sS http://127.0.0.1:18080/api/agent/chat/stream \
  -H 'Content-Type: application/json' \
  --data-binary '{"message":"/tooltest 请调用 tool_call_smoke_test，token 使用 smoke-002，并根据工具结果简短回复。","use_latest_image":false,"include_snapshot":false}'
```

期望看到：

- `event: status`
- `event: meta`
- `event: delta`
- `event: done`

### 13.6 DeepSeek upstream

```bash
curl -sS https://api.deepseek.com/models \
  -H 'Authorization: Bearer <OPENAI_API_KEY>'
```

### 13.7 端口占用

```bash
ss -ltnp | rg ':18080|:18081'
```

## 14. 当前最推荐的下一轮任务

如果没有新的明确业务指令，下一个会话最推荐从这里开始：

1. 做一个 readiness / diagnostics 接口
   - DeepSeek
   - ZED
   - ROS2 Python 依赖
   - topic 可见性
2. 把 `tool` 的原始 JSON 返回包装成人类可读回复
3. 继续补 ROS2 接入文档和配置样例
4. 如果环境允许，再真正装齐 ROS2 / ZED 依赖

## 15. 下一个会话的最短起手提示

如果要给下一个会话一句最短提示，可以直接用：

```text
先读 docs/session_handoff.md，再看 docs/zed_capture_flow_example.md 和 docs/ros2_topic_interface_example.md。当前主链路、tool calling、DeepSeek 连通都已验证；主要未完成项是 readiness 和运行环境依赖。
```
