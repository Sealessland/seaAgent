//go:build ros2_rclgo

package std_msgs_msg

/*
#cgo LDFLAGS: "-L/usr/lib" "-Wl,-rpath=/usr/lib"
#cgo CFLAGS: "-I/usr/include/builtin_interfaces"
#cgo CFLAGS: "-I/usr/include/std_msgs"
#cgo CFLAGS: "-I/usr/include/rosidl_runtime_c"
#cgo LDFLAGS: "-L/opt/ros/humble/lib" "-Wl,-rpath=/opt/ros/humble/lib"
#cgo CFLAGS: "-I/opt/ros/humble/include/builtin_interfaces"
#cgo CFLAGS: "-I/opt/ros/humble/include/std_msgs"
#cgo CFLAGS: "-I/opt/ros/humble/include/rosidl_runtime_c"
#cgo LDFLAGS: -lbuiltin_interfaces__rosidl_generator_c -lbuiltin_interfaces__rosidl_typesupport_c
#cgo LDFLAGS: -lstd_msgs__rosidl_generator_c -lstd_msgs__rosidl_typesupport_c

#include <rosidl_runtime_c/message_type_support_struct.h>
#include <std_msgs/msg/header.h>
*/
import "C"

import (
	"unsafe"

	builtin_interfaces_msg "eino-vlm-agent-demo/msgs/builtin_interfaces/msg"

	primitives "github.com/tiiuae/rclgo/pkg/rclgo/primitives"
	"github.com/tiiuae/rclgo/pkg/rclgo/types"
)

type Header struct {
	Stamp   builtin_interfaces_msg.Time `yaml:"stamp"`
	FrameId string                      `yaml:"frame_id"`
}

func NewHeader() *Header {
	msg := &Header{}
	msg.SetDefaults()
	return msg
}

func (h *Header) Clone() *Header {
	return &Header{
		Stamp:   *h.Stamp.Clone(),
		FrameId: h.FrameId,
	}
}

func (h *Header) CloneMsg() types.Message {
	return h.Clone()
}

func (h *Header) SetDefaults() {
	h.Stamp.SetDefaults()
	h.FrameId = ""
}

func (h *Header) GetTypeSupport() types.MessageTypeSupport {
	return HeaderTypeSupport
}

var HeaderTypeSupport types.MessageTypeSupport = headerTypeSupport{}

type headerTypeSupport struct{}

func (headerTypeSupport) New() types.Message {
	return NewHeader()
}

func (headerTypeSupport) PrepareMemory() unsafe.Pointer {
	return unsafe.Pointer(C.std_msgs__msg__Header__create())
}

func (headerTypeSupport) ReleaseMemory(pointer unsafe.Pointer) {
	C.std_msgs__msg__Header__destroy((*C.std_msgs__msg__Header)(pointer))
}

func (headerTypeSupport) AsCStruct(dst unsafe.Pointer, msg types.Message) {
	value := msg.(*Header)
	mem := (*C.std_msgs__msg__Header)(dst)
	builtin_interfaces_msg.TimeTypeSupport.AsCStruct(unsafe.Pointer(&mem.stamp), &value.Stamp)
	primitives.StringAsCStruct(unsafe.Pointer(&mem.frame_id), value.FrameId)
}

func (headerTypeSupport) AsGoStruct(dst types.Message, src unsafe.Pointer) {
	value := dst.(*Header)
	mem := (*C.std_msgs__msg__Header)(src)
	builtin_interfaces_msg.TimeTypeSupport.AsGoStruct(&value.Stamp, unsafe.Pointer(&mem.stamp))
	primitives.StringAsGoStruct(&value.FrameId, unsafe.Pointer(&mem.frame_id))
}

func (headerTypeSupport) TypeSupport() unsafe.Pointer {
	return unsafe.Pointer(C.rosidl_typesupport_c__get_message_type_support_handle__std_msgs__msg__Header())
}
