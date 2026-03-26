package observation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"eino-vlm-agent-demo/internal/peripherals"
)

type cameraReadTool struct {
	workdir     string
	peripherals *peripherals.Manager
}

func newCameraReadTool(workdir string, peripheralsManager *peripherals.Manager) *cameraReadTool {
	return &cameraReadTool{
		workdir:     workdir,
		peripherals: peripheralsManager,
	}
}

func (t *cameraReadTool) Capture(ctx context.Context) (*peripherals.CaptureResult, ToolCallRecord, error) {
	filename := filepath.Join(t.workdir, "capture-"+time.Now().Format("20060102-150405.000000000")+".jpg")
	result, err := t.peripherals.CapturePrimary(ctx, filename)
	record := ToolCallRecord{
		Name: "camera_read",
		Input: map[string]string{
			"mode":        "capture_fresh",
			"output_path": filename,
		},
	}
	if result != nil {
		record.Output = map[string]string{
			"ok":     boolString(result.OK),
			"output": result.Output,
			"error":  result.Error,
		}
	}
	return result, record, err
}

func (t *cameraReadTool) LatestImagePath() (string, ToolCallRecord, error) {
	path, err := latestCapturePath(t.workdir)
	record := ToolCallRecord{
		Name: "camera_read",
		Input: map[string]string{
			"mode": "use_latest_image",
		},
		Output: map[string]string{
			"output": path,
		},
	}
	if err != nil {
		record.Output["error"] = err.Error()
	}
	return path, record, err
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func latestCapturePath(workdir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(workdir, "capture-*.jpg"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no captured image is available yet")
	}

	sort.Slice(matches, func(i int, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] > matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})

	return matches[0], nil
}
