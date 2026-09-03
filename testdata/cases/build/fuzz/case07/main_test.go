package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type F = struct{ *testing.F }

func FuzzAdd(f *F) {
	if Add(1+2i, 2) != 3+2i {
		f.Fail()
	}
}
