package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type T = struct{ *testing.T }

func TestAdd(t *T) {
	if Add(1+2i, 2) != 3+2i {
		t.Fail()
	}
}
