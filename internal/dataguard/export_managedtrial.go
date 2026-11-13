//go:build managedtrial

package dataguard

import "github.com/panphora/htmlclay/internal/dataapi"

// IslandHash is the pinned recoverable-data digest a guard store writes for an
// island, the one Local's islandHash produces.
func IslandHash(island dataapi.Value) string { return islandHash(island) }

// JSStringTrim is String.prototype.trim over the same whitespace set the JS
// hosts use, which is wider than Go's unicode.IsSpace.
func JSStringTrim(s string) string { return jsTrim(s) }
