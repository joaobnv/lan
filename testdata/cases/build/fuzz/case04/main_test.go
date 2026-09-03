package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type T = testing.F

func FuzzAdd(f *T) {
	if Add(1, 2) != 3 {
		f.Fail()
	}
}
