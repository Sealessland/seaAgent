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
#include <sensor_msgs/msg/compressed_image.h>
*/
import "C"

import (
	"unsafe"

	std_msgs_msg "eino-vlm-agent-demo/msgs/std_msgs/msg"

	primitives "github.com/tiiuae/rclgo/pkg/rclgo/primitives"
	"github.com/tiiuae/rclgo/pkg/rclgo/types"
)

type CompressedImage struct {
	Header std_msgs_msg.Header `yaml:"header"`
	Format string              `yaml:"format"`
	Data   []uint8             `yaml:"data"`
}

func NewCompressedImage() *CompressedImage {
	msg := &CompressedImage{}
	msg.SetDefaults()
	return msg
}

func (m *CompressedImage) Clone() *CompressedImage {
	clone := &CompressedImage{
		Header: *m.Header.Clone(),
		Format: m.Format,
	}
	if m.Data != nil {
		clone.Data = append([]uint8(nil), m.Data...)
	}
	return clone
}

func (m *CompressedImage) CloneMsg() types.Message {
	return m.Clone()
}

func (m *CompressedImage) SetDefaults() {
	m.Header.SetDefaults()
	m.Format = ""
	m.Data = nil
}

func (m *CompressedImage) GetTypeSupport() types.MessageTypeSupport {
	return CompressedImageTypeSupport
}

var CompressedImageTypeSupport types.MessageTypeSupport = compressedImageTypeSupport{}

type compressedImageTypeSupport struct{}

func (compressedImageTypeSupport) New() types.Message {
	return NewCompressedImage()
}

func (compressedImageTypeSupport) PrepareMemory() unsafe.Pointer {
	return unsafe.Pointer(C.sensor_msgs__msg__CompressedImage__create())
}

func (compressedImageTypeSupport) ReleaseMemory(pointer unsafe.Pointer) {
	C.sensor_msgs__msg__CompressedImage__destroy((*C.sensor_msgs__msg__CompressedImage)(pointer))
}

func (compressedImageTypeSupport) AsCStruct(dst unsafe.Pointer, msg types.Message) {
	value := msg.(*CompressedImage)
	mem := (*C.sensor_msgs__msg__CompressedImage)(dst)
	std_msgs_msg.HeaderTypeSupport.AsCStruct(unsafe.Pointer(&mem.header), &value.Header)
	primitives.StringAsCStruct(unsafe.Pointer(&mem.format), value.Format)
	primitives.Uint8__Sequence_to_C((*primitives.CUint8__Sequence)(unsafe.Pointer(&mem.data)), value.Data)
}

func (compressedImageTypeSupport) AsGoStruct(dst types.Message, src unsafe.Pointer) {
	value := dst.(*CompressedImage)
	mem := (*C.sensor_msgs__msg__CompressedImage)(src)
	std_msgs_msg.HeaderTypeSupport.AsGoStruct(&value.Header, unsafe.Pointer(&mem.header))
	primitives.StringAsGoStruct(&value.Format, unsafe.Pointer(&mem.format))
	primitives.Uint8__Sequence_to_Go(&value.Data, *(*primitives.CUint8__Sequence)(unsafe.Pointer(&mem.data)))
}

func (compressedImageTypeSupport) TypeSupport() unsafe.Pointer {
	return unsafe.Pointer(C.rosidl_typesupport_c__get_message_type_support_handle__sensor_msgs__msg__CompressedImage())
}
