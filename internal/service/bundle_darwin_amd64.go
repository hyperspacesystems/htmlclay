//go:build service_bundle && darwin && amd64

package service

import _ "embed"

//go:embed blobs/darwin-amd64.tar.gz
var bundled []byte

//go:embed blobs/darwin-amd64.sha256
var bundledSum string
