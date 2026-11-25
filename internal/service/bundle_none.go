//go:build !service_bundle || !((darwin && (arm64 || amd64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package service

var bundled []byte

var bundledSum string
