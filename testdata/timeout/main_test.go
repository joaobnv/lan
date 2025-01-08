package main

import (
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	<-time.After(1 * time.Second)
}
