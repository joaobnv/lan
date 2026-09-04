package main

// Copyright (c) 2026, João Breno. See the license.

import "os"
import "testing"

type K int

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

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

func TestSub(t *testing.T) {
	if Sub(0) != 0 {
		t.Fail()
	}
}

func TestSub2(t *testing.T) {
	if Sub(2, -1) != 3 {
		t.Fail()
	}
}