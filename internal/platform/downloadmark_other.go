//go:build !darwin && !linux && !windows

package platform

func keepDownloadMark(from, to string) error { return nil }
