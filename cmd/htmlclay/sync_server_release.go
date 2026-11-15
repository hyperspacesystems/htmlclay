//go:build !synctest

package main

// syncServerFromBuild is empty in every build but synctest's, so a shipped
// binary carries no way to point sync elsewhere.
func syncServerFromBuild() string { return "" }
