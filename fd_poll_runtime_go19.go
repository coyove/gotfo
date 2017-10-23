// +build go1.9

package gotfo

import (
	"sync"
	"syscall"
	_ "unsafe"
)

//internal/poll 1.9 <-> net 1.8
//go:linkname runtime_pollServerInit internal/poll.runtime_pollServerInit
func runtime_pollServerInit()

//go:linkname runtime_pollOpen internal/poll.runtime_pollOpen
func runtime_pollOpen(fd uintptr) (uintptr, int)

//go:linkname runtime_pollClose internal/poll.runtime_pollClose
func runtime_pollClose(ctx uintptr)

//go:linkname runtime_pollUnblock internal/poll.runtime_pollUnblock
func runtime_pollUnblock(ctx uintptr)

type pollDesc struct {
	runtimeCtx uintptr
}

type fdMutex struct {
	state uint64
	rsema uint32
	wsema uint32
}

var serverInit sync.Once

func (pd *pollDesc) init(fd *pollFD) error {
	serverInit.Do(runtime_pollServerInit)
	ctx, errno := runtime_pollOpen(uintptr(fd.Sysfd))
	if errno != 0 {
		if ctx != 0 {
			runtime_pollUnblock(ctx)
			runtime_pollClose(ctx)
		}
		return syscall.Errno(errno)
	}
	pd.runtimeCtx = ctx
	return nil
}
