package observation

import (
	"context"
	"strings"
	"time"

	"eino-vlm-agent-demo/internal/peripherals"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type cameraReadToolInput struct {
	Mode string `json:"mode" jsonschema:"required,enum=capture_fresh,enum=use_latest_image" jsonschema_description:"capture_fresh captures a new frame. use_latest_image reuses the most recent capture."`
}

type cameraReadToolOutput struct {
	OK         bool   `json:"ok"`
	Mode       string `json:"mode"`
	ImagePath  string `json:"image_path,omitempty"`
	Error      string `json:"error,omitempty"`
	Primary    string `json:"primary,omitempty"`
	CapturedAt string `json:"captured_at,omitempty"`
}

type smokeTestToolInput struct {
	Token string `json:"token" jsonschema:"required" jsonschema_description:"Opaque token that should be echoed back for tool-call verification."`
}

type smokeTestToolOutput struct {
	OK         bool   `json:"ok"`
	Token      string `json:"token"`
	Echo       string `json:"echo"`
	VerifiedAt string `json:"verified_at"`
}

type ros2TopicToolInput struct {
	Mode   string `json:"mode" jsonschema:"required,enum=list,enum=inspect,enum=capture" jsonschema_description:"list returns all configured ROS2 topic devices. inspect returns one device snapshot. capture grabs one image from a configured ROS2 image topic device."`
	Device string `json:"device,omitempty" jsonschema_description:"Configured peripheral device name. Required for inspect and capture."`
}

type ros2TopicDeviceInfo struct {
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Driver          string            `json:"driver"`
	SupportsCapture bool              `json:"supports_capture"`
	Summary         string            `json:"summary"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type ros2TopicToolOutput struct {
	OK         bool                  `json:"ok"`
	Mode       string                `json:"mode"`
	Device     string                `json:"device,omitempty"`
	ImagePath  string                `json:"image_path,omitempty"`
	CapturedAt string                `json:"captured_at,omitempty"`
	Devices    []ros2TopicDeviceInfo `json:"devices,omitempty"`
	Error      string                `json:"error,omitempty"`
}

func newDialogueTools(workdir string, peripheralsManager *peripherals.Manager) ([]einotool.InvokableTool, error) {
	cameraTool, err := utils.InferTool(
		"camera_read",
		"Read the primary camera when the user asks for a fresh or latest visual observation. Use mode=capture_fresh for a new frame and mode=use_latest_image to reuse the newest local capture.",
		func(ctx context.Context, input cameraReadToolInput) (cameraReadToolOutput, error) {
			adapter := newCameraReadTool(workdir, peripheralsManager)
			output := cameraReadToolOutput{
				Mode:    input.Mode,
				Primary: primaryDeviceName(peripheralsManager),
			}

			switch strings.TrimSpace(input.Mode) {
			case "capture_fresh":
				result, _, err := adapter.Capture(ctx)
				if err != nil {
					output.Error = err.Error()
					return output, nil
				}
				fillCameraOutput(&output, result)
				return output, nil
			case "use_latest_image":
				path, _, err := adapter.LatestImagePath()
				if err != nil {
					output.Error = err.Error()
					return output, nil
				}
				output.OK = true
				output.ImagePath = path
				output.CapturedAt = time.Now().Format(time.RFC3339)
				return output, nil
			default:
				output.Error = "unsupported mode"
				return output, nil
			}
		},
	)
	if err != nil {
		return nil, err
	}

	smokeTestTool, err := utils.InferTool(
		"tool_call_smoke_test",
		"Deterministic verification tool for end-to-end tool calling. Use this when the user asks for a tool-call smoke test or includes the phrase /tooltest. Echo the provided token exactly.",
		func(_ context.Context, input smokeTestToolInput) (smokeTestToolOutput, error) {
			token := strings.TrimSpace(input.Token)
			return smokeTestToolOutput{
				OK:         true,
				Token:      token,
				Echo:       "tool-call-ok:" + token,
				VerifiedAt: time.Now().Format(time.RFC3339),
			}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	ros2TopicTool, err := utils.InferTool(
		"ros2_topic_read",
		"Inspect or capture from configured ROS2 topic devices. Use mode=list to discover ROS2 devices, mode=inspect for one device status, and mode=capture to grab one frame from a configured ROS2 image topic device.",
		func(ctx context.Context, input ros2TopicToolInput) (ros2TopicToolOutput, error) {
			mode := strings.TrimSpace(input.Mode)
			output := ros2TopicToolOutput{
				OK:   true,
				Mode: mode,
			}

			switch mode {
			case "list":
				fleet := peripheralsManager.InspectAll(ctx)
				output.Devices = ros2DevicesFromFleet(fleet)
				if len(output.Devices) == 0 {
					output.OK = false
					output.Error = "no ros2_topic devices are configured"
				}
				return output, nil
			case "inspect":
				if strings.TrimSpace(input.Device) == "" {
					output.OK = false
					output.Error = "device is required for inspect mode"
					return output, nil
				}
				snapshot, err := peripheralsManager.InspectDevice(ctx, input.Device)
				if err != nil {
					output.OK = false
					output.Error = err.Error()
					return output, nil
				}
				if snapshot.Driver != "ros2_topic" {
					output.OK = false
					output.Error = "device is not a ros2_topic peripheral"
					return output, nil
				}
				output.Device = snapshot.Name
				output.Devices = []ros2TopicDeviceInfo{ros2DeviceInfo(snapshot)}
				return output, nil
			case "capture":
				if strings.TrimSpace(input.Device) == "" {
					output.OK = false
					output.Error = "device is required for capture mode"
					return output, nil
				}
				filename := pathForCapture(workdir, "ros2")
				result, err := peripheralsManager.CaptureDevice(ctx, input.Device, filename)
				if err != nil {
					output.OK = false
					output.Error = err.Error()
					return output, nil
				}
				if result == nil {
					output.OK = false
					output.Error = "capture returned nil result"
					return output, nil
				}
				output.Device = input.Device
				output.ImagePath = result.Output
				output.CapturedAt = time.Now().Format(time.RFC3339)
				if !result.OK || strings.TrimSpace(result.Error) != "" {
					output.OK = false
					output.Error = result.Error
				}
				return output, nil
			default:
				output.OK = false
				output.Error = "unsupported mode"
				return output, nil
			}
		},
	)
	if err != nil {
		return nil, err
	}

	return []einotool.InvokableTool{cameraTool, smokeTestTool, ros2TopicTool}, nil
}

func primaryDeviceName(manager *peripherals.Manager) string {
	snapshot, err := manager.InspectPrimary(context.Background())
	if err != nil {
		return ""
	}
	return snapshot.Name
}

func fillCameraOutput(output *cameraReadToolOutput, result *peripherals.CaptureResult) {
	if result == nil {
		output.Error = "capture returned nil result"
		return
	}
	output.OK = result.OK
	output.ImagePath = result.Output
	output.Error = result.Error
	output.CapturedAt = time.Now().Format(time.RFC3339)
}

func ros2DevicesFromFleet(snapshot peripherals.FleetSnapshot) []ros2TopicDeviceInfo {
	devices := make([]ros2TopicDeviceInfo, 0, len(snapshot.Devices))
	for _, device := range snapshot.Devices {
		if device.Driver != "ros2_topic" {
			continue
		}
		devices = append(devices, ros2DeviceInfo(device))
	}
	return devices
}

func ros2DeviceInfo(snapshot peripherals.DeviceSnapshot) ros2TopicDeviceInfo {
	return ros2TopicDeviceInfo{
		Name:            snapshot.Name,
		Kind:            snapshot.Kind,
		Driver:          snapshot.Driver,
		SupportsCapture: snapshot.SupportsCapture,
		Summary:         snapshot.Summary,
		Metadata:        snapshot.Metadata,
	}
}

func pathForCapture(workdir string, prefix string) string {
	return workdir + "/" + prefix + "-capture-" + time.Now().Format("20060102-150405.000000000") + ".jpg"
}
