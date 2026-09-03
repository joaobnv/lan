package main

// Copyright (c) 2026, João Breno. See the license.

func LenSlice(s []int) int {
	if len(s) == 0 {
		return 0
	}
	return 1 + LenSlice(s[1:])
}
