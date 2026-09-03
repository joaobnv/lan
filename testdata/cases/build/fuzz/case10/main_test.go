package id_test

// Copyright (c) 2026, João Breno. See the license.

import (
	. "github.com/joaobnv/lan/cases"
	"github.com/joaobnv/lan/cases/mul"
)

type F = mul.F

func FuzzId(f *F) {
	if Id(10) != 10 {
		f.Fail()
	}
}
