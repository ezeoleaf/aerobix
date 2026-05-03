package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// TryDesktopNotify shows a lightweight OS notification when available (best-effort, non-blocking caller).
func TryDesktopNotify(title, body string) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		title = "Aerobix"
	}
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`,
			body, title)
		_ = exec.Command("osascript", "-e", script).Start()
	case "linux", "freebsd", "netbsd", "openbsd":
		path, err := exec.LookPath("notify-send")
		if err != nil {
			return
		}
		_ = exec.Command(path, title, body).Start()
	default:
		return
	}
}
