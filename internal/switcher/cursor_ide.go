package switcher

import (
	"os"
	"os/exec"
	"runtime"
)

// CheckCursorIDE reports whether we should refuse a Cursor write because the
// IDE is running (it holds state.vscdb). Tests set LEEWAY_CURSOR_IDE_CHECK=0.
func CheckCursorIDE() bool {
	if os.Getenv("LEEWAY_CURSOR_IDE_CHECK") == "0" {
		return false
	}
	return CursorIDERunning()
}

// CursorIDERunning is true when the Cursor desktop process is up.
// cursor-agent is a different binary and is ignored.
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
	if _, err := exec.LookPath("pgrep"); err != nil {
		return false
	}
	for _, name := range names {
		if exec.Command("pgrep", "-x", name).Run() == nil {
			return true
		}
	}
	return false
}
