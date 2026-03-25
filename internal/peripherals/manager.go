package peripherals

import (
	"context"
	"fmt"
)

type Manager struct {
	primaryCapture string
	devices        map[string]Device
	ordered        []Device
}

func NewManager(cfg FleetConfig) (*Manager, error) {
	manager := &Manager{
		primaryCapture: cfg.PrimaryCaptureDevice,
		devices:        make(map[string]Device, len(cfg.Devices)),
		ordered:        make([]Device, 0, len(cfg.Devices)),
	}

	for _, deviceCfg := range cfg.Devices {
		device, err := newDevice(deviceCfg)
		if err != nil {
			return nil, fmt.Errorf("init device %s: %w", deviceCfg.Name, err)
		}
		if _, exists := manager.devices[deviceCfg.Name]; exists {
			return nil, fmt.Errorf("duplicate device name %q", deviceCfg.Name)
		}
		manager.devices[deviceCfg.Name] = device
		manager.ordered = append(manager.ordered, device)
	}

	if manager.primaryCapture == "" {
		for _, device := range manager.ordered {
			if _, ok := device.(CaptureDevice); ok {
				manager.primaryCapture = device.Descriptor().Name
				break
			}
		}
	}

	if manager.primaryCapture != "" {
		device, ok := manager.devices[manager.primaryCapture]
		if !ok {
			return nil, fmt.Errorf("primary capture device %q not found", manager.primaryCapture)
		}
		if _, ok := device.(CaptureDevice); !ok {
			return nil, fmt.Errorf("primary capture device %q does not support capture", manager.primaryCapture)
		}
	}

	return manager, nil
}

func (m *Manager) InspectAll(ctx context.Context) FleetSnapshot {
	snapshot := FleetSnapshot{
		PrimaryCaptureDevice: m.primaryCapture,
		Devices:              make([]DeviceSnapshot, 0, len(m.ordered)),
	}
	for _, device := range m.ordered {
		snapshot.Devices = append(snapshot.Devices, device.Inspect(ctx))
	}
	return snapshot
}

func (m *Manager) InspectPrimary(ctx context.Context) (DeviceSnapshot, error) {
	device, err := m.primaryDevice()
	if err != nil {
		return DeviceSnapshot{}, err
	}
	return device.Inspect(ctx), nil
}

func (m *Manager) CapturePrimary(ctx context.Context, outputPath string) (*CaptureResult, error) {
	device, err := m.primaryDevice()
	if err != nil {
		return nil, err
	}

	captureDevice, ok := device.(CaptureDevice)
	if !ok {
		return nil, fmt.Errorf("primary device %q does not support capture", device.Descriptor().Name)
	}
	return captureDevice.Capture(ctx, outputPath)
}

func (m *Manager) primaryDevice() (Device, error) {
	if m.primaryCapture == "" {
		return nil, fmt.Errorf("no primary capture device configured")
	}

	device, ok := m.devices[m.primaryCapture]
	if !ok {
		return nil, fmt.Errorf("primary capture device %q is unavailable", m.primaryCapture)
	}
	return device, nil
}

func newDevice(cfg DeviceConfig) (Device, error) {
	switch cfg.Driver {
	case "zed":
		return newZEDDevice(cfg)
	default:
		return newExecDevice(cfg)
	}
}
