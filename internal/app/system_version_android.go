//go:build android

package app

import (
	"os/exec"
	"strings"
	"sync"
)

var systemVersion = sync.OnceValue(func() string {
	release, err := exec.Command("/system/bin/getprop", "ro.build.version.release").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(release))
})
