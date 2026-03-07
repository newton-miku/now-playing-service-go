// Package utils provides common utility functions
package utils

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/newton-miku/now-playing-service-go/music"
)

// OpenBrowser opens the web UI in the default browser
func OpenBrowser(port string) {
	url := fmt.Sprintf("http://localhost:%s", port)

	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = exec.Command("xdg-open", url).Start()
	}

	if err != nil {
		fmt.Printf("无法打开浏览器: %v\n", err)
	}
}

// PrintStatus prints the music status to console
func PrintStatus(status *music.Status) {
	if status.Status != "None" {
		fmt.Printf("%s\n%s - %s\n", status.Status, status.Title, status.Artist)
	} else {
		fmt.Println("None")
	}
}
