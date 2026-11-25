//go:build service_bundle && linux && amd64

package service

import _ "embed"

//go:embed blobs/linux-amd64.tar.gz
var bundled []byte

//go:embed blobs/linux-amd64.sha256
var bundledSum string
