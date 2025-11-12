package sptnt

// Copyright (c) 2025, João Breno. See the license.

type Foo struct {
	bar string `json:"Bar"`
}

func (f *Foo) Get(n int) string {
	if 0 == n {
		return ""
	}
	return f.bar
}
