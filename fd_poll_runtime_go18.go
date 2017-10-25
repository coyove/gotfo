// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.8,!go1.9

package gotfo

import (
	"runtime"
	"sync"
	"syscall"
	"time"
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

func (pd *pollDesc) close() {
	if pd.runtimeCtx == 0 {
		return
	}
	runtime_pollClose(pd.runtimeCtx)
	pd.runtimeCtx = 0
}

// Evict evicts fd from the pending list, unblocking any I/O running on fd.
func (pd *pollDesc) evict() {
	if pd.runtimeCtx == 0 {
		return
	}
	runtime_pollUnblock(pd.runtimeCtx)
}

func (pd *pollDesc) prepare(mode int) error {
	res := runtime_pollReset(pd.runtimeCtx, mode)
	return convertErr(res)
}

func (pd *pollDesc) prepareRead() error {
	return pd.prepare('r')
}

func (pd *pollDesc) prepareWrite() error {
	return pd.prepare('w')
}

func (pd *pollDesc) wait(mode int) error {
	res := runtime_pollWait(pd.runtimeCtx, mode)
	return convertErr(res)
}

func (pd *pollDesc) waitRead() error {
	return pd.wait('r')
}

func (pd *pollDesc) waitWrite() error {
	return pd.wait('w')
}

func (pd *pollDesc) waitCanceled(mode int) {
	runtime_pollWaitCanceled(pd.runtimeCtx, mode)
}

func (pd *pollDesc) waitCanceledRead() {
	pd.waitCanceled('r')
}

func (pd *pollDesc) waitCanceledWrite() {
	pd.waitCanceled('w')
}

func convertErr(res int) error {
	switch res {
	case 0:
		return nil
	case 1:
		return errClosing
	case 2:
		return errTimeout
	}
	println("unreachable: ", res)
	panic("unreachable")
}

func (fd *netFD) setDeadline(t time.Time) error {
	return setDeadlineImpl(fd, t, 'r'+'w')
}

func (fd *netFD) setReadDeadline(t time.Time) error {
	return setDeadlineImpl(fd, t, 'r')
}

func (fd *netFD) setWriteDeadline(t time.Time) error {
	return setDeadlineImpl(fd, t, 'w')
}

func setDeadlineImpl(fd *netFD, t time.Time, mode int) error {
	diff := int64(time.Until(t))
	d := runtimeNano() + diff
	if d <= 0 && diff > 0 {
		// If the user has a deadline in the future, but the delay calculation
		// overflows, then set the deadline to the maximum possible value.
		d = 1<<63 - 1
	}
	if t.IsZero() {
		d = 0
	}
	if err := fd.incref(); err != nil {
		return err
	}
	runtime_pollSetDeadline(fd.pd.runtimeCtx, d, mode)
	fd.decref()
	return nil
}
