//go:build service_bundle && windows && arm64

package service

import _ "embed"

//go:embed blobs/windows-arm64.tar.gz
var bundled []byte

//go:embed blobs/windows-arm64.sha256
var bundledSum string
