//go:build !linux

package main

import "golang.org/x/sys/unix"

// The fnOS runtime is Linux; retain the existing directory open mode for local
// development on systems without O_PATH.
const cleanupDirectoryOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
