// Package gridcodec is the Go port of the grid codec's value rule and strict
// reader: the same bytes JavaScript accepts, the same model it reports and the
// same digits it prints.
package gridcodec

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// MaxDigits is the value rule's limit: a number with more significant digits
// stays text.
const MaxDigits = 15

var numberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func significantDigits(text string) int {
	s := strings.TrimPrefix(text, "-")
	s = strings.Replace(s, ".", "", 1)
	s = strings.TrimLeft(s, "0")
	s = strings.TrimRight(s, "0")
	return len(s)
}

// Typed applies the value rule to stored text. It returns nil (empty), bool
// (TRUE/FALSE), float64 (a number by the rule), or string.
func Typed(text string, forceText bool) any {
	if text == "" {
		return nil
	}
	if forceText {
		return text
	}
	if text == "TRUE" {
		return true
	}
	if text == "FALSE" {
		return false
	}
	if !numberPattern.MatchString(text) {
		return text
	}
	digits := significantDigits(text)
	if digits > MaxDigits {
		return text
	}
	number, _ := strconv.ParseFloat(text, 64)
	if math.IsNaN(number) || math.IsInf(number, 0) || (number == 0 && digits > 0) {
		return text
	}
	if number == 0 {
		return float64(0)
	}
	return number
}

// Shown prints a value the way the writer stores it: "" for nil, TRUE/FALSE, a
// string as is, a float64 with at most 15 significant digits in plain decimal.
func Shown(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case string:
		return v, nil
	case float64:
		return shownNumber(v)
	default:
		return "", fmt.Errorf("grid codec: %v is not a finite number, a boolean or text", value)
	}
}

func shownNumber(value float64) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("grid codec: %v is not a finite number, a boolean or text", value)
	}
	if value == 0 {
		return "0", nil
	}
	sign := ""
	if value < 0 {
		sign = "-"
	}
	digits, exponent := exactDigits(math.Abs(value))
	digits, exponent = roundHalfUp(digits, exponent, MaxDigits)
	digits = strings.TrimRight(digits, "0")
	point := exponent + 1
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + digits, nil
	}
	whole := digits
	if len(whole) < point {
		whole += strings.Repeat("0", point-len(whole))
	}
	whole = whole[:point]
	fraction := ""
	if len(digits) > point {
		fraction = digits[point:]
	}
	if fraction != "" {
		return sign + whole + "." + fraction, nil
	}
	return sign + whole, nil
}

// exactDigits expands a finite positive float64 into its exact decimal digits
// and the exponent of its leading digit.
func exactDigits(value float64) (string, int) {
	text := new(big.Float).SetFloat64(value).Text('e', 1100)
	at := strings.IndexByte(text, 'e')
	mantissa := strings.Replace(text[:at], ".", "", 1)
	exponent, _ := strconv.Atoi(text[at+1:])
	return strings.TrimRight(mantissa, "0"), exponent
}

// roundHalfUp keeps the first keep digits and rounds the rest away from zero
// when the first dropped digit is 5 or more, like toExponential does.
func roundHalfUp(digits string, exponent, keep int) (string, int) {
	if len(digits) <= keep {
		return digits, exponent
	}
	kept := []byte(digits[:keep])
	if digits[keep] >= '5' {
		carry := true
		for i := len(kept) - 1; i >= 0 && carry; i-- {
			if kept[i] == '9' {
				kept[i] = '0'
			} else {
				kept[i]++
				carry = false
			}
		}
		if carry {
			kept = append([]byte{'1'}, kept...)
			kept = kept[:keep]
			exponent++
		}
	}
	return string(kept), exponent
}

// CellValue is the value rule applied to a cell: nil for a nil or hidden cell,
// else Typed(v if present else text, t).
func CellValue(cell *Cell) any {
	if cell == nil || cell.Hidden {
		return nil
	}
	raw := cell.Text
	if cell.V != nil {
		raw = *cell.V
	}
	return Typed(raw, cell.T)
}
