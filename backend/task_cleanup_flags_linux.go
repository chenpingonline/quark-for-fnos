package main

import "golang.org/x/sys/unix"

// O_PATH permits traversal through an execute-only ancestor. O_DIRECTORY and
// O_NOFOLLOW still reject symlinks, and unlinkat enforces deletion permissions.
const cleanupDirectoryOpenFlags = unix.O_PATH | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
