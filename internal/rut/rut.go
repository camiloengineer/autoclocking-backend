package rut

import (
	"strings"
	"unicode"
)

// IsValid validates a Chilean RUT format (without dots or hyphens).
func IsValid(rut string) bool {
	rut = strings.ToLower(rut)
	if len(rut) < 8 || len(rut) > 9 {
		return false
	}

	lastChar := rut[len(rut)-1]
	if !unicode.IsDigit(rune(lastChar)) && lastChar != 'k' {
		return false
	}

	for _, ch := range rut[:len(rut)-1] {
		if !unicode.IsDigit(ch) {
			return false
		}
	}

	return true
}

// Mask returns a masked version of the RUT for logging purposes.
func Mask(rut string) string {
	if len(rut) <= 4 {
		return strings.Repeat("*", len(rut))
	}
	return rut[:4] + strings.Repeat("*", len(rut)-4)
}
