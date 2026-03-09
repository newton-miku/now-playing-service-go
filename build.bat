@echo off
chcp 65001 >nul

REM Windows 构建脚本
REM 用法: build.bat [386|amd64] [--package]
REM   --package: 构建后自动打包成 zip


setlocal enabledelayedexpansion

set "GOARCH=%~1"
set "PACKAGE=0"

if "%GOARCH%"=="--package" (
    set "GOARCH=amd64"
    set "PACKAGE=1"
)
if "%GOARCH%"=="" set "GOARCH=amd64"
if "%~2"=="--package" set "PACKAGE=1"

if not "%GOARCH%"=="386" if not "%GOARCH%"=="amd64" (
    echo Error: Invalid architecture %GOARCH%
    echo Usage: build.bat [386^|amd64] [--package]
    exit /b 1
)

REM Get version info
for /f "delims=" %%i in ('git describe --tags 2^>nul') do set "VERSION=%%i"
if "%VERSION%"=="" set "VERSION=unknown version"

REM Get build time using PowerShell (more reliable in CI)
for /f "delims=" %%a in ('powershell -Command "Get-Date -Format 'yyyy-MM-dd HH:mm:ss'"') do set "BUILD_TIME=%%a"

echo Build Configuration:
echo   Architecture: %GOARCH%
echo   Version: %VERSION%
echo   Build Time: %BUILD_TIME%
if %PACKAGE% EQU 1 echo   Package: Yes
echo.

REM Check GCC compiler (required for CGO)
set "GCC_AVAILABLE=0"
where /q gcc
if %ERRORLEVEL% EQU 0 (
    set "GCC_AVAILABLE=1"
    echo Found GCC compiler, CGO enabled
) else (
    echo Error: GCC compiler not found!
    echo Please install MinGW-w64 or TDM-GCC
    echo Download: https://jmeubank.github.io/tdm-gcc/
    exit /b 1
)

echo.
echo Starting build...
set "LDFLAGS=-X \"github.com/newton-miku/now-playing-service-go/tools.Version=%VERSION%\" -X \"github.com/newton-miku/now-playing-service-go/tools.BuildTime=%BUILD_TIME%\" -w -s -H=windowsgui"

set "GOOS=windows"
set "CGO_ENABLED=1"
go build -trimpath -ldflags "%LDFLAGS%" -o bin\now-playing-windows-%GOARCH%.exe

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo Build failed!
    exit /b 1
)

echo.
echo Build successful!
echo Output: bin\now-playing-windows-%GOARCH%.exe
echo.
echo Note: Ensure ico\icon.ico exists when running for tray icon

if %PACKAGE% NEQ 1 goto end

echo.
echo Packaging...

REM Create package directory
set "PKG_DIR=bin\now-playing-windows-%GOARCH%"
if exist "%PKG_DIR%" rmdir /s /q "%PKG_DIR%"
mkdir "%PKG_DIR%"

REM Copy executable
copy "bin\now-playing-windows-%GOARCH%.exe" "%PKG_DIR%\" >nul

REM Copy config directory if exists
if exist "config" (
    xcopy /e /i /y "config" "%PKG_DIR%\config" >nul
)

REM Copy web directory if exists
if exist "web" (
    xcopy /e /i /y "web" "%PKG_DIR%\web" >nul
)

REM Copy ico directory if exists
if exist "ico" (
    xcopy /e /i /y "ico" "%PKG_DIR%\ico" >nul
)

REM Create zip - try multiple methods
set "ZIP_NAME=bin\now-playing-windows-%GOARCH%-%VERSION%.zip"
if exist "%ZIP_NAME%" del "%ZIP_NAME%"

REM Try 7z first
where /q 7z
if %ERRORLEVEL% EQU 0 (
    echo Using 7z...
    cd bin
    7z a -tzip "now-playing-windows-%GOARCH%-%VERSION%.zip" "now-playing-windows-%GOARCH%" >nul
    cd ..
    goto package_done
)

REM Try PowerShell
where /q powershell
if %ERRORLEVEL% EQU 0 (
    echo Using PowerShell...
    powershell -Command "Compress-Archive -Path 'bin\now-playing-windows-%GOARCH%' -DestinationPath 'bin\now-playing-windows-%GOARCH%-%VERSION%.zip' -Force" >nul
    goto package_done
)

echo Warning: Could not create zip package (7z or PowerShell required)
goto cleanup

:package_done
echo Package created: %ZIP_NAME%

:cleanup
REM Clean up package directory
rmdir /s /q "%PKG_DIR%" 2>nul

:end
endlocal
