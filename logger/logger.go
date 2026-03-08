// Package logger provides logging functionality with file output
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// LogEntry represents a single log message with metadata
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     int       `json:"level"`
	LevelName string    `json:"level_name"`
	Message   string    `json:"message"`
}

var (
	logger      *log.Logger
	logFile     *os.File
	once        sync.Once
	logDir      string
	logPath     string
	maxLogSize  int64 = 10 * 1024 * 1024 // 10 MB
	maxLogFiles       = 3

	// Memory buffer for recent logs
	logBuffer     []LogEntry
	bufferMutex   sync.RWMutex
	maxBufferSize = 500

	// SSE broadcast channels
	logSubscribers = make(map[chan LogEntry]bool)
	subMutex       sync.RWMutex
)

// Log levels
const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = []string{
	"DEBUG",
	"INFO",
	"WARN",
	"ERROR",
	"FATAL",
}

// currentLogLevel is the current minimum level to log
var currentLogLevel = INFO

// SetLogLevel sets the minimum log level to output
func SetLogLevel(level int) {
	if level >= DEBUG && level <= FATAL {
		currentLogLevel = level
	}
}

// isGUIApp returns true if stdout is not available (GUI application)
func isGUIApp() bool {
	// Check if stdout is available (not a GUI app)
	_, err := os.Stdout.Stat()
	return err != nil
}

