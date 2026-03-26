package peripherals

import (
	"encoding/json"
	"fmt"
	"os"
)

type FleetConfig struct {
	PrimaryCaptureDevice string         `json:"primary_capture_device"`
	Devices              []DeviceConfig `json:"devices"`
}

type DeviceConfig struct {
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	Driver   string            `json:"driver"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Capture  *CaptureConfig    `json:"capture,omitempty"`
	Checks   []CheckConfig     `json:"checks,omitempty"`
}

type CaptureConfig struct {
	Binary         string   `json:"binary,omitempty"`
	Script         string   `json:"script,omitempty"`
	Command        []string `json:"command,omitempty"`
	Topic          string   `json:"topic,omitempty"`
	MessageType    string   `json:"message_type,omitempty"`
	Encoding       string   `json:"encoding,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	ROSSetup       []string `json:"ros_setup,omitempty"`
}

type CheckConfig struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

func LoadConfig(path string) (FleetConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FleetConfig{}, err
	}

	var cfg FleetConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FleetConfig{}, fmt.Errorf("decode peripheral config: %w", err)
	}

	if len(cfg.Devices) == 0 {
		return FleetConfig{}, fmt.Errorf("peripheral config has no devices")
	}

	return cfg, nil
}
