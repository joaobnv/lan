package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	<-time.After(1 * time.Second)
}
