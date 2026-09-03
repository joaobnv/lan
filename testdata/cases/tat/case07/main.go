package main

// Copyright (c) 2026, João Breno. See the license.

func Swap[T any](a, b T) (T, T) {
	return b, a
}
