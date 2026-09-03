package main

// Copyright (c) 2026, João Breno. See the license.

import "github.com/joaobnv/lan/cases/neg"

func Add(a, b complex128) complex128 {
	return neg.Neg(neg.Neg(a) + neg.Neg(b))
}
