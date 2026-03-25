package peripherals

import (
	"context"

	"eino-vlm-agent-demo/internal/camera"
)

type CaptureResult = camera.CaptureResult

type DeviceDescriptor struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Driver          string `json:"driver"`
	SupportsCapture bool   `json:"supports_capture"`
}

type CheckResult struct {
	Name   string `json:"name"`
	Output string `json:"output"`
}

type DeviceSnapshot struct {
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Driver          string            `json:"driver"`
	SupportsCapture bool              `json:"supports_capture"`
	Summary         string            `json:"summary"`
	Checks          []CheckResult     `json:"checks,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type FleetSnapshot struct {
	PrimaryCaptureDevice string           `json:"primary_capture_device"`
	Devices              []DeviceSnapshot `json:"devices"`
}

type Device interface {
	Descriptor() DeviceDescriptor
	Inspect(ctx context.Context) DeviceSnapshot
}

type CaptureDevice interface {
	Capture(ctx context.Context, outputPath string) (*CaptureResult, error)
}
