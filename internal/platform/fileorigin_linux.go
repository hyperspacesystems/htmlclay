//go:build linux

package platform

import (
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// downloaded reads the origin a browser recorded when it saved the file. Only
// an internet origin counts: a file copied from another folder on this machine
// or from a page served here carries a mark too, and nobody downloaded it.
func downloaded(path string) bool {
	mark, err := readXattr(path, downloadMarkAttr)
	if err != nil || mark == nil {
		return false
	}
	return internetOrigin(string(mark))
}

func internetOrigin(mark string) bool {
	u, err := url.Parse(strings.TrimSpace(mark))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return false
	}
	return true
}

// createTime is unknown here: Linux keeps no birth time in the stat every
// filesystem answers, so the modification time is all there is.
func createTime(os.FileInfo) (time.Time, bool) { return time.Time{}, false }

func changeTime(info os.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Ctim.Unix()), true
}
