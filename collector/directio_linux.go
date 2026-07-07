//go:build linux

package collector

import "syscall"

const directIOFlag = syscall.O_DIRECT
