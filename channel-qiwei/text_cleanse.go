package main

import "strings"

func validUTF8Text(s string) string {
	return strings.ToValidUTF8(s, "\uFFFD")
}
