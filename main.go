// Package main provides music service detection and foreground window detection
// Uses modular architecture with separate packages for music and foreground detection
package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/newton-miku/now-playing-service-go/client"
	"github.com/newton-miku/now-playing-service-go/logger"
	"github.com/newton-miku/now-playing-service-go/music"
	"github.com/newton-miku/now-playing-service-go/server"
	"github.com/newton-miku/now-playing-service-go/settings"
	"github.com/newton-miku/now-playing-service-go/tools"
	"github.com/newton-miku/now-playing-service-go/tray"
	"github.com/newton-miku/now-playing-service-go/utils"
	"github.com/newton-miku/now-playing-service-go/webview"
)

var (
	reporterInstance *client.Reporter
	reporterMutex    sync.RWMutex
)

func main() {
	// Initialize logger first
	if err := logger.Init("now-playing-service-go"); err != nil {
		fmt.Printf("Warning: Could not initialize logger: %v\n", err)
	}
	defer logger.Close()

	// Set log level to DEBUG for SMTC debugging
	logger.SetLogLevel(logger.DEBUG)

	logger.Info("Application starting...")
	logger.Infof("Version: %s", tools.Version)
	logger.Infof("Build Time: %s", tools.BuildTime)

	// Load settings first
	s := settings.Get()

	// Apply log level from settings
	logger.SetLogLevel(s.LogLevel)

	// Register settings change callback for real-time updates
	settings.RegisterCallback(func(newSettings *settings.Settings) {
		logger.Info("Settings changed, reloading configuration...")
		// Update log level
		logger.SetLogLevel(newSettings.LogLevel)
		// Restart reporter with new settings if configured and enabled
		if newSettings.ReportServerURL != "" && newSettings.EnableReport {
			go startDeviceReporterWithSMTC(
				newSettings.ReportServerURL,
				newSettings.ReportDeviceID,
				newSettings.ReportDeviceName,
				newSettings.ReportAPIKey,
				newSettings.ReportInterval,
				newSettings.PreferredPlatform,
				newSettings.SMTCPreferred,
			)
		} else if !newSettings.EnableReport || newSettings.ReportServerURL == "" {
			// Stop reporter if disabled or URL is empty
			stopDeviceReporter()
		}
	})

	// Parse command-line flags (use empty defaults to detect if user provided them)
	preferred := flag.String("preferred", "", "Preferred music platform (priority detection)")
	platform := flag.String("platform", "", "Specify single music platform for detection")
	serverMode := flag.Bool("server", true, "Start HTTP API server (default: true)")
	port := flag.String("port", "", "HTTP server port")
	noTray := flag.Bool("no-tray", false, "Disable system tray icon")
	smtcPreferred := flag.String("smtc", "", "Prefer SMTC for media detection (true/false)")
	reportServer := flag.String("report-server", "", "Device server URL to report status (e.g., http://localhost:21081)")
	reportDeviceID := flag.String("report-id", "", "Device ID for reporting (defaults to hostname)")
	reportDeviceName := flag.String("report-name", "", "Device name for reporting (defaults to hostname)")
	reportAPIKey := flag.String("report-api-key", "", "API key for device server authentication")
	reportInterval := flag.Int("report-interval", 0, "Reporting interval to device server in milliseconds (default: 5000)")
	saveSettings := flag.Bool("save", false, "Save settings to config file")
	help := flag.Bool("h", false, "Show help")
	flag.BoolVar(help, "help", false, "Show help")
	flag.Parse()

	if *help {
		printHelp()
		logger.Info("Showing help and exiting")
		os.Exit(0)
	}

	// Merge command-line flags with settings (command-line takes precedence)
	settingsModified := false
	effectivePreferred := s.PreferredPlatform
	effectivePort := s.Port
	effectiveReportServer := s.ReportServerURL
	effectiveReportID := s.ReportDeviceID
	effectiveReportName := s.ReportDeviceName
	effectiveReportAPIKey := s.ReportAPIKey
	effectiveSMTCPreferred := s.SMTCPreferred
	effectiveReportInterval := s.ReportInterval

	if *preferred != "" && *preferred != s.PreferredPlatform {
		effectivePreferred = *preferred
		s.PreferredPlatform = *preferred
		settingsModified = true
	}
	if *port != "" && *port != s.Port {
		effectivePort = *port
		s.Port = *port
		settingsModified = true
	}
	if *reportServer != "" && *reportServer != s.ReportServerURL {
		effectiveReportServer = *reportServer
		s.ReportServerURL = *reportServer
		settingsModified = true
	}
	if *reportDeviceID != "" && *reportDeviceID != s.ReportDeviceID {
		effectiveReportID = *reportDeviceID
		s.ReportDeviceID = *reportDeviceID
		settingsModified = true
	}
	if *reportDeviceName != "" && *reportDeviceName != s.ReportDeviceName {
		effectiveReportName = *reportDeviceName
		s.ReportDeviceName = *reportDeviceName
		settingsModified = true
	}
	if *reportAPIKey != "" && *reportAPIKey != s.ReportAPIKey {
		effectiveReportAPIKey = *reportAPIKey
		s.ReportAPIKey = *reportAPIKey
		settingsModified = true
	}
	if *reportInterval > 0 && *reportInterval != s.ReportInterval {
		effectiveReportInterval = *reportInterval
		s.ReportInterval = *reportInterval
		settingsModified = true
	}
	if *smtcPreferred != "" {
		// Parse boolean from string
		preferred := false
		if *smtcPreferred == "true" || *smtcPreferred == "1" {
			preferred = true
		}
		if preferred != s.SMTCPreferred {
			effectiveSMTCPreferred = preferred
			s.SMTCPreferred = preferred
			settingsModified = true
		}
	}

	// Save settings if requested or modified
	if *saveSettings || settingsModified {
		if err := s.Save(); err != nil {
			logger.Warn("Failed to save settings:", err)
		} else {
			logger.Info("Settings saved successfully")
		}
	}

	logger.Infof("Server mode: %v, Port: %s, Preferred platform: %s", *serverMode, effectivePort, effectivePreferred)
	if effectiveReportServer != "" && s.EnableReport {
		logger.Infof("Reporting to: %s (device: %s, interval: %dms)", effectiveReportServer, effectiveReportName, effectiveReportInterval)
	}

	// Start device reporter if configured and enabled
	if effectiveReportServer != "" && s.EnableReport {
		go startDeviceReporterWithSMTC(effectiveReportServer, effectiveReportID, effectiveReportName, effectiveReportAPIKey, effectiveReportInterval, effectivePreferred, effectiveSMTCPreferred)
	}

	if *serverMode {
		runServerMode(effectivePreferred, effectivePort, *noTray, *platform, effectiveSMTCPreferred)
		return
	}

	runConsoleMode(effectivePreferred, effectiveSMTCPreferred)
}

