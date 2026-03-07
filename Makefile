# ==============================================================================
# 项目基础配置 - Windows 客户端
# ==============================================================================
# 项目名称
NAME = now-playing
# 输出目录
BINDIR = bin
# 配置目录
CONF_DIR = config
# 静态资源目录
ASSET_DIR = web
# 图标目录
ICO_DIR = ico

# 日期命令 - 跨平台兼容
BUILD_TIME ?= $(shell TZ=UTC-8 date +%Y-%m-%d\ %H:%M:%S 2>/dev/null || TZ=Asia/Shanghai date +%Y-%m-%d\ %H:%M:%S 2>/dev/null || date +%Y-%m-%d\ %H:%M:%S)
DATE_CMD = date +%Y%m%d

# 版本号生成（使用跨平台日期命令）
VERSION=$(shell git describe --tags 2>/dev/null || echo "unknown version")

# Windows 构建（需要 CGO 支持 webview）
GOBUILD_WINDOWS=CGO_ENABLED=1 go build -trimpath -ldflags '-X "github.com/newton-miku/now-playing-service-go/tools.Version=$(VERSION)" -X "github.com/newton-miku/now-playing-service-go/tools.BuildTime=$(BUILD_TIME)" -w -s -H=windowsgui'

# Windows 图标资源（可选，EXE 图标）
RC_FILE=main.rc
SYSO_FILE_386=main_windows_386.syso
SYSO_FILE_AMD64=main_windows_amd64.syso

# 检测可用的 windres 命令
WINDRES_386=$(shell which i686-w64-mingw32-windres 2>/dev/null || which windres 2>/dev/null || echo windres)
WINDRES_AMD64=$(shell which x86_64-w64-mingw32-windres 2>/dev/null || which windres 2>/dev/null || echo windres)


# ==============================================================================
# 平台配置
# ==============================================================================
WINDOWS_ARCH_LIST = \
	windows-386 \
	windows-amd64

.PHONY: all windows-386 windows-amd64 all-arch releases clean

all: windows-amd64

# Windows 图标资源编译（可选）
$(SYSO_FILE_386): $(RC_FILE)
	$(WINDRES_386) -i $(RC_FILE) -o $@ -O coff -F pe-i386 2>/dev/null || rm -f $@

$(SYSO_FILE_AMD64): $(RC_FILE)
	$(WINDRES_AMD64) -i $(RC_FILE) -o $@ -O coff -F pe-x86-64 2>/dev/null || rm -f $@

# 优先尝试带图标，失败则不带图标构建
windows-386:
	-$(MAKE) $(SYSO_FILE_386) 2>/dev/null || true
	GOARCH=386 GOOS=windows $(GOBUILD_WINDOWS) -o $(BINDIR)/$(NAME)-$@.exe
	rm -f $(SYSO_FILE_386) 2>/dev/null || true

windows-amd64:
	-$(MAKE) $(SYSO_FILE_AMD64) 2>/dev/null || true
	GOARCH=amd64 GOOS=windows $(GOBUILD_WINDOWS) -o $(BINDIR)/$(NAME)-$@.exe
	rm -f $(SYSO_FILE_AMD64) 2>/dev/null || true

zip_releases=$(addsuffix .zip, $(WINDOWS_ARCH_LIST))

# zip 打包规则 - 包含配置和资源目录
$(zip_releases): %.zip : %
	# 创建临时目录用于打包
	mkdir -p $(BINDIR)/release/$(NAME)-$(basename $@)
	mv $(BINDIR)/$(NAME)-$(basename $@).exe $(BINDIR)/release/$(NAME)-$(basename $@)/
	# 复制资源文件
	-cp -r $(CONF_DIR) $(BINDIR)/release/$(NAME)-$(basename $@)/ 2>/dev/null || true
	-cp -r $(ASSET_DIR) $(BINDIR)/release/$(NAME)-$(basename $@)/ 2>/dev/null || true
	-cp -r $(ICO_DIR) $(BINDIR)/release/$(NAME)-$(basename $@)/ 2>/dev/null || true
	# 创建压缩包（尝试多种方法）
	cd $(BINDIR)/release && ( \
		(which 7z 2>/dev/null && 7z a -tzip ../$(NAME)-$(basename $@)-$(VERSION).zip $(NAME)-$(basename $@)) || \
		(which powershell 2>/dev/null && powershell -Command "Compress-Archive -Path $(NAME)-$(basename $@) -DestinationPath ../$(NAME)-$(basename $@)-$(VERSION).zip -Force") || \
		(which zip 2>/dev/null && zip -r ../$(NAME)-$(basename $@)-$(VERSION).zip $(NAME)-$(basename $@)) \
	)
	# 清理临时文件
	rm -rf $(BINDIR)/release

all-arch: $(WINDOWS_ARCH_LIST)

releases: $(zip_releases)

clean:
	rm -rf $(BINDIR)/*
	rm -f $(SYSO_FILE_386) $(SYSO_FILE_AMD64) 2>/dev/null || true
