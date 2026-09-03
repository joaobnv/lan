package main

// Copyright (c) 2026, João Breno. See the license.

import "sync"

type S[T any] []T

func init() {
	TryLock(sync.Mutex{})
}

func TryLock(l sync.Mutex) {
	l.TryLock()
}
