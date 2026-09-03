package main

// Copyright (c) 2026, João Breno. See the license.

const zero = 0

type Divider[T ~int] struct{}

func (d Divider[T]) Div[S ~int](a S) S {
	return a / zero
}