// printHelp prints the help message
func printHelp() {
	fmt.Printf("Now Playing Service - Music playback status detector\n")
	fmt.Printf("Version: %s\n", tools.Version)
	fmt.Printf("Build Time: %s\n", tools.BuildTime)
	fmt.Println()
	fmt.Println("Usage: now-playing [options]")
	fmt.Println()
	fmt.Println("Configuration is saved to config/settings.json")
	fmt.Println("Command-line flags override saved settings and are auto-saved.")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
}

// startDeviceReporter starts the device status reporter
func startDeviceReporter(serverURL, deviceID, deviceName, apiKey string, interval int, preferred string) {
	startDeviceReporterWithSMTC(serverURL, deviceID, deviceName, apiKey, interval, preferred, true)
}

// startDeviceReporterWithSMTC starts the device status reporter with SMTC preference
func startDeviceReporterWithSMTC(serverURL, deviceID, deviceName, apiKey string, interval int, preferred string, smtcPreferred bool) {
	reporterMutex.Lock()
	defer reporterMutex.Unlock()

	// Stop existing reporter if it's running
	if reporterInstance != nil {
		logger.Info("Stopping existing reporter...")
		reporterInstance.Stop()
		// Wait a bit for it to stop
		time.Sleep(100 * time.Millisecond)
	}

	// Double check if report is still enabled
	if !settings.Get().EnableReport {
		logger.Info("Report is disabled, not starting reporter")
		return
	}

	// Create new configuration
	cfg := client.DefaultConfig()
	cfg.ServerURL = serverURL
	cfg.APIKey = apiKey
	cfg.ReportInterval = interval
	if deviceID != "" {
		cfg.DeviceID = deviceID
	}
	if deviceName != "" {
		cfg.DeviceName = deviceName
	}

	// Create and start new reporter
	reporterInstance = client.NewReporter(cfg)
	logger.Infof("Starting new reporter to %s (device: %s, interval: %dms, smtcPreferred: %v)", serverURL, cfg.DeviceID, cfg.ReportInterval, smtcPreferred)

	// Start reporter in goroutine
	go reporterInstance.StartWithSMTCPreference(preferred, smtcPreferred)
}

