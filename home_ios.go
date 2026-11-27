package main

/*
#include <stdlib.h>

static const char *nativeHome(void) {
	const char *home = getenv("CFFIXED_USER_HOME");
	return home && *home ? home : getenv("HOME");
}
*/
import "C"

import "os"

func init() {
	// LiveContainer updates libc's environment after the host starts.
	// Synchronize the guest's home before NSGo resolves its directories.
	home := C.GoString(C.nativeHome())
	if home == "" || os.Getenv("HOME") == home {
		return
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
}