// openLogFileWithShareMode opens a log file with Windows file sharing mode
// This allows other processes to read the log file while it's being written
func openLogFileWithShareMode(path string) (*os.File, error) {
	if runtime.GOOS == "windows" {
		// Use Windows-specific CreateFile to set share mode
		pathPtr, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			return nil, err
		}

		const (
			FILE_APPEND_DATA   = 0x0004
			FILE_SHARE_READ    = 0x00000001
			FILE_SHARE_WRITE   = 0x00000002
			OPEN_ALWAYS        = 4
			FILE_ATTRIBUTE_NORMAL = 0x00000080
		)

		handle, err := syscall.CreateFile(
			pathPtr,
			FILE_APPEND_DATA,
			FILE_SHARE_READ|FILE_SHARE_WRITE,
			nil,
			OPEN_ALWAYS,
			FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if err != nil {
			return nil, err
		}

		return os.NewFile(uintptr(handle), path), nil
	}

	// For non-Windows platforms, use standard os.OpenFile
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// Init initializes the logger with file output
func Init(appName string) error {
	var initErr error
	once.Do(func() {
		initErr = initLogger(appName)
	})
	return initErr
}

func initLogger(appName string) error {
	// Use executable directory for logs to support auto-start
	exePath, err := os.Executable()
	workingDir := filepath.Dir(exePath)
	if err != nil {
		workingDir, _ = os.Getwd()
	}

	// Create log directory
	logDir = filepath.Join(workingDir, "logs")
	if err = os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Generate log filename with date
	timestamp := time.Now().Format("2006-01-02")
	logPath = filepath.Join(logDir, fmt.Sprintf("%s-%s.log", appName, timestamp))

	// Rotate logs if needed
	rotateLogs()

	// Open log file with proper sharing mode for Windows
	// This allows other processes to read the log file while it's being written
	logFile, err = openLogFileWithShareMode(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Create writer: file only for GUI apps, file + stdout for console apps
	var writer io.Writer = logFile
	if !isGUIApp() {
		writer = io.MultiWriter(os.Stdout, logFile)
	}
	logger = log.New(writer, "", log.LstdFlags|log.Lmicroseconds)

	Info("Logger initialized")
	Info("Log file:", logPath)

	return nil
}

// rotateLogs rotates log files when they reach max size
func rotateLogs() {
	// Check current log file
	info, err := os.Stat(logPath)
	if err == nil && info.Size() > maxLogSize {
		// Rotate existing logs
		for i := maxLogFiles - 1; i >= 1; i-- {
			src := filepath.Join(logDir, fmt.Sprintf("now-playing-service-go-%s.log.%d", time.Now().Format("2006-01-02"), i))
			dst := filepath.Join(logDir, fmt.Sprintf("now-playing-service-go-%s.log.%d", time.Now().Format("2006-01-02"), i+1))
			os.Rename(src, dst)
		}

		// Rename current log to .1
		if maxLogFiles > 1 {
			dst := filepath.Join(logDir, fmt.Sprintf("now-playing-service-go-%s.log.1", time.Now().Format("2006-01-02")))
			os.Rename(logPath, dst)
		}
	}
}

// Close closes the log file
func Close() {
	if logFile != nil {
		Info("Logger shutting down")
		logFile.Close()
		logFile = nil
	}
}

// GetLogPath returns the path to the current log file
func GetLogPath() string {
	return logPath
}

// GetLogDir returns the log directory path
func GetLogDir() string {
	return logDir
}

// logf logs a formatted message with level
func logf(level int, format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		LevelName: levelNames[level],
		Message:   message,
	}

	// Add to memory buffer
	bufferMutex.Lock()
	logBuffer = append(logBuffer, entry)
	if len(logBuffer) > maxBufferSize {
		logBuffer = logBuffer[1:]
	}
	bufferMutex.Unlock()

	// Broadcast to SSE subscribers
	subMutex.RLock()
	for ch := range logSubscribers {
		select {
		case ch <- entry:
		default:
			// Client slow, skip this message
		}
	}
	subMutex.RUnlock()

	if level < currentLogLevel {
		return
	}

	if logger == nil {
		// Fallback to stdout if logger not initialized
		fmt.Printf("[%s] %s\n", levelNames[level], message)
		return
	}

	prefix := fmt.Sprintf("[%s] ", levelNames[level])
	logger.Output(3, prefix+message)

	// Exit on fatal
	if level == FATAL {
		Close()
		os.Exit(1)
	}
}

// SubscribeLogs adds a channel to receive log updates
func SubscribeLogs() (chan LogEntry, func()) {
	ch := make(chan LogEntry, 100)
	subMutex.Lock()
	logSubscribers[ch] = true
	subMutex.Unlock()

	unsubscribe := func() {
		subMutex.Lock()
		delete(logSubscribers, ch)
		subMutex.Unlock()
		close(ch)
	}

	return ch, unsubscribe
}

// GetRecentLogs returns the current memory buffer of logs
func GetRecentLogs() []LogEntry {
	bufferMutex.RLock()
	defer bufferMutex.RUnlock()

	logs := make([]LogEntry, len(logBuffer))
	copy(logs, logBuffer)
	return logs
}

// GetLogLevel returns the current log level
func GetLogLevel() int {
	return currentLogLevel
}

// Debug logs a debug message
func Debug(v ...interface{}) {
	logf(DEBUG, "%s", fmt.Sprint(v...))
}

// Debugf logs a formatted debug message
func Debugf(format string, v ...interface{}) {
	logf(DEBUG, format, v...)
}

// Info logs an info message
func Info(v ...interface{}) {
	logf(INFO, "%s", fmt.Sprint(v...))
}

// Infof logs a formatted info message
func Infof(format string, v ...interface{}) {
	logf(INFO, format, v...)
}

// Warn logs a warning message
func Warn(v ...interface{}) {
	logf(WARN, "%s", fmt.Sprint(v...))
}

// Warnf logs a formatted warning message
func Warnf(format string, v ...interface{}) {
	logf(WARN, format, v...)
}

// Error logs an error message
func Error(v ...interface{}) {
	logf(ERROR, "%s", fmt.Sprint(v...))
}

// Errorf logs a formatted error message
func Errorf(format string, v ...interface{}) {
	logf(ERROR, format, v...)
}

// Fatal logs a fatal message and exits
func Fatal(v ...interface{}) {
	logf(FATAL, "%s", fmt.Sprint(v...))
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(format string, v ...interface{}) {
	logf(FATAL, format, v...)
}
