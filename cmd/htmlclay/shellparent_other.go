//go:build !darwin

package main

import "sync/atomic"

var shellParentWatch atomic.Bool

func exitWithParent() {}
