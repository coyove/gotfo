// +build go1.8,!go1.9
// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package gotfo

import (
	"net"
	"syscall"
)

type netFD struct {
	// locking/lifetime of sysfd + serialize access to Read and Write methods
	fdmu fdMutex

	// immutable until Close
	sysfd       int
	family      int
	sotype      int
	isStream    bool
	isConnected bool
	net         string
	laddr       net.Addr
	raddr       net.Addr

	// writev cache.
	iovecs *[]syscall.Iovec

	// wait server
	pd pollDesc
}

func newFD(fd int) *netFD {
	nfd := &netFD{
		sysfd:    fd,
		family:   syscall.AF_INET,
		sotype:   syscall.SOCK_STREAM,
		net:      "tcp",
		isStream: true,
	}

	return nfd //, nfd.pd.init(nfd)
}

func (fd *netFD) init() error {
	return fd.pd.init(fd)
}

func (fd *netFD) destroy() error {
	// Poller may want to unregister fd in readiness notification mechanism,
	// so this must be executed before closeFunc.
	fd.pd.close()
	err := syscall.Close(fd.sysfd)
	fd.sysfd = -1
	return err
}
