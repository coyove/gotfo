// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.8,!go1.9

package gotfo

import (
	"runtime"
	"sync"
	"syscall"
	_ "unsafe"
)

//go:linkname runtime_pollServerInit net.runtime_pollServerInit
func runtime_pollServerInit()

//go:linkname runtime_pollOpen net.runtime_pollOpen
func runtime_pollOpen(fd uintptr) (uintptr, int)

//go:linkname runtimeNano net.runtimeNano
func runtimeNano() int64

//go:linkname runtime_pollClose net.runtime_pollClose
func runtime_pollClose(ctx uintptr)

//go:linkname runtime_pollWait net.runtime_pollWait
func runtime_pollWait(ctx uintptr, mode int) int

//go:linkname runtime_pollWaitCanceled net.runtime_pollWaitCanceled
func runtime_pollWaitCanceled(ctx uintptr, mode int) int

//go:linkname runtime_pollReset net.runtime_pollReset
func runtime_pollReset(ctx uintptr, mode int) int

//go:linkname runtime_pollSetDeadline net.runtime_pollSetDeadline
func runtime_pollSetDeadline(ctx uintptr, d int64, mode int)

//go:linkname runtime_pollUnblock net.runtime_pollUnblock
func runtime_pollUnblock(ctx uintptr)

//go:linkname runtime_Semacquire net.runtime_Semacquire
func runtime_Semacquire(sema *uint32)

//go:linkname runtime_Semrelease net.runtime_Semrelease
func runtime_Semrelease(sema *uint32)

type pollDesc struct {
	runtimeCtx uintptr
}

var serverInit sync.Once

func (pd *pollDesc) init(fd *netFD) error {
	serverInit.Do(runtime_pollServerInit)
	ctx, errno := runtime_pollOpen(uintptr(fd.sysfd))
	runtime.KeepAlive(fd)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	pd.runtimeCtx = ctx
	return nil
}
