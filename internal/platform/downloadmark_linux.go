//go:build linux

package platform

import "golang.org/x/sys/unix"

const downloadMarkAttr = "user.xdg.origin.url"

var noXattr = unix.ENODATA
