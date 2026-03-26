//go:build ros2_rclgo

package sensor_msgs_msg

/*
#cgo LDFLAGS: "-L/usr/lib" "-Wl,-rpath=/usr/lib"
#cgo CFLAGS: "-I/usr/include/builtin_interfaces"
#cgo CFLAGS: "-I/usr/include/std_msgs"
#cgo CFLAGS: "-I/usr/include/sensor_msgs"
#cgo CFLAGS: "-I/usr/include/rosidl_runtime_c"
#cgo LDFLAGS: "-L/opt/ros/humble/lib" "-Wl,-rpath=/opt/ros/humble/lib"
#cgo CFLAGS: "-I/opt/ros/humble/include/builtin_interfaces"
#cgo CFLAGS: "-I/opt/ros/humble/include/std_msgs"
#cgo CFLAGS: "-I/opt/ros/humble/include/sensor_msgs"
#cgo CFLAGS: "-I/opt/ros/humble/include/rosidl_runtime_c"
#cgo LDFLAGS: -lbuiltin_interfaces__rosidl_generator_c -lbuiltin_interfaces__rosidl_typesupport_c
#cgo LDFLAGS: -lstd_msgs__rosidl_generator_c -lstd_msgs__rosidl_typesupport_c
#cgo LDFLAGS: -lsensor_msgs__rosidl_generator_c -lsensor_msgs__rosidl_typesupport_c

#include <rosidl_runtime_c/message_type_support_struct.h>
#include <sensor_msgs/msg/image.h>
*/
import "C"

import (
	"unsafe"

	std_msgs_msg "eino-vlm-agent-demo/msgs/std_msgs/msg"

	primitives "github.com/tiiuae/rclgo/pkg/rclgo/primitives"
	"github.com/tiiuae/rclgo/pkg/rclgo/types"
)

type Image struct {
	Header      std_msgs_msg.Header `yaml:"header"`
	Height      uint32              `yaml:"height"`
	Width       uint32              `yaml:"width"`
	Encoding    string              `yaml:"encoding"`
	IsBigendian uint8               `yaml:"is_bigendian"`
	Step        uint32              `yaml:"step"`
	Data        []uint8             `yaml:"data"`
}

func NewImage() *Image {
	msg := &Image{}
	msg.SetDefaults()
	return msg
}

func (m *Image) Clone() *Image {
	clone := &Image{
		Header:      *m.Header.Clone(),
		Height:      m.Height,
		Width:       m.Width,
		Encoding:    m.Encoding,
		IsBigendian: m.IsBigendian,
		Step:        m.Step,
	}
	if m.Data != nil {
		clone.Data = append([]uint8(nil), m.Data...)
	}
	return clone
}

func (m *Image) CloneMsg() types.Message {
	return m.Clone()
}

func (m *Image) SetDefaults() {
	m.Header.SetDefaults()
	m.Height = 0
	m.Width = 0
	m.Encoding = ""
	m.IsBigendian = 0
	m.Step = 0
	m.Data = nil
}

func (m *Image) GetTypeSupport() types.MessageTypeSupport {
	return ImageTypeSupport
}

var ImageTypeSupport types.MessageTypeSupport = imageTypeSupport{}

type imageTypeSupport struct{}

func (imageTypeSupport) New() types.Message {
	return NewImage()
}

func (imageTypeSupport) PrepareMemory() unsafe.Pointer {
	return unsafe.Pointer(C.sensor_msgs__msg__Image__create())
}

func (imageTypeSupport) ReleaseMemory(pointer unsafe.Pointer) {
	C.sensor_msgs__msg__Image__destroy((*C.sensor_msgs__msg__Image)(pointer))
}

func (imageTypeSupport) AsCStruct(dst unsafe.Pointer, msg types.Message) {
	value := msg.(*Image)
	mem := (*C.sensor_msgs__msg__Image)(dst)
	std_msgs_msg.HeaderTypeSupport.AsCStruct(unsafe.Pointer(&mem.header), &value.Header)
	mem.height = C.uint32_t(value.Height)
	mem.width = C.uint32_t(value.Width)
	primitives.StringAsCStruct(unsafe.Pointer(&mem.encoding), value.Encoding)
	mem.is_bigendian = C.uint8_t(value.IsBigendian)
	mem.step = C.uint32_t(value.Step)
	primitives.Uint8__Sequence_to_C((*primitives.CUint8__Sequence)(unsafe.Pointer(&mem.data)), value.Data)
}

func (imageTypeSupport) AsGoStruct(dst types.Message, src unsafe.Pointer) {
	value := dst.(*Image)
	mem := (*C.sensor_msgs__msg__Image)(src)
	std_msgs_msg.HeaderTypeSupport.AsGoStruct(&value.Header, unsafe.Pointer(&mem.header))
	value.Height = uint32(mem.height)
	value.Width = uint32(mem.width)
	primitives.StringAsGoStruct(&value.Encoding, unsafe.Pointer(&mem.encoding))
	value.IsBigendian = uint8(mem.is_bigendian)
	value.Step = uint32(mem.step)
	primitives.Uint8__Sequence_to_Go(&value.Data, *(*primitives.CUint8__Sequence)(unsafe.Pointer(&mem.data)))
}

func (imageTypeSupport) TypeSupport() unsafe.Pointer {
	return unsafe.Pointer(C.rosidl_typesupport_c__get_message_type_support_handle__sensor_msgs__msg__Image())
}
