//go:build ros2_rclgo

package builtin_interfaces_msg

/*
#cgo LDFLAGS: "-L/usr/lib" "-Wl,-rpath=/usr/lib"
#cgo CFLAGS: "-I/usr/include/builtin_interfaces"
#cgo LDFLAGS: "-L/opt/ros/humble/lib" "-Wl,-rpath=/opt/ros/humble/lib"
#cgo CFLAGS: "-I/opt/ros/humble/include/builtin_interfaces"
#cgo LDFLAGS: -lbuiltin_interfaces__rosidl_generator_c -lbuiltin_interfaces__rosidl_typesupport_c

#include <rosidl_runtime_c/message_type_support_struct.h>
#include <builtin_interfaces/msg/time.h>
*/
import "C"

import (
	"unsafe"

	"github.com/tiiuae/rclgo/pkg/rclgo/types"
)

type Time struct {
	Sec     int32  `yaml:"sec"`
	Nanosec uint32 `yaml:"nanosec"`
}

func NewTime() *Time {
	msg := &Time{}
	msg.SetDefaults()
	return msg
}

func (t *Time) Clone() *Time {
	return &Time{
		Sec:     t.Sec,
		Nanosec: t.Nanosec,
	}
}

func (t *Time) CloneMsg() types.Message {
	return t.Clone()
}

func (t *Time) SetDefaults() {
	t.Sec = 0
	t.Nanosec = 0
}

func (t *Time) GetTypeSupport() types.MessageTypeSupport {
	return TimeTypeSupport
}

var TimeTypeSupport types.MessageTypeSupport = timeTypeSupport{}

type timeTypeSupport struct{}

func (timeTypeSupport) New() types.Message {
	return NewTime()
}

func (timeTypeSupport) PrepareMemory() unsafe.Pointer {
	return unsafe.Pointer(C.builtin_interfaces__msg__Time__create())
}

func (timeTypeSupport) ReleaseMemory(pointer unsafe.Pointer) {
	C.builtin_interfaces__msg__Time__destroy((*C.builtin_interfaces__msg__Time)(pointer))
}

func (timeTypeSupport) AsCStruct(dst unsafe.Pointer, msg types.Message) {
	value := msg.(*Time)
	mem := (*C.builtin_interfaces__msg__Time)(dst)
	mem.sec = C.int32_t(value.Sec)
	mem.nanosec = C.uint32_t(value.Nanosec)
}

func (timeTypeSupport) AsGoStruct(dst types.Message, src unsafe.Pointer) {
	value := dst.(*Time)
	mem := (*C.builtin_interfaces__msg__Time)(src)
	value.Sec = int32(mem.sec)
	value.Nanosec = uint32(mem.nanosec)
}

func (timeTypeSupport) TypeSupport() unsafe.Pointer {
	return unsafe.Pointer(C.rosidl_typesupport_c__get_message_type_support_handle__builtin_interfaces__msg__Time())
}
