// Package foreground provides functionality to get the current foreground/active window
// Uses Windows API directly for better performance and reliability
package foreground

import (
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowInfo represents information about the foreground window
type WindowInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	ProcessID   int    `json:"process_id"`
}

// GetForegroundWindow returns information about the currently active/focused window
func GetForegroundWindow() *WindowInfo {
	// Get foreground window handle using user32.dll directly
	user32 := syscall.MustLoadDLL("user32.dll")
	getForegroundWindow := user32.MustFindProc("GetForegroundWindow")
	hwnd, _, _ := getForegroundWindow.Call()
	if hwnd == 0 {
		return nil
	}

	// Get window title
	getWindowText := user32.MustFindProc("GetWindowTextW")
	titleBuf := make([]uint16, 256)
	len, _, _ := getWindowText.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), 256)
	if len == 0 {
		return nil
	}
	title := syscall.UTF16ToString(titleBuf[:len])

	// Get process ID
	getWindowThreadProcessId := user32.MustFindProc("GetWindowThreadProcessId")
	var pid uint32
	getWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return nil
	}

	// Get process name
	procName := getProcessName(pid)

	return &WindowInfo{
		Title:       title,
		ProcessName: procName,
		ProcessID:   int(pid),
	}
}

// getProcessName returns the process name given a process ID
func getProcessName(pid uint32) string {
	// Open process with query information access
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle) //nolint:errcheck

	// Get process name
	var nameBuf [260]uint16
	size := uint32(len(nameBuf))
	err = windows.QueryFullProcessImageName(handle, 0, &nameBuf[0], &size)
	if err != nil {
		return ""
	}

	// Extract just the filename
	name := utf16.Decode(nameBuf[:size])
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '\\' || name[i] == '/' {
			return string(name[i+1:])
		}
	}
	return string(name)
}

// GetWindowsWithTitles returns all visible windows with their titles
// This is useful for finding music player windows
func GetWindowsWithTitles() []*WindowInfo {
	var result []*WindowInfo
	user32 := syscall.MustLoadDLL("user32.dll")

	callback := syscall.NewCallback(func(hwnd syscall.Handle, lParam uintptr) uintptr {
		// Get window title
		getWindowText := user32.MustFindProc("GetWindowTextW")
		titleBuf := make([]uint16, 256)
		len, _, _ := getWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&titleBuf[0])), 256)
		if len == 0 {
			return 1 // Continue enumeration
		}
		title := syscall.UTF16ToString(titleBuf[:len])

		// Skip empty titles
		if title == "" {
			return 1
		}

		// Also check if window is visible or minimized
		isWindowVisible := user32.MustFindProc("IsWindowVisible")
		visible, _, _ := isWindowVisible.Call(uintptr(hwnd))
		
		isIconic := user32.MustFindProc("IsIconic")
		minimized, _, _ := isIconic.Call(uintptr(hwnd))

		if visible == 0 && minimized == 0 {
			// If not visible and not minimized, check if it's a tool window or something we might care about
			// For now, let's just skip if it's completely invisible unless it's a known player
			// But wait, we don't know the process yet. 
			// Let's just include all windows with titles for now and filter later.
		}

		// Get process ID
		getWindowThreadProcessId := user32.MustFindProc("GetWindowThreadProcessId")
		var pid uint32
		getWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
		if pid == 0 {
			return 1
		}

		// Get process name
		procName := getProcessName(pid)

		result = append(result, &WindowInfo{
			Title:       title,
			ProcessName: procName,
			ProcessID:   int(pid),
		})

		return 1 // Continue enumeration
	})

	enumWindows := user32.MustFindProc("EnumWindows")
	enumWindows.Call(callback, 0)
	return result
}
