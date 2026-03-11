// Package music provides music playback detection functionality
// Supports SMTC (System Media Transport Controls) and window title detection
package music

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/media/control"

	"github.com/newton-miku/now-playing-service-go/audio"
	"github.com/newton-miku/now-playing-service-go/foreground"
	"github.com/newton-miku/now-playing-service-go/logger"
)

// Status represents the current music playback status
type Status struct {
	Status      string `json:"status"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	ProcessName string `json:"process_name"`
}

// StatusWithMethod includes detection method information
type StatusWithMethod struct {
	Status
	Method           string   `json:"method"`
	MethodDesc       string   `json:"method_desc"`
	AvailableMethods []string `json:"available_methods"`
}

// Detection methods
const (
	MethodSMTC        = "smtc"
	MethodWindowTitle = "window_title"
	MethodNone        = "none"
)

var (
	MethodSMTCDesc        = "SMTC (支持 Playing/Paused)"
	MethodWindowTitleDesc = "窗口标题+PDH音频检测 (支持 Playing/Paused)"
)

// AppIDMappings maps common SMTC AppIDs to friendly names
var AppIDMappings = map[string]string{
	"QQMusic":                  "QQ音乐",
	"CloudMusic":               "网易云音乐",
	"Kugou":                    "酷狗音乐",
	"Kuwo":                     "酷我音乐",
	"Spotify":                  "Spotify",
	"Music":                    "QQ音乐",
	"Microsoft.ZuneMusic":      "Windows Media Player",
	"Microsoft.Windows.Photos": "Photos",
	"Chrome":                   "Chrome",
	"msedge":                   "Edge",
}

// PlatformConfig defines detection rules for a music platform
type PlatformConfig struct {
	ProcessNames []string
	TitleParts   []string
}

// PlatformConfigs contains configurations for different music platforms
var PlatformConfigs = map[string]PlatformConfig{
	"netease": {
		ProcessNames: []string{"cloudmusic", "CloudMusic"},
		TitleParts:   []string{" - 网易云音乐", " - CloudMusic"},
	},
	"qq": {
		ProcessNames: []string{"QQMusic", "qqmusic", "QQMusic.exe"},
		TitleParts:   []string{"QQ音乐", " - QQ音乐", "QQ音乐 - "},
	},
	"kugou": {
		ProcessNames: []string{"Kugou", "kugou"},
		TitleParts:   []string{" - 酷狗音乐"},
	},
	"kuwo": {
		ProcessNames: []string{"Kuwo", "kuwo"},
		TitleParts:   []string{" - 酷我音乐"},
	},
	"spotify": {
		ProcessNames: []string{"Spotify"},
		TitleParts:   []string{" — Spotify"},
	},
	"apple": {
		ProcessNames: []string{"AppleMusic", "Music"},
		TitleParts:   []string{" – Apple Music"},
	},
	"foobar": {
		ProcessNames: []string{"foobar2000"},
		TitleParts:   []string{""},
	},
	"potplayer": {
		ProcessNames: []string{"PotPlayer", "PotPlayerMini64", "PotPlayerMini"},
		TitleParts:   []string{""},
	},
	"aimp": {
		ProcessNames: []string{"AIMP", "AIMP2"},
		TitleParts:   []string{""},
	},
	"lx": {
		ProcessNames: []string{"lxmusic", "LxMusic"},
		TitleParts:   []string{" - 洛雪音乐"},
	},
}

func convertPlaybackStatus(smtcStatus control.GlobalSystemMediaTransportControlsSessionPlaybackStatus) string {
	switch smtcStatus {
	case control.GlobalSystemMediaTransportControlsSessionPlaybackStatusPlaying:
		return "Playing"
	case control.GlobalSystemMediaTransportControlsSessionPlaybackStatusPaused:
		return "Paused"
	case control.GlobalSystemMediaTransportControlsSessionPlaybackStatusStopped:
		return "Stopped"
	case control.GlobalSystemMediaTransportControlsSessionPlaybackStatusClosed:
		return "None"
	case control.GlobalSystemMediaTransportControlsSessionPlaybackStatusOpened:
		return "Opened"
	default:
		return "None"
	}
}

func waitForAsyncOperation(asyncOp *foundation.IAsyncOperation, timeout time.Duration) (unsafe.Pointer, error) {
	deadline := time.Now().Add(timeout)

	// 尝试多次获取结果，而不是检查completed handler
	for time.Now().Before(deadline) {
		// 直接尝试获取结果
		results, err := asyncOp.GetResults()
		if err == nil && results != nil {
			return results, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout after %v", timeout)
}

// GetSMTCStatus gets music status using SMTC (System Media Transport Controls)
func GetSMTCStatus() *Status {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf("PANIC in GetSMTCStatus: %v", rec)
		}
	}()

	logger.Debug("GetSMTCStatus: Starting...")

	// 尝试初始化为单线程单元（STA）- WinRT通常需要STA
	// 如果已经初始化过，会返回RPC_E_CHANGED_MODE错误，我们忽略它
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)

	asyncOp, err := control.GlobalSystemMediaTransportControlsSessionManagerRequestAsync()
	if err != nil {
		if strings.Contains(err.Error(), "0x80010106") { // RPC_E_CHANGED_MODE
			logger.Debug("GetSMTCStatus: Already initialized with different mode")
		} else {
			logger.Debugf("GetSMTCStatus: RequestAsync failed: %v", err)
			return nil
		}
	}
	if asyncOp == nil {
		logger.Debug("GetSMTCStatus: asyncOp is nil")
		return nil
	}
	defer asyncOp.Release()

	logger.Debug("GetSMTCStatus: Waiting for async operation...")
	results, err := waitForAsyncOperation(asyncOp, 5*time.Second)
	if err != nil {
		logger.Debugf("GetSMTCStatus: waitForAsyncOperation failed: %v", err)
		return nil
	}
	logger.Debug("GetSMTCStatus: Async operation completed")

	manager := (*control.GlobalSystemMediaTransportControlsSessionManager)(results)
	if manager == nil {
		logger.Debug("GetSMTCStatus: manager is nil")
		return nil
	}
	defer manager.Release()

	// Get all sessions
	sessions, err := manager.GetSessions()
	if err != nil {
		logger.Debugf("GetSMTCStatus: GetSessions failed: %v", err)
		return nil
	}
	if sessions == nil {
		logger.Debug("GetSMTCStatus: sessions is nil")
		return nil
	}
	defer sessions.Release()

	size, _ := sessions.GetSize()
	logger.Debugf("GetSMTCStatus: Found %d sessions", size)
	if size == 0 {
		logger.Debug("GetSMTCStatus: No sessions")
		return nil
	}

	var pausedCandidate *Status
	var firstCandidate *Status

	// 遍历所有会话，优先选择正在播放的
	for i := uint32(0); i < size; i++ {
		sessionPtr, err := sessions.GetAt(i)
		if err != nil || sessionPtr == nil {
			logger.Debugf("GetSMTCStatus: Session %d GetAt failed: %v", i, err)
			continue
		}
		session := (*control.GlobalSystemMediaTransportControlsSession)(sessionPtr)

		status := handleSession(session)
		session.Release()

		if status != nil {
			logger.Debugf("GetSMTCStatus: Session %d: Status=%s, Title=%s, Artist=%s, Process=%s",
				i, status.Status, status.Title, status.Artist, status.ProcessName)

			// 找到正在播放的，直接返回（即使标题为空也返回，只要状态是Playing）
			if status.Status == "Playing" {
				logger.Debug("GetSMTCStatus: Found playing session, returning it")
				return status
			}

			// 保存暂停的候选（即使标题为空）
			if status.Status == "Paused" && pausedCandidate == nil {
				pausedCandidate = status
			}

			// 保存第一个候选
			if firstCandidate == nil {
				firstCandidate = status
			}
		} else {
			logger.Debugf("GetSMTCStatus: Session %d: handleSession returned nil", i)
		}
	}

	// 优先返回暂停的有内容的会话
	if pausedCandidate != nil {
		logger.Debug("GetSMTCStatus: Returning paused candidate")
		return pausedCandidate
	}

	// 其次返回第一个有内容的会话
	if firstCandidate != nil {
		logger.Debug("GetSMTCStatus: Returning first candidate")
		return firstCandidate
	}

	logger.Debug("GetSMTCStatus: No valid sessions found")
	return nil
}

func handleSession(session *control.GlobalSystemMediaTransportControlsSession) *Status {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf("PANIC in handleSession: %v", rec)
		}
	}()

	playbackInfo, err := session.GetPlaybackInfo()
	if err != nil || playbackInfo == nil {
		logger.Debugf("handleSession: GetPlaybackInfo failed: %v", err)
		return nil
	}
	defer playbackInfo.Release()

	playbackStatus, _ := playbackInfo.GetPlaybackStatus()
	statusStr := convertPlaybackStatus(playbackStatus)
	sourceApp, _ := session.GetSourceAppUserModelId()

	logger.Debugf("handleSession: playbackStatus=%d (%s), sourceApp=%s", playbackStatus, statusStr, sourceApp)

	if statusStr == "None" || statusStr == "Stopped" {
		logger.Debugf("handleSession: Status %s, skipping", statusStr)
		return nil
	}

	mediaAsyncOp, _ := session.TryGetMediaPropertiesAsync()
	if mediaAsyncOp == nil {
		logger.Debug("handleSession: mediaAsyncOp is nil")
		return &Status{Status: statusStr, ProcessName: simplifyAppName(sourceApp)}
	}
	defer mediaAsyncOp.Release()

	mediaResults, err := waitForAsyncOperation(mediaAsyncOp, 5*time.Second)
	if err != nil {
		logger.Debugf("handleSession: waitForAsyncOperation for media failed: %v", err)
		return &Status{Status: statusStr, ProcessName: simplifyAppName(sourceApp)}
	}

	mediaProps := (*control.GlobalSystemMediaTransportControlsSessionMediaProperties)(mediaResults)
	if mediaProps == nil {
		logger.Debug("handleSession: mediaProps is nil")
		return &Status{Status: statusStr, ProcessName: simplifyAppName(sourceApp)}
	}
	defer mediaProps.Release()

	title, _ := mediaProps.GetTitle()
	artist, _ := mediaProps.GetArtist()
	album, _ := mediaProps.GetAlbumTitle()

	logger.Debugf("handleSession: Title=%s, Artist=%s, Album=%s", title, artist, album)

	// 移除对暂停状态空标题的过滤，让更多会话能被检测到
	if title == "" && statusStr == "Paused" {
		logger.Debug("handleSession: Paused with empty title, skipping")
		return nil
	}

	friendlyName := simplifyAppName(sourceApp)
	// logger.Debugf("handleSession: Simplified app name from %s to %s", sourceApp, friendlyName)
	// 如果简化后的名称还是很长，尝试使用映射表
	// 注意：先检查更长/更具体的键，避免 "Music" 匹配到 "CloudMusic"
	for _, id := range []string{
		"QQMusic", "CloudMusic", "Kugou", "Kuwo", "Spotify",
		"Microsoft.ZuneMusic", "Microsoft.Windows.Photos", "Chrome", "msedge",
		"Music", // 最后检查这个，因为它比较短
	} {
		name := AppIDMappings[id]
		if strings.Contains(strings.ToLower(friendlyName), strings.ToLower(id)) {
			friendlyName = name
			break
		}
	}

	return &Status{
		Status:      statusStr,
		Title:       title,
		Artist:      artist,
		Album:       album,
		ProcessName: friendlyName,
	}
}

// simplifyAppName 简化应用名称显示（参考Python实现）
func simplifyAppName(appName string) string {
	if appName == "" {
		return "未知应用"
	}

	// 参考Python实现：如果包含 "!"，只取前面的部分
	if idx := strings.Index(appName, "!"); idx != -1 {
		appName = appName[:idx]
	}

	return appName
}

func checkWindowsForPlatform(windows []*foreground.WindowInfo, config PlatformConfig, platformName string) *Status {
	var bestStatus *Status

	for _, win := range windows {
		lowTitle := strings.ToLower(win.Title)
		// 基础过滤 - 过滤掉各种非主窗口
		if strings.Contains(lowTitle, "default ime") ||
			strings.Contains(lowTitle, "msctfime ui") ||
			strings.Contains(lowTitle, "dummy window") ||
			strings.Contains(lowTitle, "gdi+ window") ||
			strings.Contains(lowTitle, "lyric") || // 过滤包含 "lyric" 的窗口（歌词窗口）
			strings.Contains(lowTitle, "歌词") || // 过滤包含 "歌词" 的窗口
			strings.Contains(lowTitle, "桌面歌词") || // 过滤桌面歌词窗口
			strings.Contains(lowTitle, "desktiplyric") ||
			win.Title == "TXMenuWindow" ||
			win.Title == "DynamicLyricWindow" ||
			win.Title == "LyricWindow" ||
			win.Title == "歌词" ||
			(len(win.Title) > 0 && len(win.Title) < 3 && !strings.Contains(win.Title, " - ")) { // 过滤过短且不包含 " - " 的标题
			continue
		}

		winProcClean := strings.ToLower(win.ProcessName)
		if idx := strings.LastIndex(winProcClean, "."); idx != -1 {
			winProcClean = winProcClean[:idx]
		}

		for _, procName := range config.ProcessNames {
			procNameClean := strings.ToLower(procName)
			if idx := strings.LastIndex(procNameClean, "."); idx != -1 {
				procNameClean = procNameClean[:idx]
			}

			if winProcClean == procNameClean {
				title := win.Title
				for _, suffix := range config.TitleParts {
					title = strings.TrimSuffix(title, suffix)
				}

				if title != "" && title != win.ProcessName {
					var artist, songTitle string
					if strings.Contains(title, " - ") {
						parts := strings.SplitN(title, " - ", 2)
						songTitle = strings.TrimSpace(parts[0])
						artist = strings.TrimSpace(parts[1])
					} else {
						songTitle = strings.TrimSpace(title)
					}

					for _, suffix := range config.TitleParts {
						songTitle = strings.TrimSuffix(songTitle, strings.TrimSpace(suffix))
						artist = strings.TrimSuffix(artist, strings.TrimSpace(suffix))
					}
					songTitle = strings.TrimSpace(songTitle)
					artist = strings.TrimSpace(artist)

					if songTitle == "QQ音乐" || songTitle == "播放队列" || songTitle == "已开始播放提示" ||
						songTitle == "网易云音乐" || songTitle == "CloudMusic" ||
						songTitle == "歌词" || songTitle == "Lyric" ||
						strings.Contains(songTitle, "桌面") ||
						(len(songTitle) < 2 && songTitle != "-") || // 过滤太短的标题
						strings.HasPrefix(songTitle, " ") || // 过滤以空格开头的标题
						strings.HasSuffix(songTitle, " ") { // 过滤以空格结尾的标题
						continue
					}

					status := "Playing"
					if win.ProcessID > 0 {
						if hasAudio := audio.IsAudioProcess(win.ProcessName, uint32(win.ProcessID)); !hasAudio {
							if hasAudio = audio.HasAudioOutput(uint32(win.ProcessID)); !hasAudio {
								status = "Paused"
							}
						}
					}

					displayName := procName
					for id, name := range AppIDMappings {
						if strings.EqualFold(procNameClean, strings.ToLower(id)) {
							displayName = name
							break
						}
					}

					currentStatus := &Status{
						Status:      status,
						Title:       songTitle,
						Artist:      artist,
						ProcessName: displayName,
					}

					if status == "Playing" {
						return currentStatus
					}
					if bestStatus == nil {
						bestStatus = currentStatus
					}
				}
			}
		}
	}
	return bestStatus
}

// GetWindowTitleStatus gets music status using window title detection
func GetWindowTitleStatus(platform string) *Status {
	config := PlatformConfigs[platform]
	if config.ProcessNames == nil {
		config = PlatformConfigs["netease"]
	}
	windows := foreground.GetWindowsWithTitles()
	return checkWindowsForPlatform(windows, config, platform)
}

// GetStatusWithMethod returns music status along with detection method information
func GetStatusWithMethod(platform string) *StatusWithMethod {
	return GetStatusWithMethodSMTCPreferred(platform, true)
}

// GetStatusWithMethodSMTCPreferred returns music status with SMTC preference option
func GetStatusWithMethodSMTCPreferred(platform string, smtcPreferred bool) *StatusWithMethod {
	result := &StatusWithMethod{
		AvailableMethods: []string{MethodSMTC, MethodWindowTitle},
	}

	// If SMTC is preferred, check it first
	if smtcPreferred {
		if status := GetSMTCStatus(); status != nil {
			result.Status = *status
			result.Method = MethodSMTC
			result.MethodDesc = MethodSMTCDesc
			return result
		}
	}

	// Check window title method
	if status := GetWindowTitleStatus(platform); status != nil {
		result.Status = *status
		result.Method = MethodWindowTitle
		result.MethodDesc = MethodWindowTitleDesc
		return result
	}

	// If SMTC is preferred and we haven't checked it yet (window title failed), check SMTC
	if smtcPreferred {
		if status := GetSMTCStatus(); status != nil {
			result.Status = *status
			result.Method = MethodSMTC
			result.MethodDesc = MethodSMTCDesc
			return result
		}
	}

	result.Status.Status = "None"
	result.Method = MethodNone
	result.MethodDesc = "No music detected"

	return result
}

// GetStatus returns music status (convenience function)
func GetStatus(platform string) *Status {
	result := GetStatusWithMethod(platform)
	return &result.Status
}

// GetGlobalStatus detects music status across all platforms
func GetGlobalStatus(preferredPlatform string) *StatusWithMethod {
	return GetGlobalStatusSMTCPreferred(preferredPlatform, true)
}

// GetGlobalStatusSMTCPreferred detects music status with SMTC preference option
func GetGlobalStatusSMTCPreferred(preferredPlatform string, smtcPreferred bool) *StatusWithMethod {
	return GetStatusWithMethodSMTCPreferred(preferredPlatform, smtcPreferred)
}

// GetPlatforms returns list of supported platforms
func GetPlatforms() []string {
	platforms := make([]string, 0, len(PlatformConfigs))
	for p := range PlatformConfigs {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	return platforms
}
