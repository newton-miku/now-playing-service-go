// Package settings provides configurable settings functionality
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows/registry"
)

// Settings represents the application settings
type Settings struct {
	PreferredPlatform string `json:"preferred_platform"`
	Port              string `json:"port"`
	CheckInterval     int    `json:"check_interval_ms"`
	AutoOpenBrowser   bool   `json:"auto_open_browser"`
	AutoStart         bool   `json:"auto_start"`
	// SMTC settings
	SMTCPreferred bool `json:"smtc_preferred"`
	// Device reporter settings
	EnableReport     bool   `json:"enable_report"`
	ReportServerURL  string `json:"report_server_url"`
	ReportInterval   int    `json:"report_interval_ms"` // In milliseconds
	ReportDeviceID   string `json:"report_device_id"`
	ReportDeviceName string `json:"report_device_name"`
	ReportAPIKey     string `json:"report_api_key"`
	// Logger settings
	LogLevel int `json:"log_level"`
}

var (
	instance     *Settings
	once         sync.Once
	settingsPath string
	mu           sync.RWMutex
	callbacks    []func(*Settings)
)

const (
	appName = "NowPlayingMonitor"
	runKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// DefaultSettings returns the default settings
func DefaultSettings() *Settings {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-device"
	}

	return &Settings{
		PreferredPlatform: "netease",
		Port:              "21080",
		CheckInterval:     100,
		AutoOpenBrowser:   true,
		AutoStart:         false,
		SMTCPreferred:     true,
		// Device reporter defaults
		EnableReport:     true,
		ReportServerURL:  "",
		ReportInterval:   1000, // 1 seconds
		ReportDeviceID:   hostname,
		ReportDeviceName: hostname,
		ReportAPIKey:     "",
		LogLevel:         1, // INFO
	}
}

// UpdateAutoStart updates the Windows registry for auto-start
func (s *Settings) UpdateAutoStart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer k.Close()

	if s.AutoStart {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
		// Add --server flag to ensure it starts in background/server mode
		cmd := fmt.Sprintf("\"%s\" --server", exePath)
		err = k.SetStringValue(appName, cmd)
		if err != nil {
			return fmt.Errorf("failed to set registry value: %w", err)
		}
	} else {
		// Ignore error if key doesn't exist
		_ = k.DeleteValue(appName)
	}
	return nil
}

// SyncAutoStart ensures registry matches setting
func (s *Settings) SyncAutoStart() {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	_, _, err = k.GetStringValue(appName)
	exists := err == nil
	if exists != s.AutoStart {
		_ = s.UpdateAutoStart()
	}
}

// RegisterCallback registers a callback to be notified when settings change
func RegisterCallback(callback func(*Settings)) {
	mu.Lock()
	defer mu.Unlock()
	callbacks = append(callbacks, callback)
}

// Get returns the singleton settings instance
func Get() *Settings {
	once.Do(func() {
		instance = DefaultSettings()
		loadSettings()
		instance.SyncAutoStart()
	})
	return instance
}

// getSettingsPath returns the path to the settings file
func getSettingsPath() string {
	if settingsPath != "" {
		return settingsPath
	}

	// Use executable directory for settings to support auto-start
	exePath, err := os.Executable()
	workingDir := filepath.Dir(exePath)
	if err != nil {
		workingDir, _ = os.Getwd()
	}

	// Create config directory
	configDir := filepath.Join(workingDir, "config")
	os.MkdirAll(configDir, 0755)

	settingsPath = filepath.Join(configDir, "settings.json")
	return settingsPath
}

// loadSettings loads settings from file
func loadSettings() {
	path := getSettingsPath()

	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist, use defaults
		return
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		// Invalid JSON, use defaults
		return
	}

	// Update instance with loaded settings
	instance = &s
}

// Save saves the current settings to file
func (s *Settings) Save() error {
	path := getSettingsPath()

	// Validation
	if s.ReportInterval < 100 {
		s.ReportInterval = 100
	}
	if s.CheckInterval < 100 {
		s.CheckInterval = 100
	}

	// Sync registry before saving
	_ = s.UpdateAutoStart()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// Notify callbacks about settings change
	s.notifyCallbacks()
	return nil
}

// notifyCallbacks notifies all registered callbacks of settings change
func (s *Settings) notifyCallbacks() {
	mu.RLock()
	defer mu.RUnlock()

	for _, callback := range callbacks {
		// Run callbacks in goroutines to avoid blocking
		go callback(s)
	}
}

// SetPreferredPlatform sets the preferred music platform
func (s *Settings) SetPreferredPlatform(platform string) error {
	s.PreferredPlatform = platform
	return s.Save()
}

// SetPort sets the HTTP server port
func (s *Settings) SetPort(port string) error {
	s.Port = port
	return s.Save()
}

// SetCheckInterval sets the music check interval in milliseconds
func (s *Settings) SetCheckInterval(ms int) error {
	s.CheckInterval = ms
	return s.Save()
}

// SetAutoOpenBrowser sets whether to auto-open the browser on startup
func (s *Settings) SetAutoOpenBrowser(autoOpen bool) error {
	s.AutoOpenBrowser = autoOpen
	return s.Save()
}

// SetSMTCPreferred sets whether to prefer SMTC for media detection
func (s *Settings) SetSMTCPreferred(preferred bool) error {
	s.SMTCPreferred = preferred
	return s.Save()
}

// SetReportServerURL sets the device server URL to report to
func (s *Settings) SetReportServerURL(url string) error {
	s.ReportServerURL = url
	return s.Save()
}

// SetReportDeviceID sets the device ID for reporting
func (s *Settings) SetReportDeviceID(id string) error {
	s.ReportDeviceID = id
	return s.Save()
}

// SetReportDeviceName sets the device name for reporting
func (s *Settings) SetReportDeviceName(name string) error {
	s.ReportDeviceName = name
	return s.Save()
}

// SetReportAPIKey sets the API key for reporting
func (s *Settings) SetReportAPIKey(key string) error {
	s.ReportAPIKey = key
	return s.Save()
}
