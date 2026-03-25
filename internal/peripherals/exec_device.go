package peripherals

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type execDevice struct {
	cfg DeviceConfig
}

func newExecDevice(cfg DeviceConfig) (Device, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(cfg.Kind) == "" {
		return nil, fmt.Errorf("device kind is required")
	}
	if strings.TrimSpace(cfg.Driver) == "" {
		return nil, fmt.Errorf("device driver is required")
	}
	return &execDevice{cfg: cfg}, nil
}

func (d *execDevice) Descriptor() DeviceDescriptor {
	return DeviceDescriptor{
		Name:            d.cfg.Name,
		Kind:            d.cfg.Kind,
		Driver:          d.cfg.Driver,
		SupportsCapture: d.cfg.Capture != nil,
	}
}

func (d *execDevice) Inspect(ctx context.Context) DeviceSnapshot {
	checks := make([]CheckResult, 0, len(d.cfg.Checks))
	for _, check := range d.cfg.Checks {
		checks = append(checks, CheckResult{
			Name:   check.Name,
			Output: runCommand(ctx, check.Command),
		})
	}

	summary := "device inspection completed"
	if len(checks) == 0 {
		summary = "no checks configured for this device"
	}

	return DeviceSnapshot{
		Name:            d.cfg.Name,
		Kind:            d.cfg.Kind,
		Driver:          d.cfg.Driver,
		SupportsCapture: d.cfg.Capture != nil,
		Summary:         summary,
		Checks:          checks,
		Metadata:        d.cfg.Metadata,
	}
}

func (d *execDevice) Capture(ctx context.Context, outputPath string) (*CaptureResult, error) {
	if d.cfg.Capture == nil {
		return nil, fmt.Errorf("device %q does not support capture", d.cfg.Name)
	}
	if len(d.cfg.Capture.Command) == 0 {
		return nil, fmt.Errorf("device %q capture command is not configured", d.cfg.Name)
	}

	command := expandOutputPlaceholder(d.cfg.Capture.Command, outputPath)
	text, err := runCommandOutput(ctx, command)
	result := &CaptureResult{RawOutput: text}
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.OK = true
	result.Output = outputPath
	return result, nil
}

func expandOutputPlaceholder(command []string, outputPath string) []string {
	expanded := make([]string, 0, len(command))
	for _, part := range command {
		expanded = append(expanded, strings.ReplaceAll(part, "{{output}}", outputPath))
	}
	return expanded
}

func runCommand(ctx context.Context, command []string) string {
	text, err := runCommandOutput(ctx, command)
	if err != nil && text == "" {
		return fmt.Sprintf("command failed: %v", err)
	}
	if strings.TrimSpace(text) == "" {
		return "(empty)"
	}
	return text
}

func runCommandOutput(parent context.Context, command []string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("empty command")
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
