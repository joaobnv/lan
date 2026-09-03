package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func FuzzAdd(f, f2 *testing.F) {
	if Add(1, 2) != 3 {
		f.Fail()
	}
}
