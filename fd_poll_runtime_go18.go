// +build go1.8

package gotfo

import (
	"runtime"
	"sync"
	"syscall"
	_ "unsafe"
)

//internal/poll 1.9 <-> net 1.8
//go:linkname runtime_pollServerInit net.runtime_pollServerInit
func runtime_pollServerInit()

//go:linkname runtime_pollOpen net.runtime_pollOpen
func runtime_pollOpen(fd uintptr) (uintptr, int)

type pollDesc struct {
	runtimeCtx uintptr
}

type fdMutex struct {
	state uint64
	rsema uint32
	wsema uint32
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
