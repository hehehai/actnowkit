//go:build !(darwin || linux)

package hfhub

func freeBytes(string) int64 { return -1 }
