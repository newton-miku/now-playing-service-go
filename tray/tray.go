// Package tray provides Windows system tray icon functionality using getlantern/systray
package tray

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/getlantern/systray"
	"github.com/newton-miku/now-playing-service-go/logger"
	"github.com/newton-miku/now-playing-service-go/music"
	"github.com/newton-miku/now-playing-service-go/settings"
	"github.com/newton-miku/now-playing-service-go/webview"
)

// Config holds the tray icon configuration
type Config struct {
	Title  string
	Port   string
	OnExit func()
}

var (
	trayConfig *Config
)

// Start initializes and starts the system tray icon
func Start(config *Config) {
	runtime.LockOSThread()
	trayConfig = config
	systray.Run(onReady, onExit)
}

// Stop stops the system tray icon
func Stop() {
	systray.Quit()
}

// onReady is called when the systray is ready
func onReady() {
	logger.Info("Tray: onReady started")
	// Set title and tooltip
	systray.SetTitle(trayConfig.Title)
	systray.SetTooltip(trayConfig.Title)

	// Try to load icon
	iconData, err := os.ReadFile("ico/icon.ico")
	if err == nil {
		systray.SetIcon(iconData)
	} else {
		// Try fallback to png if ico fails
		iconData, err = os.ReadFile("ico/icon.png")
		if err == nil {
			systray.SetIcon(iconData)
		} else {
			logger.Warnf("Tray: Could not load icon: %v", err)
		}
	}

	// Create menu items
	mShow := systray.AddMenuItem("显示界面", "打开Web界面")
	mSettings := systray.AddMenuItem("设置", "打开设置")
	systray.AddSeparator()

	// Add reporting toggle
	s := settings.Get()
	mReport := systray.AddMenuItemCheckbox("启用状态上报", "将状态上报至服务器", s.EnableReport)

	systray.AddSeparator()
	mExit := systray.AddMenuItem("退出", "退出程序")

	// Start tooltip update goroutine
	go updateTooltip()

	// Handle menu clicks
	go func() {
		logger.Info("Tray: event loop started")
		for {
			select {
			case <-mShow.ClickedCh:
				logger.Debug("Tray: Show clicked")
				openWebview(trayConfig.Port)
			case <-mSettings.ClickedCh:
				logger.Debug("Tray: Settings clicked")
				openWebview(trayConfig.Port)
			case <-mReport.ClickedCh:
				logger.Debug("Tray: Report toggle clicked")
				// Toggle reporting
				s := settings.Get()
				s.EnableReport = !s.EnableReport
				if s.EnableReport {
					mReport.Check()
				} else {
					mReport.Uncheck()
				}
				// Save settings (this will trigger callback in main.go)
				if err := s.Save(); err != nil {
					logger.Errorf("Failed to save settings from tray: %v", err)
				}
			case <-mExit.ClickedCh:
				logger.Info("Tray: Exit clicked")
				if trayConfig.OnExit != nil {
					trayConfig.OnExit()
				}
				systray.Quit()
				return
			}
		}
	}()

	// Watch settings for external changes (like from Web UI)
	settings.RegisterCallback(func(newSettings *settings.Settings) {
		logger.Debugf("Tray: Received settings update, EnableReport: %v", newSettings.EnableReport)
		if newSettings.EnableReport {
			mReport.Check()
		} else {
			mReport.Uncheck()
		}
	})
	logger.Info("Tray: onReady finished")
}

// onExit is called when the systray is exiting
func onExit() {
	// Cleanup if needed
	webview.Terminate()
}

// openWebview opens or focuses the webview window
func openWebview(port string) {
	logger.Info("Opening webview window from tray")
	url := fmt.Sprintf("http://localhost:%s", port)

	config := webview.DefaultConfig()
	config.URL = url

	// Show webview (can be called from any goroutine)
	if err := webview.Show(config); err != nil {
		logger.Errorf("Failed to show webview from tray: %v", err)
	}
}

// updateTooltip updates the tooltip with current music status
func updateTooltip() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		status := music.GetGlobalStatus("netease")
		var tooltip string

		if status.Status.Status == "Playing" || status.Status.Status == "Paused" {
			tooltip = status.Status.Title + " - " + status.Status.Artist
			if status.Status.Status == "Paused" {
				tooltip += " [已暂停]"
			}
		} else {
			tooltip = "未在播放音乐"
		}

		systray.SetTooltip(tooltip)
	}
}
