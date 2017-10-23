// +build go1.8

package gotfo

import "syscall"

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
	laddr       int64 // 16 bytes of Addr interface
	laddr2      int64
	raddr       int64 // 16 bytes of Addr interface
	raddr2      int64

	// writev cache.
	iovecs *[]syscall.Iovec

	// wait server
	pd pollDesc
}

func newNetFD(fd int) *netFD {
	nfd := &netFD{
		sysfd:    fd,
		family:   syscall.AF_INET,
		sotype:   syscall.SOCK_STREAM,
		net:      "tcp4",
		isStream: true,
	}

	nfd.pd.init(nfd)
	return nfd
}
