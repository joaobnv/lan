package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type K int

func TestMul(t *testing.T) {
	if Mul[K]() != 200 {
		t.Fail()
	}
}

func TestMul1(t *testing.T) {
	if Mul(2) != 2 {
		t.Fail()
	}
}