# Eino VLM Agent Demo

这套工程用于在 Jetson 本地验证这条链路：

`ZED 2i -> Jetson 本地抓帧脚本 -> Go/Eino agent -> 本地 vLLM`

它不依赖 `live-vlm-webui`。`webui` 只作为之前排查消息链路时的参考，不参与这里的运行闭环。

## 结构

- `cmd/jetson_camera_agent`
  - Jetson 本地 HTTP 服务，默认监听 `127.0.0.1:18080`
- `internal/camera`
  - 摄像头抓帧和状态探测
- `internal/agent`
  - Eino 多模态代理封装
- `scripts/capture_zed_frame.py`
  - 直接通过 ZED SDK 抓单帧

## 依赖

- Jetson 上本地 vLLM 已运行:
  - `http://127.0.0.1:8000/v1`
  - model: `Qwen3.5-2B-local`
- Go 已安装
- Python3 可导入:
  - `pyzed`
  - `cv2`

## 运行

```bash
cd ~/inference/eino-vlm-agent-demo
go mod tidy
go build ./cmd/jetson_camera_agent

OPENAI_BASE_URL=http://127.0.0.1:8000/v1 \
OPENAI_MODEL_NAME=Qwen3.5-2B-local \
OPENAI_API_KEY=EMPTY \
./jetson_camera_agent
```

访问:

```bash
http://127.0.0.1:18080
```

## Jetson 上的实际使用方法

当前 Jetson 本地服务监听：

```bash
127.0.0.1:18080
```

直接在 Jetson 本机浏览器打开：

```bash
http://127.0.0.1:18080
```

## 管理脚本

工程目录里带了一个管理脚本：

```bash
~/inference/eino-vlm-agent-demo/manage_jetson_camera_agent.sh
```

常用命令：

```bash
cd ~/inference/eino-vlm-agent-demo

./manage_jetson_camera_agent.sh start
./manage_jetson_camera_agent.sh stop
./manage_jetson_camera_agent.sh restart
./manage_jetson_camera_agent.sh status
./manage_jetson_camera_agent.sh health
./manage_jetson_camera_agent.sh capture
./manage_jetson_camera_agent.sh preview-head
./manage_jetson_camera_agent.sh analyze "Describe the current camera view briefly."
./manage_jetson_camera_agent.sh logs
```

用途说明：

- `start`
  - 编译并启动 `127.0.0.1:18080` 的服务
- `stop`
  - 停止当前 camera agent
- `restart`
  - 重启服务
- `status`
  - 查看当前进程和监听配置
- `health`
  - 检查 agent 到本地 vLLM 是否可用
- `capture`
  - 直接抓一帧
- `preview-head`
  - 检查最新抓图接口 `/api/camera/latest.jpg`
- `analyze`
  - 抓帧并交给 Go/Eino agent
- `logs`
  - 查看 `/tmp/jetson_camera_agent.log`

页面里的按钮对应关系：

- `检查 vLLM`
  - 验证本地 `http://127.0.0.1:8000/v1` 是否可用
- `检查摄像头状态`
  - 查看 ZED USB、ROS2 topic、ZED 日志、SDK probe 状态
- `只抓一帧`
  - 直接从 Jetson 本机抓一张图片
  - 抓完后页面会显示最新抓到的图片预览
- `抓帧并交给 Agent`
  - 先抓一帧
  - 再把这张图交给 Go/Eino agent
  - 最后调用 Jetson 本地 vLLM 返回文本结果

如果你想直接测接口，可以在 Jetson 上执行：

```bash
curl http://127.0.0.1:18080/api/health
curl http://127.0.0.1:18080/api/camera/status
curl http://127.0.0.1:18080/api/camera/capture
curl -I http://127.0.0.1:18080/api/camera/latest.jpg
curl -X POST http://127.0.0.1:18080/api/capture-and-analyze \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Describe the current camera view briefly."}'
```

图片预览接口：

```bash
http://127.0.0.1:18080/api/camera/latest.jpg
```

如果页面能看到最新抓图，说明这条链已经通了：

```text
ZED 2i -> Jetson 本地抓帧 -> 18080 服务
```

如果 `抓帧并交给 Agent` 也返回文本，说明完整链路已经通了：

```text
ZED 2i -> Jetson 本地抓帧 -> Go/Eino agent -> 本地 vLLM
```

## 测试接口

- `GET /api/health`
  - 检查本地 vLLM `/v1/models`
- `GET /api/camera/status`
  - 检查 USB、ROS2 topic、ZED 日志、直接 SDK probe
- `GET /api/camera/capture`
  - 直接抓一帧，不跑模型
- `POST /api/capture-and-analyze`
  - 直接抓一帧并交给 Go/Eino agent

## 结果解释

如果 `capture` 就失败，说明问题在 ZED SDK / 设备开流层，还没有进入 agent。

如果 `capture` 成功但 `capture-and-analyze` 失败，说明问题在 Eino / vLLM 调用层。
