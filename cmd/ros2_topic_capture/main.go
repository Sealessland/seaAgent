//go:build ros2_rclgo

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"eino-vlm-agent-demo/internal/camera"
	sensor_msgs_msg "eino-vlm-agent-demo/msgs/sensor_msgs/msg"

	"github.com/tiiuae/rclgo/pkg/rclgo"
)

type captureConfig struct {
	OutputPath     string
	Topic          string
	MessageType    string
	Encoding       string
	TimeoutSeconds int
	Probe          bool
}

var captureNodeSeq uint64

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		emit(camera.CaptureResult{OK: false, Error: err.Error()})
		return
	}
	if cfg.Probe {
		fmt.Println("rclgo_ready")
		return
	}

	result, err := captureOnce(cfg)
	if err != nil {
		result.OK = false
		if strings.TrimSpace(result.Error) == "" {
			result.Error = err.Error()
		}
	}
	emit(result)
}

func parseFlags(args []string) (captureConfig, error) {
	var cfg captureConfig
	fs := flag.NewFlagSet("ros2_topic_capture", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.OutputPath, "output", "", "output image path")
	fs.StringVar(&cfg.Topic, "topic", "", "ROS2 image topic")
	fs.StringVar(&cfg.MessageType, "message-type", "", "ROS2 message type")
	fs.StringVar(&cfg.Encoding, "encoding", "", "preferred decoding encoding")
	fs.IntVar(&cfg.TimeoutSeconds, "timeout-seconds", 5, "timeout while waiting for one frame")
	fs.BoolVar(&cfg.Probe, "probe", false, "print helper readiness and exit")
	if err := fs.Parse(args); err != nil {
		return captureConfig{}, err
	}
	if cfg.Probe {
		return cfg, nil
	}
	switch {
	case strings.TrimSpace(cfg.OutputPath) == "":
		return captureConfig{}, fmt.Errorf("--output is required")
	case strings.TrimSpace(cfg.Topic) == "":
		return captureConfig{}, fmt.Errorf("--topic is required")
	case strings.TrimSpace(cfg.MessageType) == "":
		return captureConfig{}, fmt.Errorf("--message-type is required")
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 5
	}
	return cfg, nil
}

func captureOnce(cfg captureConfig) (camera.CaptureResult, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.OutputPath), 0o755); err != nil {
		return camera.CaptureResult{}, err
	}

	ctx, err := rclgo.NewContextWithOpts(nil, nil)
	if err != nil {
		return camera.CaptureResult{}, err
	}
	defer ctx.Close()

	node, err := ctx.NewNode(nextNodeName(), "")
	if err != nil {
		return camera.CaptureResult{}, err
	}
	defer node.Close()

	subOpts := rclgo.NewDefaultSubscriptionOptions()
	subOpts.Qos.Reliability = rclgo.ReliabilityBestEffort
	subOpts.Qos.Depth = 1

	spinCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan camera.CaptureResult, 1)
	errCh := make(chan error, 2)
	var finish sync.Once

	finishResult := func(result camera.CaptureResult) {
		finish.Do(func() {
			resultCh <- result
			cancel()
		})
	}
	finishError := func(err error) {
		finish.Do(func() {
			errCh <- err
			cancel()
		})
	}

	switch normalizeEncoding(cfg.MessageType) {
	case "sensor_msgs/msg/image", "sensor_msgs/image":
		_, err = node.NewSubscription(cfg.Topic, sensor_msgs_msg.ImageTypeSupport, subOpts, func(sub *rclgo.Subscription) {
			msg := sensor_msgs_msg.NewImage()
			if _, takeErr := sub.TakeMessage(msg); takeErr != nil {
				finishError(takeErr)
				return
			}
			result, decodeErr := saveRawImage(cfg, msg)
			if decodeErr != nil {
				finishError(decodeErr)
				return
			}
			finishResult(result)
		})
	case "sensor_msgs/msg/compressedimage", "sensor_msgs/compressedimage":
		_, err = node.NewSubscription(cfg.Topic, sensor_msgs_msg.CompressedImageTypeSupport, subOpts, func(sub *rclgo.Subscription) {
			msg := sensor_msgs_msg.NewCompressedImage()
			if _, takeErr := sub.TakeMessage(msg); takeErr != nil {
				finishError(takeErr)
				return
			}
			result, decodeErr := saveCompressedImage(cfg, msg)
			if decodeErr != nil {
				finishError(decodeErr)
				return
			}
			finishResult(result)
		})
	default:
		return camera.CaptureResult{}, fmt.Errorf("unsupported message type: %s", cfg.MessageType)
	}
	if err != nil {
		return camera.CaptureResult{}, err
	}

	go func() {
		if spinErr := node.Spin(spinCtx); spinErr != nil && !errors.Is(spinErr, context.Canceled) {
			finishError(spinErr)
		}
	}()

	timeout := time.NewTimer(time.Duration(cfg.TimeoutSeconds) * time.Second)
	defer timeout.Stop()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return camera.CaptureResult{}, err
	case <-timeout.C:
		return camera.CaptureResult{}, fmt.Errorf("timeout waiting for topic %s", cfg.Topic)
	}
}

