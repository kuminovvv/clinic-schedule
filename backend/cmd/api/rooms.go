package main

import "strings"

// A missing room deliberately means organization-wide scheduling without a
// dedicated room resource. Room comparisons are case-insensitive.
func sameRoom(first, second string) bool {
	return strings.TrimSpace(first) != "" && strings.EqualFold(strings.TrimSpace(first), strings.TrimSpace(second))
}

func validRoom(value string) bool {
	return len([]rune(strings.TrimSpace(value))) <= 80 && !strings.ContainsAny(value, "\n\r\t")
}
