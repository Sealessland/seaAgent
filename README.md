# Jetson Agent Console

接手仓库时，优先先读：

- [docs/session_handoff.md](/home/sealessland/inference/eino-vlm-agent-demo/docs/session_handoff.md)

这个仓库的目标很直接：

- 在 Jetson 上运行一个可修改的 Go/Eino agent 服务
- 把外设接入、agent 编排、用户前端、debug 前端分开
- 让人和 AI 都能快速看懂、修改、扩展

## 当前边界

- 用户端口：`127.0.0.1:18080`
  - 正常对话
  - 不暴露摄像头/外设调试细节
- Debug 端口：`127.0.0.1:18081`
  - 外设状态
  - 最新图像
  - 原始检查输出
  - SSE 实时流

## 核心链路

```text
Peripheral Config -> Peripheral Manager -> Observation Service -> Agent API -> Frontend
                                                   |
                                                   +-> Debug API / SSE
```

图像链路：

```text
ZED 2i -> scripts/capture_zed_frame.py -> internal/camera -> ObservationService
```

推理链路：

```text
Frontend -> /api/agent/chat -> ObservationService -> internal/agent -> Model API
```

## 目录

- `cmd/jetson_camera_agent`
  - 服务入口、配置、路由、HTTP handler、fallback 页面
- `internal/agent`
  - Eino 模型封装
  - 多轮对话
  - 基础 RAG / action planning
- `internal/peripherals`
  - 外设抽象
  - 外设管理器
  - 设备驱动适配
- `internal/camera`
  - 具体摄像头抓帧实现
- `configs/peripherals.json`
  - 外设注册入口
- `front-end`
  - 用户态前端
  - Preact + Vite
- `debug-front-end`
  - Debug 前端
  - Preact + Vite
- `scripts`
  - 设备脚本

## 配置

主要配置都在 `.env`：

- `JETSON_AGENT_LISTEN_ADDR`
- `JETSON_DEBUG_LISTEN_ADDR`
- `OPENAI_BASE_URL`
- `OPENAI_API_KEY`
- `OPENAI_MODEL_NAME`
- `VISION_SYSTEM_PROMPT`
- `JETSON_DEFAULT_PROMPT`
- `JETSON_PERIPHERAL_CONFIG`
- `JETSON_FRONTEND_DIST_DIR`
- `JETSON_DEBUG_DIST_DIR`

外设统一配置在 `configs/peripherals.json`。

## 外设扩展规则

新增外设时，优先只改这三层：

1. `configs/peripherals.json`
2. `internal/peripherals`
3. 必要时新增驱动脚本

不要先改前端。

用户前端不感知外设调试细节，debug 前端专门承接状态展示。

## AI 修改建议

如果你是 AI，要优先遵守下面这组修改顺序：

1. 先确认需求属于哪一层：
   - 用户对话
   - agent 编排
   - 外设接入
   - debug 展示
2. 优先改最靠近职责边界的文件
3. 不把 debug 逻辑重新塞回用户前端
4. 不把具体设备细节硬编码进通用 agent 层
5. 新增外设优先走 `configs/peripherals.json + internal/peripherals`

## 当前接口

用户接口：

- `GET /api/config`
- `GET /api/health`
- `GET /api/agent/capabilities`
- `POST /api/agent/chat`

外设/调试接口：

- `GET /api/peripherals`
- `GET /api/peripherals/stream`
- `GET /api/camera/status`
- `GET /api/camera/capture`
- `GET /api/camera/latest.jpg`
- `POST /api/capture-and-analyze`

## 运行

后端：

```bash
cd ~/inference/eino-vlm-agent-demo
go build ./cmd/jetson_camera_agent
./jetson_camera_agent
```

用户前端：

```bash
cd ~/inference/eino-vlm-agent-demo/front-end
npm install
npm run dev
```

Debug 前端：

```bash
cd ~/inference/eino-vlm-agent-demo/debug-front-end
npm install
npm run dev
```

## 状态

现在这套工程已经具备：

- 用户态对话前端
- 独立 debug 前端
- 外设统一注册
- 基础 RAG
- 基础 action planning
- 多轮对话

还明确保留了一个扩展位：

- TODO: 注册更多外设驱动和统一注册 UI
