package switcher

import (
	"os"
	"os/exec"
	"runtime"
)

// CheckCursorIDE reports whether we should refuse a Cursor write because the
// IDE is running (it holds state.vscdb), or because we cannot tell. Tests set
// LEEWAY_CURSOR_IDE_CHECK=0.
func CheckCursorIDE() (block bool, reason string) {
	if os.Getenv("LEEWAY_CURSOR_IDE_CHECK") == "0" {
		return false, ""
	}
	if _, err := exec.LookPath("pgrep"); err != nil {
		return true, "找不到 pgrep，无法确认 Cursor 是否在跑。先退出 Cursor，或把 pgrep 装上"
	}
	if CursorIDERunning() {
		return true, "Cursor 正在运行。先退出再切号，避免写坏 state.vscdb"
	}
	return false, ""
}

// CursorIDERunning is true when the Cursor desktop process is up.
// cursor-agent is a different binary and is ignored. Caller must have pgrep.
func CursorIDERunning() bool {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"Cursor"}
	case "linux":
		names = []string{"cursor", "cursor-bin"}
	default:
		return false
	}
	for _, name := range names {
		if exec.Command("pgrep", "-x", name).Run() == nil {
			return true
		}
	}
	return false
}
