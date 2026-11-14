package dataguard

import "github.com/panphora/htmlclay/internal/dataapi"

// IslandHash is the pinned recoverable-data digest a guard store writes for an
// island, the one Local's islandHash produces.
func IslandHash(island dataapi.Value) string { return islandHash(island) }

// JSStringTrim is String.prototype.trim over the same whitespace set the JS
// hosts use, which is wider than Go's unicode.IsSpace.
func JSStringTrim(s string) string { return jsTrim(s) }

// UnionTyped merges newly lost typed values into a pending event's rows, for a
// host that keeps its own record.
func UnionTyped(have, add []TypedValue) []TypedValue { return unionTyped(have, add) }

// TypedRows is the newest typed values a loss took, capped for the panel.
func TypedRows(typed []TypedValue) []TypedRow { return typedRows(typed) }

// CanPutBack reports whether at least one typed value can still be written back
// into a place the current page kept.
func CanPutBack(typed []TypedValue, current, recoverable dataapi.Value) bool {
	return canPutBack(typed, current, recoverable)
}