// stopDeviceReporter stops the device status reporter
func stopDeviceReporter() {
	reporterMutex.Lock()
	defer reporterMutex.Unlock()

	if reporterInstance != nil {
		logger.Info("Stopping reporter...")
		reporterInstance.Stop()
		reporterInstance = nil
	}
}

// runServerMode runs the application in server mode with HTTP API
func runServerMode(preferred, portStr string, noTray bool, singlePlatform string, smtcPreferred bool) {
	logger.Info("Starting in server mode")

	// Get settings (already loaded in main)
	s := settings.Get()
	logger.Infof("Settings loaded, effective port: %s", portStr)

	// Create and start HTTP server
	srv := server.New(s, portStr)

	// Start server in goroutine
	go func() {
		logger.Infof("Starting HTTP server on port %s", portStr)
		if err := srv.Start(); err != nil {
			logger.Error("HTTP server error:", err)
			fmt.Printf("HTTP server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait a bit for server to start
	time.Sleep(500 * time.Millisecond)

	// Start webview if enabled
	if s.AutoOpenBrowser {
		logger.Info("Starting webview window")
		wvConfig := webview.DefaultConfig()
		wvConfig.URL = fmt.Sprintf("http://localhost:%s", portStr)

		// Show webview (non-blocking)
		if err := webview.Show(wvConfig); err != nil {
			logger.Errorf("Failed to show webview: %v", err)
		}
	}

	// Setup system tray icon if not disabled
	if !noTray {
		logger.Info("Starting system tray icon")
		trayConfig := &tray.Config{
			Title: "Now Playing",
			Port:  portStr,
			OnExit: func() {
				logger.Info("Tray exit requested")
				webview.Terminate() // Terminate webview
				os.Exit(0)
			},
		}

		// Run tray in goroutine
		go func() {
			tray.Start(trayConfig)
		}()
	}

	// Run webview event loop (blocking, must be on main thread)
	logger.Info("Starting webview event loop")
	webview.Run()

	logger.Info("Application exiting")
}

// runConsoleMode runs the application in console mode (no HTTP server)
func runConsoleMode(preferred string, smtcPreferred bool) {
	logger.Info("Starting in console mode")
	var prevStatus *music.StatusWithMethod

	for {
		status := music.GetGlobalStatusSMTCPreferred(preferred, smtcPreferred)

		// Only print when status changes
		if prevStatus == nil ||
			status.Status.Status != prevStatus.Status.Status ||
			status.Status.Title != prevStatus.Status.Title ||
			status.Status.Artist != prevStatus.Status.Artist {
			utils.PrintStatus(&status.Status)
			if status.Status.Status == "Playing" {
				logger.Infof("Now playing: %s - %s", status.Status.Title, status.Status.Artist)
			}
		}

		prevStatus = status
		time.Sleep(100 * time.Millisecond)
	}
}
