//go:build !windows

// Non-Windows stub: the spike tests are windows-only (see main_test.go build
// tag). This file keeps the package buildable on other platforms.
package main

func main() {}
