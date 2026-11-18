//go:build !windows && !linux

package platform

import "errors"

func readFileTypes() FileTypeState {
	return FileTypeState{Default: "unknown"}
}

func makeDefault(_, _ string) error { return errors.ErrUnsupported }

func stopOpening() error { return errors.ErrUnsupported }

func setHTMLOpenWith(_ string, _ bool) error { return errors.ErrUnsupported }
