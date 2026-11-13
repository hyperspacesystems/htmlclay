//go:build service_bundle && darwin && arm64

package service

import _ "embed"

//go:embed blobs/darwin-arm64.tar.gz
var bundled []byte

//go:embed blobs/darwin-arm64.sha256
var bundledSum string
