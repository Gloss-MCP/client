package server

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser best-effort opens url in the user's default browser.
// Failure is never fatal to the caller -- headless/SSH/CI sessions have
// no browser to open and should just keep the printed URL.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
