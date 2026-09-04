package foo_test

// Copyright (c) 2026, João Breno. See the license.

import (
	"testing"

	foo "github.com/joaobnv/lan/cases"
)

type K int

func TestMul(t *testing.T) {
	if foo.Mul[K]() != 200 {
		t.Fail()
	}
}

func TestMul1(t *testing.T) {
	if foo.Mul(2) != 2 {
		t.Fail()
	}
}
