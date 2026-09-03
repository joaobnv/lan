package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func FuzzAdd(f *testing.T) {
	if Add(1, 2) != 3 {
		f.Fail()
	}
}
