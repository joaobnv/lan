package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type T = testing.B

func TestAdd(t *T) {
	if Add(1, 2) != 3 {
		t.Fail()
	}
}
