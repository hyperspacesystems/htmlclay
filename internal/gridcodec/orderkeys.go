package gridcodec

import (
	"fmt"
	"regexp"
	"strings"
)

const orderDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var validKey = regexp.MustCompile(`^[0-9A-Za-z]*[1-9A-Za-z]$`)

// IsKey reports whether key is a valid fractional order key: ^[0-9A-Za-z]*[1-9A-Za-z]$.
func IsKey(key string) bool {
	return validKey.MatchString(key)
}

// digit reads a[n] || "0": past the end a is "0".
func digit(s string, n int) byte {
	if n < len(s) {
		return s[n]
	}
	return '0'
}

func sliceFrom(s string, n int) string {
	if n > len(s) {
		return ""
	}
	return s[n:]
}

func digitIndex(c byte) int {
	return strings.IndexByte(orderDigits, c)
}

func midpoint(a string, b *string) string {
	if b != nil {
		n := 0
		for n < len(*b) && digit(a, n) == (*b)[n] {
			n++
		}
		if n > 0 {
			return (*b)[:n] + midpoint(sliceFrom(a, n), strp(sliceFrom(*b, n)))
		}
	}
	da := 0
	if a != "" {
		da = digitIndex(a[0])
	}
	db := len(orderDigits)
	if b != nil {
		db = -1
		if *b != "" {
			db = digitIndex((*b)[0])
		}
	}
	if db-da > 1 {
		return string(orderDigits[(da+db+1)/2])
	}
	if b != nil && len(*b) > 1 {
		return (*b)[:1]
	}
	return string(orderDigits[da]) + midpoint(sliceFrom(a, 1), nil)
}

// KeyBetween returns a key strictly between a and b, where nil means "no bound". It errors for
// an invalid key or a >= b (byte comparison; keys are ASCII).
func KeyBetween(a, b *string) (string, error) {
	if a != nil && !IsKey(*a) {
		return "", fmt.Errorf("grid codec: invalid order key %s", *a)
	}
	if b != nil && !IsKey(*b) {
		return "", fmt.Errorf("grid codec: invalid order key %s", *b)
	}
	if a != nil && b != nil && *a >= *b {
		return "", fmt.Errorf("grid codec: order key %s is not before %s", *a, *b)
	}
	start := ""
	if a != nil {
		start = *a
	}
	return midpoint(start, b), nil
}

// KeysBetween returns n keys between a and b, spread by halving.
func KeysBetween(a, b *string, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	if n == 1 {
		key, err := KeyBetween(a, b)
		if err != nil {
			return nil, err
		}
		return []string{key}, nil
	}
	mid, err := KeyBetween(a, b)
	if err != nil {
		return nil, err
	}
	half := n / 2
	left, err := KeysBetween(a, &mid, half)
	if err != nil {
		return nil, err
	}
	right, err := KeysBetween(&mid, b, n-half-1)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, n)
	keys = append(keys, left...)
	keys = append(keys, mid)
	keys = append(keys, right...)
	return keys, nil
}
