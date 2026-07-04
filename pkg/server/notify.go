package server

import (
	"fmt"
	"os/exec"
	"runtime"
)

// notifyPopup 跨平台桌面通知。用于 namespace_delete 确认码弹窗。
// Go 进程（agentflow.exe）有用户桌面权限，可以直接弹。
func notifyPopup(title, msg string) {
	switch runtime.GOOS {
	case "windows":
		// PowerShell WinForms MessageBox
		ps := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.MessageBox]::Show('%s','%s',0,64)`,
			msg, title,
		)
		_ = exec.Command("powershell", "-NoProfile", "-Command", ps).Run()

	case "darwin":
		// macOS 通知中心
		_ = exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification "%s" with title "%s"`, msg, title),
		).Run()

	default:
		// Linux notify-send
		_ = exec.Command("notify-send", title, msg).Run()
	}
}
