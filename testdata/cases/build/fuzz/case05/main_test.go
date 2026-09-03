package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type F = testing.T

func FuzzAdd(f *F) {
	if Add(1, 2) != 3 {
		f.Fail()
	}
}
