// Package server provides HTTP server and API endpoint functionality
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/newton-miku/now-playing-service-go/foreground"
	"github.com/newton-miku/now-playing-service-go/logger"
	"github.com/newton-miku/now-playing-service-go/music"
	"github.com/newton-miku/now-playing-service-go/settings"
	"github.com/newton-miku/now-playing-service-go/tools"
	"github.com/newton-miku/now-playing-service-go/utils"
)

// Server represents the HTTP server configuration
type Server struct {
	settings *settings.Settings
	port     string
}

// New creates a new Server instance
func New(s *settings.Settings, port string) *Server {
	return &Server{
		settings: s,
		port:     port,
	}
}

// Start initializes and starts the HTTP server
func (s *Server) Start() error {
	logger.Info("Registering HTTP handlers")
	s.registerHandlers()

	addr := fmt.Sprintf(":%s", s.port)
	logger.Info("=== Server Started ===")
	logger.Infof("Web UI: http://localhost:%s", s.port)
	logger.Infof("Global Music API (auto-detect): http://localhost:%s/api/music/global?preferred=%s", s.port, s.settings.PreferredPlatform)
	logger.Infof("Single Platform API: http://localhost:%s/api/music/platform", s.port)
	logger.Infof("Foreground API: http://localhost:%s/api/foreground", s.port)
	logger.Infof("Settings API: http://localhost:%s/api/settings", s.port)
	logger.Infof("Logs API: http://localhost:%s/api/logs", s.port)

	return http.ListenAndServe(addr, nil)
}

// registerHandlers registers all HTTP handlers
func (s *Server) registerHandlers() {
	logger.Debug("Registering static file handlers")
	// Static files - CSS, JS, and legacy static directory
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("web/css"))))
	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("web/js"))))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	logger.Debug("Registering API endpoints")
	// API endpoints
	http.HandleFunc("/api/status", s.statusHandler)
	http.HandleFunc("/api/music/global", s.globalMusicStatusHandler)
	http.HandleFunc("/api/music/platform", s.platformStatusHandler)
	http.HandleFunc("/api/platforms", s.platformsHandler)
	http.HandleFunc("/api/foreground", s.foregroundHandler)
	http.HandleFunc("/api/settings", s.settingsHandler)
	http.HandleFunc("/api/logs", s.logsHandler)
	http.HandleFunc("/api/logs/stream", s.logsStreamHandler)
	http.HandleFunc("/api/logs/level", s.logsLevelHandler)
	http.HandleFunc("/api/version", s.versionHandler)
	http.HandleFunc("/api/open-external", s.openExternalHandler)
	http.HandleFunc("/", s.webUIHandler)

	logger.Info("All HTTP handlers registered successfully")
}

// statusHandler handles aggregated status requests
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	preferred := r.URL.Query().Get("preferred")
	if preferred == "" {
		preferred = s.settings.PreferredPlatform
	}

	musicStatus := music.GetGlobalStatusSMTCPreferred(preferred, s.settings.SMTCPreferred)
	fgWindow := foreground.GetForegroundWindow()

	response := map[string]interface{}{
		"music":      musicStatus,
		"foreground": fgWindow,
		"timestamp":  time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(response)
}

// globalMusicStatusHandler handles global music status requests
func (s *Server) globalMusicStatusHandler(w http.ResponseWriter, r *http.Request) {
	preferred := r.URL.Query().Get("preferred")
	if preferred == "" {
		preferred = s.settings.PreferredPlatform
	}

	status := music.GetGlobalStatusSMTCPreferred(preferred, s.settings.SMTCPreferred)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(status)
}

// platformsHandler handles platform list requests
func (s *Server) platformsHandler(w http.ResponseWriter, r *http.Request) {
	platforms := make([]string, 0, len(music.PlatformConfigs))
	for k := range music.PlatformConfigs {
		platforms = append(platforms, k)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string][]string{"platforms": platforms})
}

// platformStatusHandler handles specific platform status requests
func (s *Server) platformStatusHandler(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = s.settings.PreferredPlatform
	}

	status := music.GetStatusWithMethodSMTCPreferred(platform, s.settings.SMTCPreferred)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(status)
}

// foregroundHandler handles foreground window requests
func (s *Server) foregroundHandler(w http.ResponseWriter, r *http.Request) {
	fg := foreground.GetForegroundWindow()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if fg == nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "no foreground window"})
		return
	}
	json.NewEncoder(w).Encode(fg)
}

// settingsHandler handles settings GET and POST requests
func (s *Server) settingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method == "GET" {
		logger.Debug("GET /api/settings requested")
		json.NewEncoder(w).Encode(s.settings)
		return
	}

	if r.Method == "POST" {
		logger.Info("POST /api/settings - saving settings")
		var newSettings settings.Settings
		if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
			logger.Warn("Invalid settings format received:", err)
			http.Error(w, "Invalid settings format", http.StatusBadRequest)
			return
		}

		// Update settings
		*s.settings = newSettings
		if err := s.settings.Save(); err != nil {
			logger.Error("Failed to save settings:", err)
			http.Error(w, "Failed to save settings", http.StatusInternalServerError)
			return
		}

		logger.Info("Settings saved successfully")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// webUIHandler handles web UI requests
func (s *Server) webUIHandler(w http.ResponseWriter, r *http.Request) {
	webDir, err := os.Getwd()
	if err != nil {
		logger.Errorf("Failed to get working directory for web UI: %v", err)
		http.Error(w, "Failed to get working directory", http.StatusInternalServerError)
		return
	}
	htmlPath := filepath.Join(webDir, "web", "index.html")

	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		logger.Errorf("Web UI file not found at: %s", htmlPath)
		http.Error(w, "Web UI file not found", http.StatusNotFound)
		return
	}

	logger.Debugf("Serving web UI from: %s", htmlPath)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, htmlPath)
}

// logsHandler handles log information requests
func (s *Server) logsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method == "GET" {
		response := map[string]interface{}{
			"logFile":      logger.GetLogPath(),
			"logDir":       logger.GetLogDir(),
			"recentLogs":   logger.GetRecentLogs(),
			"currentLevel": logger.GetLogLevel(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// logsStreamHandler handles SSE log streaming
func (s *Server) logsStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, unsubscribe := logger.SubscribeLogs()
	defer unsubscribe()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send current buffer first
	recent := logger.GetRecentLogs()
	for _, entry := range recent {
		data, _ := json.Marshal(entry)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	for {
		select {
		case entry := <-ch:
			data, _ := json.Marshal(entry)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// logsLevelHandler handles getting and setting log level
func (s *Server) logsLevelHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method == "GET" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"level": logger.GetLogLevel(),
		})
		return
	}

	if r.Method == "POST" {
		var req struct {
			Level int `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		logger.SetLogLevel(req.Level)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "level": req.Level})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// versionHandler handles version information requests
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	response := map[string]string{
		"version":   tools.Version,
		"buildTime": tools.BuildTime,
		"goVersion": runtime.Version(),
		"platform":  runtime.GOOS + "/" + runtime.GOARCH,
	}
	json.NewEncoder(w).Encode(response)
}

// openExternalHandler opens URL in system default browser
func (s *Server) openExternalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	logger.Infof("Opening external URL: %s", req.URL)
	if err := utils.OpenURL(req.URL); err != nil {
		logger.Errorf("Failed to open URL: %v", err)
		http.Error(w, "Failed to open URL", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
