# Now Playing Service

一个用于监控 Windows 媒体播放状态并支持上报到设备状态服务器的 Go 语言服务。

## 功能特性

- 🎵 **多平台音乐监控** - 支持网易云音乐、QQ音乐、酷狗音乐、Spotify 等多种播放器
- 🖥️ **SMTC 支持** - 优先使用系统媒体传输控件获取播放信息
- 📊 **前台窗口检测** - 实时获取当前活动窗口信息
- 🌐 **HTTP API** - 提供 RESTful API 供外部调用
- 📝 **日志系统** - 完整的日志记录和实时查看功能
- 🎨 **Web 界面** - 内置美观的 Web UI，支持中英文切换
- 📤 **设备状态上报** - 支持将状态上报到设备服务器
- 🔔 **系统托盘** - 最小化到系统托盘，不占用任务栏
- ⚙️ **灵活配置** - 支持配置文件和命令行参数

## 安装

### 从源码构建

**要求:**
- Go 1.21 或更高版本
- GCC (MinGW-w64 或 TDM-GCC) - Windows 平台需要

**步骤:**

```bash
# 克隆仓库
git clone https://github.com/newton-miku/now-playing-service-go.git
cd now-playing-service-go

# 构建 (Windows)
build.bat amd64

# 或使用 go 命令构建
go build -ldflags "-H=windowsgui" -o now-playing-service-go.exe
```

### 下载预编译版本

从 [GitHub Releases](https://github.com/newton-miku/now-playing-service-go/releases) 下载最新版本。

## 使用方法

### 基本运行

```bash
# 直接运行
now-playing-service-go.exe

# 指定端口
now-playing-service-go.exe -port 8080

# 无托盘模式 (调试用)
now-playing-service-go.exe -no-tray
```

### 命令行参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `-port` | HTTP 服务器端口 | `-port 8080` |
| `-preferred` | 首选音乐平台 | `-preferred netease` |
| `-smtc` | 优先使用 SMTC | `-smtc true` |
| `-no-tray` | 禁用系统托盘 | `-no-tray` |
| `-report-server` | 上报服务器地址 | `-report-server http://localhost:21081` |
| `-report-name` | 设备名称 | `-report-name 我的电脑` |
| `-save` | 保存设置到配置文件 | `-save` |

### 配置文件

配置文件位于 `config/settings.json`，首次运行会自动创建。

```json
{
  "preferred_platform": "netease",
  "port": "8080",
  "check_interval_ms": 100,
  "auto_open_browser": true,
  "auto_start": false,
  "smtc_preferred": true,
  "enable_report": false,
  "report_server_url": "",
  "report_interval_ms": 5000,
  "report_device_id": "",
  "report_device_name": "",
  "report_api_key": "",
  "log_level": 1
}
```

## API 接口

### 获取当前状态

```http
GET /api/status
```

响应:
```json
{
  "music": {
    "status": "Playing",
    "title": "歌曲名",
    "artist": "艺术家",
    "album": "专辑名",
    "process_name": "cloudmusic"
  },
  "foreground": {
    "title": "窗口标题",
    "process_name": "chrome.exe",
    "process_id": 12345
  },
  "timestamp": 1704067200
}
```

### 获取支持的平台列表

```http
GET /api/platforms
```

### 获取日志

```http
GET /api/logs/stream
```

SSE 实时日志流

### 获取版本信息

```http
GET /api/version
```

## 支持的播放器

- 网易云音乐 (netease)
- QQ音乐 (qqmusic)
- 酷狗音乐 (kugou)
- 酷我音乐 (kuwo)
- Spotify
- Apple Music
- foobar2000
- PotPlayer
- AIMP
- 洛雪音乐 (lxmusic)

## 项目结构

```
now-playing-service-go/
├── main.go              # 程序入口
├── build.bat            # Windows 构建脚本
├── config/              # 配置文件目录
│   └── settings.json
├── web/                 # Web 界面
│   ├── index.html
│   ├── css/
│   │   └── style.css
│   └── js/
│       ├── i18n.js
│       └── app.js
├── client/              # 设备上报客户端
├── foreground/          # 前台窗口检测
├── logger/              # 日志系统
├── music/               # 音乐状态检测
├── server/              # HTTP 服务器
├── settings/            # 配置管理
├── tray/                # 系统托盘
├── utils/               # 工具函数
└── webview/             # WebView 窗口
```

## 开发

### 运行测试

```bash
go test ./...
```

### 代码格式化

```bash
go fmt ./...
```

## 许可证

MIT License

## 致谢

- [webview/webview_go](https://github.com/webview/webview_go) - WebView2 绑定
- [getlantern/systray](https://github.com/getlantern/systray) - 系统托盘支持
