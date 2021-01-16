// +build windows

package nginxconf

import (
	"path/filepath"

	which "github.com/hairyhenderson/go-which"
)

func init() {
	// "nginx/Windows uses the directory where it has been run as the prefix for relative paths in the configuration."
	// Source: https://nginx.org/en/docs/windows.html
	// However, the "conf/" directory, where some of the standard snippets live, is neighboring the nginx.exe. Therefore,
	// it's more likely to hit a match in this root than elsewhere.
	nginxPath := which.Which("nginx.exe")
	nginxConfPrefix = filepath.Dir(nginxPath)
	for k, v := range nginxConfDirs {
		if v == "conf.d/" {
			nginxConfDirs[k] = "conf/"
		}
	}
}
