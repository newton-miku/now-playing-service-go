// Package webview provides embedded WebView2 window support
//
// Requirements:
//   - WebView2 Runtime must be installed on Windows
//   - Build with CGO enabled: set CGO_ENABLED=1
//   - GCC compiler required (TDM-GCC or MinGW-w64)
//
// Build command:
//
//	set CGO_ENABLED=1
//	go build -ldflags="-H=windowsgui" -o now-playing-service-go.exe
//
// Note: The webview repository is at github.com/webview/webview_go
// Note: WebView must run on main OS thread - use Show() to create window from any thread
//       but Run() must be called from main() to start the event loop
package webview

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/newton-miku/now-playing-service-go/logger"
	realWebview "github.com/webview/webview_go"
)

var (
	webviewInstance realWebview.WebView
	isRunning       bool
	mu              sync.Mutex
	initDone        bool
	showChan        chan *Config
)

// Config holds webview configuration
type Config struct {
	Title     string
	URL       string
	Width     int
	Height    int
	Resizable bool
	Debug     bool
}

// DefaultConfig returns default webview configuration
func DefaultConfig() *Config {
	return &Config{
		Title:     "Now Playing - 媒体状态监控",
		URL:       "http://localhost:8080",
		Width:     520,
		Height:    900,
		Resizable: true,
		Debug:     false,
	}
}

// Show creates or shows the webview window
// Can be called from any goroutine
func Show(config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	mu.Lock()
	defer mu.Unlock()

	// Initialize on first call
	if !initDone {
		showChan = make(chan *Config, 1)
		initDone = true
	}

	// Send config to show channel (non-blocking)
	select {
	case showChan <- config:
		logger.Infof("Requested webview show: %s", config.URL)
		return nil
	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("webview is not initialized")
	}
}

// Run starts the webview event loop
// This must be called from main() and will block until Terminate() is called
func Run() {
	logger.Info("Webview event loop starting")

	// Ensure we run on main OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Process show requests
	for config := range showChan {
		// Create or recreate webview
		mu.Lock()

		if isRunning {
			// Just navigate to new URL
			logger.Info("Webview already running, navigating")
			webviewInstance.Navigate(config.URL)
			mu.Unlock()
			continue
		}

		logger.Info("Creating new webview instance")
		webviewInstance = realWebview.New(config.Debug)
		if webviewInstance == nil {
			logger.Error("Failed to create webview instance")
			mu.Unlock()
			continue
		}

		// Configure window
		webviewInstance.SetTitle(config.Title)
		var hint realWebview.Hint
		if config.Resizable {
			hint = realWebview.HintNone
		} else {
			hint = realWebview.HintFixed
		}
		webviewInstance.SetSize(config.Width, config.Height, hint)
		webviewInstance.Navigate(config.URL)

		isRunning = true
		mu.Unlock()

		logger.Infof("Webview running: %s", config.URL)

		// Run webview (blocking until terminated)
		webviewInstance.Run()

		// After Run() returns, cleanup
		mu.Lock()
		isRunning = false
		if webviewInstance != nil {
			webviewInstance.Destroy()
			webviewInstance = nil
		}
		mu.Unlock()

		logger.Info("Webview stopped")
	}
}

// Terminate stops the webview
func Terminate() {
	mu.Lock()
	defer mu.Unlock()

	if !isRunning || webviewInstance == nil {
		logger.Warn("Cannot terminate: webview is not running")
		return
	}

	logger.Info("Terminating webview")
	webviewInstance.Terminate()
}

// SetTitle changes the window title
func SetTitle(title string) {
	mu.Lock()
	defer mu.Unlock()

	if !isRunning || webviewInstance == nil {
		logger.Warn("Cannot set title: webview is not running")
		return
	}

	webviewInstance.SetTitle(title)
}

// SetSize changes the window size
func SetSize(width, height int, resizable bool) {
	mu.Lock()
	defer mu.Unlock()

	if !isRunning || webviewInstance == nil {
		logger.Warn("Cannot set size: webview is not running")
		return
	}

	var hint realWebview.Hint
	if resizable {
		hint = realWebview.HintNone
	} else {
		hint = realWebview.HintFixed
	}
	webviewInstance.SetSize(width, height, hint)
}

// Eval executes JavaScript in the webview
func Eval(js string) {
	mu.Lock()
	defer mu.Unlock()

	if !isRunning || webviewInstance == nil {
		logger.Warn("Cannot eval: webview is not running")
		return
	}

	webviewInstance.Eval(js)
}

// Init is an alias for Eval (compatibility)
func Init(js string) {
	Eval(js)
}

// Bind binds a Go function to JavaScript
func Bind(name string, fn interface{}) error {
	mu.Lock()
	defer mu.Unlock()

	if !isRunning || webviewInstance == nil {
		logger.Warn("Cannot bind: webview is not running")
		return nil
	}

	return webviewInstance.Bind(name, fn)
}

// IsRunning returns whether the webview is currently running
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return isRunning
}