func saveRawImage(cfg captureConfig, msg *sensor_msgs_msg.Image) (camera.CaptureResult, error) {
	img, err := imageFromROSMessage(msg, cfg.Encoding)
	if err != nil {
		return camera.CaptureResult{}, err
	}
	width, height, err := writeImage(cfg.OutputPath, img)
	if err != nil {
		return camera.CaptureResult{}, err
	}
	return camera.CaptureResult{
		OK:       true,
		Output:   cfg.OutputPath,
		Width:    width,
		Height:   height,
		CameraSN: cfg.Topic,
	}, nil
}

func saveCompressedImage(cfg captureConfig, msg *sensor_msgs_msg.CompressedImage) (camera.CaptureResult, error) {
	if len(msg.Data) == 0 {
		return camera.CaptureResult{}, fmt.Errorf("compressed image payload is empty")
	}
	img, _, err := image.Decode(bytes.NewReader(msg.Data))
	if err != nil {
		return camera.CaptureResult{}, err
	}
	width, height, err := writeImage(cfg.OutputPath, img)
	if err != nil {
		return camera.CaptureResult{}, err
	}
	return camera.CaptureResult{
		OK:       true,
		Output:   cfg.OutputPath,
		Width:    width,
		Height:   height,
		CameraSN: cfg.Topic,
	}, nil
}

func imageFromROSMessage(msg *sensor_msgs_msg.Image, preferredEncoding string) (image.Image, error) {
	if msg == nil {
		return nil, fmt.Errorf("image message is nil")
	}
	if msg.Width == 0 || msg.Height == 0 {
		return nil, fmt.Errorf("image dimensions are empty")
	}

	encoding := normalizeEncoding(msg.Encoding)
	if encoding == "" || encoding == "8uc3" || encoding == "8uc4" || encoding == "16uc1" {
		if preferred := normalizeEncoding(preferredEncoding); preferred != "" {
			encoding = preferred
		}
	}
	if encoding == "" {
		return nil, fmt.Errorf("image encoding is empty")
	}

	switch encoding {
	case "rgb8":
		return rgbLikeToNRGBA(msg, 3, false)
	case "bgr8":
		return rgbLikeToNRGBA(msg, 3, true)
	case "rgba8":
		return rgbaLikeToNRGBA(msg, false)
	case "bgra8":
		return rgbaLikeToNRGBA(msg, true)
	case "mono8", "8uc1":
		return mono8ToGray(msg)
	case "mono16", "16uc1":
		return mono16ToGray(msg)
	default:
		return nil, fmt.Errorf("unsupported image encoding: %s", encoding)
	}
}

