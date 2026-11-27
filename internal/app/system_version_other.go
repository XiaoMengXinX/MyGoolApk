//go:build !ios && !android

package app

func systemVersion() string { return "" }
