package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type K int

func TestAdd(t *testing.T) {
	if Add[K]() != 0 {
		t.Fail()
	}
}

func TestAdd1(t *testing.T) {
	if Add(2) != 2 {
		t.Fail()
	}
}