//go:build darwin

package platform

import "golang.org/x/sys/unix"

const downloadMarkAttr = "com.apple.quarantine"

var noXattr = unix.ENOATTR
