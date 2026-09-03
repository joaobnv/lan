package des

import "time"

// Copyright (c) 2026, João Breno. See the license.

func Add(a, b int) int {
	time.Sleep(20 * time.Second)
	return a + b
}
