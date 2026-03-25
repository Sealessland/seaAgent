package peripherals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestROS2ShellCommandExpandsSetupPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	command := ros2ShellCommand([]string{"~/zed_ws/install/setup.bash"}, "ros2 topic list")
	if len(command) != 3 {
		t.Fatalf("unexpected command length: %d", len(command))
	}

	want := "source '" + filepath.Join(home, "zed_ws/install/setup.bash") + "' >/dev/null 2>&1;"
	if !strings.Contains(command[2], want) {
		t.Fatalf("command %q does not contain expanded setup path %q", command[2], want)
	}
}
