# ZED 2i 取图链路示例

这份文档说明当前项目里“从 ZED 2i 拿一张图片”是怎么实现的，以及失败时该去哪里排查。

目标不是描述抽象概念，而是把当前真实链路按代码路径写清楚，方便继续演进或让 AI 直接改。

## 1. 总览

当前取图链路是：

1. 用户或 agent 触发一次取图
2. Go 后端调用 `ObservationService`
3. `ObservationService` 通过外设管理器找到主采集设备
4. 主采集设备当前配置为 `front-zed`
5. `front-zed` 的 driver 是 `zed`
6. `zed` driver 最终调用 Python 脚本 `scripts/capture_zed_frame.py`
7. Python 脚本通过 `pyzed.sl` 打开 ZED，相机抓到一帧后落盘到指定路径
8. 脚本把结果用 JSON 打回 Go
9. Go 把图片路径和元数据继续交给后续 agent / API 返回

也就是说，当前不是浏览器拿图，不是 ROS topic 直接取图，也不是 OpenCV 直接连 USB。

当前真实取图入口是 `pyzed` SDK。

## 2. 配置入口

ZED 是通过统一外设配置注册进系统的。

配置文件：

- [configs/peripherals.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.json)

当前配置里：

- `primary_capture_device = front-zed`
- 设备 `front-zed`
- `driver = zed`
- `capture.script = ./scripts/capture_zed_frame.py`

这意味着：

- 默认主采集设备就是这台 ZED
- Go 不关心 ZED 细节，只认“这是主采集设备”
- 真正的抓帧实现交给 `zed` driver

## 3. Go 侧调用链

### 3.1 用户态 / agent 态入口

有两条常见入口：

- HTTP 接口 `/api/camera/capture`
- agent/tool call 里的 `camera_read`

对应代码：

- [handlers.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/jetson_camera_agent/handlers.go)
- [tools.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/jetson_camera_agent/tools.go)
- [chat_tools.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/jetson_camera_agent/chat_tools.go)

不管从哪条入口进，最终都会走到：

- [service.go](/home/sealessland/inference/eino-vlm-agent-demo/cmd/jetson_camera_agent/service.go)

其中：

- `ObservationService.CapturePrimary(...)`
- `cameraReadTool.Capture(...)`

都会继续调用：

- `peripherals.Manager.CapturePrimary(...)`

### 3.2 外设管理器如何决定“该找谁拍照”

代码：

- [manager.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/manager.go)

`Manager` 初始化时会读取配置中的：

- `primary_capture_device`

然后在 `CapturePrimary(...)` 里：

1. 找到名为 `front-zed` 的设备
2. 检查它是否实现了 `CaptureDevice`
3. 调用这个设备自己的 `Capture(ctx, outputPath)`

抽象定义在：

- [types.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/types.go)

这里的关键接口是：

```go
type CaptureDevice interface {
    Capture(ctx context.Context, outputPath string) (*CaptureResult, error)
}
```

这也是为什么后面接雷达、工业相机、双目、热成像时，可以继续复用同一套调度方式。

## 4. ZED driver 如何接 Python

代码：

- [zed_device.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/zed_device.go)

`zed` driver 做的事很少，但边界很清晰：

1. 要求配置里必须有 `capture.script`
2. 没配就直接报错
3. `Capture(...)` 时调用：

```go
return camera.CaptureWithPython(ctx, d.cfg.Capture.Script, outputPath)
```

也就是说：

- ZED driver 本身不直接嵌入 SDK
- Go 不直接 import `pyzed`
- ZED SDK 依赖被隔离在 Python 脚本里

这样做的实际好处是：

- Go 主服务不需要链接 ZED 原生库
- Jetson 上 Python 环境、ZED SDK 环境、Go 服务环境可以松耦合
- 以后换成别的脚本或别的采集程序，不需要重写上层接口

## 5. Python 脚本如何真正抓帧

代码：

- [capture_zed_frame.py](/home/sealessland/inference/eino-vlm-agent-demo/scripts/capture_zed_frame.py)

核心步骤如下。

### 5.1 接收输出路径

脚本启动参数：

```bash
python3 ./scripts/capture_zed_frame.py --output /tmp/jetson-camera-agent/capture-xxx.jpg
```

Go 会提前生成输出文件名并传进来。

### 5.2 初始化 ZED 相机

脚本里做了这些配置：

- 分辨率：`HD720`
- 帧率：`15 FPS`
- 深度模式：`DEPTH_MODE.NONE`

这里关闭了深度，仅抓左目彩色图。

也就是说当前链路里：

- 视觉推理拿的是左目 RGB 图
- 不是深度图
- 不是点云

### 5.3 打开相机

核心调用：

```python
camera = sl.Camera()
init = sl.InitParameters()
status = camera.open(init)
```

如果 `open` 失败，脚本不会抛异常给 Go，而是打印 JSON：

```json
{
  "ok": false,
  "error": "...",
  "raw_output": "camera open failed"
}
```

然后返回。

这点很重要：Go 侧依赖的是“脚本输出的 JSON”，不是进程退出码本身。

### 5.4 连续 grab，最多尝试 10 次

脚本不是只 grab 一次，而是：

- 最多重试 10 次
- 每次失败 sleep `0.1s`

这样做是为了避免刚打开相机时第一帧还没准备好。

### 5.5 取左目图像并转成 OpenCV 格式

成功后：

```python
camera.retrieve_image(image, sl.VIEW.LEFT)
frame = image.get_data()
```

