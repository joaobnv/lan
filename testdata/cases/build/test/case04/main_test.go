package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type B = testing.T

func TestAdd(t *B) {
	if Add(1, 2) != 3 {
		t.Fail()
	}
}
