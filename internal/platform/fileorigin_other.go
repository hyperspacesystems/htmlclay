//go:build !darwin && !linux && !windows

package platform

import (
	"os"
	"time"
)

func downloaded(path string) bool { return false }

func changeTime(info os.FileInfo) (time.Time, bool) { return time.Time{}, false }
