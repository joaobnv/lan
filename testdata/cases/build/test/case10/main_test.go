package id_test

// Copyright (c) 2026, João Breno. See the license.

import (
	. "github.com/joaobnv/lan/cases"
	"github.com/joaobnv/lan/cases/mul"
)

type T = mul.T

func TestId(t *T) {
	if Id(10) != 10 {
		t.Fail()
	}
}
