// Package client provides a client for reporting to device-server
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/newton-miku/now-playing-service-go/foreground"
	"github.com/newton-miku/now-playing-service-go/logger"
	"github.com/newton-miku/now-playing-service-go/music"
)

// Config represents the reporter configuration
type Config struct {
	ServerURL      string // Device server URL, e.g., "http://localhost:21081"
	DeviceID       string // Unique device identifier
	DeviceName     string // Human-readable device name
	APIKey         string // API key for authentication
	ReportInterval int    // Reporting interval in milliseconds
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-device"
	}

	return &Config{
		ServerURL:      "http://localhost:21081",
		DeviceID:       hostname,
		DeviceName:     hostname,
		APIKey:         "",
		ReportInterval: 5000, // Default to 5 seconds
	}
}

// Reporter reports music status to device-server
type Reporter struct {
	config *Config
	client *http.Client
	mu     sync.Mutex
	stopCh chan struct{}
	ticker *time.Ticker
}

// NewReporter creates a new Reporter
func NewReporter(config *Config) *Reporter {
	if config.ReportInterval < 100 {
		config.ReportInterval = 100 // Enforce minimum 100ms
	}
	return &Reporter{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Report sends the current music status to the server
func (r *Reporter) Report(status *music.StatusWithMethod) error {
	// Try to get platform name from process name
	platform := getPlatformFromProcess(status.Status.ProcessName)

	musicInfo := MusicInfo{
		Status:   status.Status.Status,
		Title:    status.Status.Title,
		Artist:   status.Status.Artist,
		Album:    status.Status.Album,
		Platform: platform,
	}

	// Get foreground window info
	fgInfo := foreground.GetForegroundWindow()
	var foregroundInfo *ForegroundInfo
	if fgInfo != nil {
		foregroundInfo = &ForegroundInfo{
			Title:       fgInfo.Title,
			ProcessName: fgInfo.ProcessName,
			ProcessID:   fgInfo.ProcessID,
		}
	}

	report := DeviceReport{
		DeviceID:   r.config.DeviceID,
		Name:       r.config.DeviceName,
		Music:      musicInfo,
		Foreground: foregroundInfo,
		APIKey:     r.config.APIKey,
	}

	return r.sendReport(report)
}

// sendReport sends the report to the server
func (r *Reporter) sendReport(report DeviceReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	url := r.config.ServerURL + "/api/report"
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add API key to header if configured
	if r.config.APIKey != "" {
		req.Header.Set("X-API-Key", r.config.APIKey)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		logger.Errorf("Failed to send report to %s: %v", r.config.ServerURL, err)
		return fmt.Errorf("failed to send report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Errorf("Server %s returned error status: %d", r.config.ServerURL, resp.StatusCode)
		if resp.StatusCode == http.StatusUnauthorized {
			logger.Errorf("Reporting failed: Invalid API Key for server %s", r.config.ServerURL)
		}
		return fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	logger.Debugf("Report sent to %s: %s - %s", r.config.ServerURL, report.Name, report.Music.Title)
	return nil
}

// Start starts periodic reporting
func (r *Reporter) Start(preferredPlatform string) {
	r.StartWithSMTCPreference(preferredPlatform, true)
}

// StartWithSMTCPreference starts periodic reporting with SMTC preference
func (r *Reporter) StartWithSMTCPreference(preferredPlatform string, smtcPreferred bool) {
	r.mu.Lock()

	if r.ticker != nil {
		r.ticker.Stop()
	}
	if r.stopCh != nil {
		close(r.stopCh)
	}
	r.stopCh = make(chan struct{})

	interval := time.Duration(r.config.ReportInterval) * time.Millisecond
	logger.Infof("Reporter started. Reporting to %s every %v", r.config.ServerURL, interval)
	r.ticker = time.NewTicker(interval)
	stopCh := r.stopCh
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			if r.ticker != nil {
				r.ticker.Stop()
			}
			r.mu.Unlock()
		}()

		for {
			select {
			case <-r.ticker.C:
				status := music.GetGlobalStatusSMTCPreferred(preferredPlatform, smtcPreferred)
				if err := r.Report(status); err != nil {
					logger.Debugf("Failed to report: %v", err)
				}
			case <-stopCh:
				logger.Infof("Reporter stopped for device: %s", r.config.DeviceID)
				return
			}
		}
	}()
}

// Stop stops the reporter
func (r *Reporter) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopCh != nil {
		close(r.stopCh)
		logger.Infof("Stopping reporter for device: %s", r.config.DeviceID)
		r.stopCh = nil
	}
}

// UpdateConfig updates the reporter configuration
func (r *Reporter) UpdateConfig(config *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
	logger.Infof("Updated reporter config for device: %s", config.DeviceID)
}

// IsStopped checks if the reporter is stopped
func (r *Reporter) IsStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopCh == nil
}

// ForegroundInfo is the foreground application information
type ForegroundInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"process"`
	ProcessID   int    `json:"processId"`
}

// DeviceReport is the report structure
type DeviceReport struct {
	DeviceID   string          `json:"deviceId"`
	Name       string          `json:"name"`
	Music      MusicInfo       `json:"music"`
	Foreground *ForegroundInfo `json:"foreground,omitempty"`
	APIKey     string          `json:"apiKey,omitempty"`
}

// MusicInfo is the music information structure
type MusicInfo struct {
	Status   string `json:"status"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Platform string `json:"platform"`
}

// getPlatformFromProcess tries to get a friendly platform name from process name
func getPlatformFromProcess(processName string) string {
	if processName == "" {
		return ""
	}

	// Common process name mappings
	platformMap := map[string]string{
		"cloudmusic": "网易云音乐",
		"CloudMusic": "网易云音乐",
		"QQMusic":    "QQ音乐",
		"qqmusic":    "QQ音乐",
		"Kugou":      "酷狗音乐",
		"kugou":      "酷狗音乐",
		"Kuwo":       "酷我音乐",
		"kuwo":       "酷我音乐",
		"Spotify":    "Spotify",
		"AppleMusic": "Apple Music",
		"Music":      "Apple Music",
		"foobar2000": "foobar2000",
		"PotPlayer":  "PotPlayer",
		"AIMP":       "AIMP",
		"lxmusic":    "洛雪音乐",
		"LxMusic":    "洛雪音乐",
	}

	// Check for exact matches first
	if name, ok := platformMap[processName]; ok {
		return name
	}

	// Check for substring matches
	for key, name := range platformMap {
		if len(key) > 0 && len(processName) >= len(key) {
			if containsIgnoreCase(processName, key) {
				return name
			}
		}
	}

	return processName
}

// containsIgnoreCase checks if a string contains another string (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(len(substr) == 0 || containsLowercase(toLower(s), toLower(substr)))
}

// toLower converts string to lowercase
func toLower(s string) string {
	res := make([]rune, len(s))
	for i, r := range s {
		if 'A' <= r && r <= 'Z' {
			res[i] = r + 32
		} else {
			res[i] = r
		}
	}
	return string(res)
}

// containsLowercase checks if lowercase string contains another (both already lowercase)
func containsLowercase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
