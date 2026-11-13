//go:build service_bundle && windows && amd64

package service

import _ "embed"

//go:embed blobs/windows-amd64.tar.gz
var bundled []byte

//go:embed blobs/windows-amd64.sha256
var bundledSum string