func rgbLikeToNRGBA(msg *sensor_msgs_msg.Image, channels int, bgr bool) (image.Image, error) {
	if err := validateStep(msg, channels); err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(msg.Width), int(msg.Height)))
	for y := 0; y < int(msg.Height); y++ {
		row := msg.Data[y*int(msg.Step) : y*int(msg.Step)+int(msg.Width)*channels]
		for x := 0; x < int(msg.Width); x++ {
			base := x * channels
			r := row[base]
			g := row[base+1]
			b := row[base+2]
			if bgr {
				r, b = b, r
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 0xff})
		}
	}
	return img, nil
}

func rgbaLikeToNRGBA(msg *sensor_msgs_msg.Image, bgra bool) (image.Image, error) {
	if err := validateStep(msg, 4); err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(msg.Width), int(msg.Height)))
	for y := 0; y < int(msg.Height); y++ {
		row := msg.Data[y*int(msg.Step) : y*int(msg.Step)+int(msg.Width)*4]
		for x := 0; x < int(msg.Width); x++ {
			base := x * 4
			r := row[base]
			g := row[base+1]
			b := row[base+2]
			a := row[base+3]
			if bgra {
				r, b = b, r
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return img, nil
}

func mono8ToGray(msg *sensor_msgs_msg.Image) (image.Image, error) {
	if err := validateStep(msg, 1); err != nil {
		return nil, err
	}
	img := image.NewGray(image.Rect(0, 0, int(msg.Width), int(msg.Height)))
	for y := 0; y < int(msg.Height); y++ {
		row := msg.Data[y*int(msg.Step) : y*int(msg.Step)+int(msg.Width)]
		copy(img.Pix[y*img.Stride:y*img.Stride+int(msg.Width)], row)
	}
	return img, nil
}

func mono16ToGray(msg *sensor_msgs_msg.Image) (image.Image, error) {
	if err := validateStep(msg, 2); err != nil {
		return nil, err
	}
	img := image.NewGray(image.Rect(0, 0, int(msg.Width), int(msg.Height)))
	for y := 0; y < int(msg.Height); y++ {
		row := msg.Data[y*int(msg.Step) : y*int(msg.Step)+int(msg.Width)*2]
		for x := 0; x < int(msg.Width); x++ {
			base := x * 2
			var value uint16
			if msg.IsBigendian != 0 {
				value = binary.BigEndian.Uint16(row[base : base+2])
			} else {
				value = binary.LittleEndian.Uint16(row[base : base+2])
			}
			img.SetGray(x, y, color.Gray{Y: uint8(value >> 8)})
		}
	}
	return img, nil
}

func validateStep(msg *sensor_msgs_msg.Image, pixelSize int) error {
	requiredRow := int(msg.Width) * pixelSize
	if int(msg.Step) < requiredRow {
		return fmt.Errorf("image step %d is smaller than required row size %d", msg.Step, requiredRow)
	}
	requiredTotal := int(msg.Step) * int(msg.Height)
	if len(msg.Data) < requiredTotal {
		return fmt.Errorf("image payload is shorter than expected: %d < %d", len(msg.Data), requiredTotal)
	}
	return nil
}

func writeImage(path string, img image.Image) (int, int, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, 0, err
	}
	file, err := os.Create(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		err = png.Encode(file, img)
	default:
		err = jpeg.Encode(file, img, &jpeg.Options{Quality: 92})
	}
	if err != nil {
		return 0, 0, err
	}

	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}

func nextNodeName() string {
	seq := atomic.AddUint64(&captureNodeSeq, 1)
	return fmt.Sprintf("jetson_ros2_topic_capture_%d", seq)
}

func normalizeEncoding(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "::", "/")
	value = strings.ReplaceAll(value, ".msg.", "/msg/")
	return value
}

func emit(result camera.CaptureResult) {
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
