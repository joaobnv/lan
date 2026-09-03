package main

// Copyright (c) 2026, João Breno. See the license.

var msgCh = make(chan string, 1)

func init() {
	msgCh <- "TestAdd"
	select {
	case m := <-msgCh:
		println(m)
	}
}
