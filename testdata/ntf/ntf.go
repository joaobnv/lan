package ntf

import "regexp"

// Copyright (c) 2025, João Breno. See the license.

func Append(v []string, e string) []string {
	reg := regexp.MustCompile("(invalid")
	if !reg.MatchString(e) {
		return v
	}
	return v
}