如果拿到的是 4 通道 BGRA，会转成 3 通道 BGR：

```python
frame = cv2.cvtColor(frame, cv2.COLOR_BGRA2BGR)
```

然后：

```python
cv2.imwrite(args.output, frame)
```

最终图片就真正落盘了。

### 5.6 返回 JSON 给 Go

成功时返回类似：

```json
{
  "ok": true,
  "output": "/tmp/jetson-camera-agent/capture-xxx.jpg",
  "width": 1280,
  "height": 720,
  "camera_sn": "xxxxxx"
}
```

Go 后续就拿这个 `output` 路径继续做视觉推理。

## 6. Go 如何解析 Python 返回值

代码：

- [zed.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/camera/zed.go)

`CaptureWithPython(...)` 做了这些事：

1. 执行：

```bash
python3 <script> --output <path>
```

2. 读取 stdout/stderr 合并输出
3. 从输出文本里提取最后一个 JSON
4. 反序列化成 `CaptureResult`

这里兼容了一种很现实的情况：

- Python 脚本中途可能会输出日志
- 但最后仍然打印一段 JSON

所以它不是要求“整个输出必须纯 JSON”，而是通过 `extractLastJSON(...)` 从最后一段 JSON 里提结果。

这让链路对现场调试更宽容。

## 7. 为什么当前日志里会显示 pyzed 不可用

当前环境里你已经看到过这类错误：

```text
ModuleNotFoundError: No module named 'pyzed'
```

这不是前端问题，也不是 agent 问题，而是 ZED Python 运行环境没准备好。

按当前实现，只要下面任一条件不满足，ZED 抓帧就会失败：

- `pyzed` Python 模块可导入
- ZED SDK 已正确安装
- 设备在 USB 上可见
- `camera.open(...)` 能成功
- `grab(...)` 能拿到帧

## 8. 当前诊断是怎么做的

除了抓图，ZED 设备还会跑一组检查项，帮助定位失败原因。

检查配置和默认实现见：

- [configs/peripherals.json](/home/sealessland/inference/eino-vlm-agent-demo/configs/peripherals.json)
- [zed_device.go](/home/sealessland/inference/eino-vlm-agent-demo/internal/peripherals/zed_device.go)

当前检查包括：

- `usb`
  - 看 `lsusb` 里有没有 ZED
- `ros_topics`
  - 看 ROS2 里有没有 `/zed*` 相关 topic
- `zed_log_tail`
  - 看 `/tmp/zed_launch.log`
- `pyzed_probe`
  - 直接跑一个最小 Python probe，测试 `pyzed` 和 `camera.open(...)`

这些检查最后会汇总成 `DeviceSnapshot.Summary`，例如：

- `ZED device is not visible on USB.`
- `ZED ROS2 topics are visible. Frame transport may be available.`

所以当前这套设计不是“抓图失败只能看一个 error string”，而是会附带一套最小现场诊断。

## 9. 一个实际调用例子

### 9.1 通过 HTTP

直接调用：

```bash
curl http://127.0.0.1:18080/api/camera/capture
```

链路会变成：

`HTTP -> ObservationService -> peripherals.Manager -> zedDevice -> CaptureWithPython -> capture_zed_frame.py`

### 9.2 通过 agent tool call

当前 agent 注册了：

- `camera_read`

当模型决定调用：

```json
{"mode":"capture_fresh"}
```

时，实际还是会走同一条底层抓图链路。

所以你可以把它理解成：

- `camera_read` 是 agent 能看到的 tool
- `CapturePrimary` 是服务层能力
- `zedDevice.Capture` 是设备层能力
- `capture_zed_frame.py` 是最终执行器

## 10. 为什么这套实现适合后续扩展

这套实现的关键点不是“写了个 Python 脚本”，而是边界清楚：

- 外设选择由配置决定
- 主采集设备由 `Manager` 决定
- 设备能力由 `CaptureDevice` 接口定义
- ZED 特殊逻辑收口在 `zed` driver
- 真正与 SDK 耦合的地方隔离在 Python

所以后面如果要扩展：

- 深度相机
- 热像仪
- 工业相机
- 雷达快照
- IMU / 串口采样

都可以继续沿用这个模式：

1. 在 `configs/peripherals.json` 注册
2. 实现对应 driver
3. 保持 `Manager -> Device -> Capture/Inspect` 这层接口不变

## 11. 当前链路的限制

当前实现有几个明确限制：

- 只抓左目图 `sl.VIEW.LEFT`
- 没把深度图一起导出
- 没做长驻相机连接复用，每次抓图都会重新 `open -> grab -> close`
- Python 运行环境必须单独维护
- 返回的是本地图片文件路径，不是共享内存或零拷贝 buffer

这意味着它的优点是简单、稳定、边界清楚；
代价是实时性和吞吐量不是最优。

如果以后要优化，可以考虑：

- 常驻采集进程
- Go 侧通过 IPC / socket 请求一帧
- 同时导出 RGB + depth
- 增加曝光、分辨率、左右目、裁剪等参数

## 12. 最短结论

当前项目里，从 ZED 2i 拿图片的真实实现就是：

`Go 服务通过外设管理器找到主设备 front-zed -> zed driver 调用 Python 脚本 -> Python 用 pyzed SDK 打开 ZED 并抓取左目一帧 -> 图片落盘 -> JSON 结果返回 Go`

如果抓图失败，优先看：

1. `pyzed_probe`
2. `lsusb`
3. `/tmp/zed_launch.log`
4. `camera.open(...)` 是否成功

