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
	showChan        chan *Config
	quitChan        chan struct{}
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

	// Initialize channels if not already done
	mu.Lock()
	if showChan == nil {
		showChan = make(chan *Config, 1)
	}
	if quitChan == nil {
		quitChan = make(chan struct{})
	}
	mu.Unlock()

	// Process show requests and quit signal
	for {
		select {
		case config := <-showChan:
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
		case <-quitChan:
			logger.Info("Webview event loop received quit signal")
			// Terminate webview if it's running
			mu.Lock()
			if isRunning && webviewInstance != nil {
				logger.Info("Terminating webview from quit signal")
				webviewInstance.Terminate()
			}
			mu.Unlock()
			return
		}
	}
}

// Terminate stops the webview and exits the event loop
func Terminate() {
	mu.Lock()

	// If quit channel is not initialized, create it
	if quitChan == nil {
		quitChan = make(chan struct{})
	}

	// Check if webview is running
	if isRunning && webviewInstance != nil {
		logger.Info("Terminating webview")
		webviewInstance.Terminate()
		mu.Unlock()
		return
	}

	// If webview is not running, just signal quit to exit the event loop
	mu.Unlock()

	logger.Info("Webview not running, sending quit signal to event loop")
	select {
	case quitChan <- struct{}{}:
		logger.Info("Quit signal sent successfully")
	default:
		logger.Warn("Quit channel is busy or not ready")
	}
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
