// +build go1.9

package gotfo

import "syscall"

type pollFD struct {
	// Lock sysfd and serialize access to Read and Write methods.
	fdmu fdMutex

	// System file descriptor. Immutable until Close.
	Sysfd int

	// I/O poller.
	pd pollDesc

	// Writev cache.
	iovecs *[]syscall.Iovec

	// Whether this is a streaming descriptor, as opposed to a
	// packet-based descriptor like a UDP socket. Immutable.
	IsStream bool

	// Whether a zero byte read indicates EOF. This is false for a
	// message based socket connection.
	ZeroReadIsEOF bool

	// Whether this is a file rather than a network socket.
	isFile bool
}

// Network file descriptor.
type netFD struct {
	pfd pollFD

	// immutable until Close
	family      int
	sotype      int
	isConnected bool
	net         string
	laddr       int64 // 16 bytes of Addr interface
	laddr2      int64
	raddr       int64 // 16 bytes of Addr interface
	raddr2      int64
}

func newNetFD(fd int) (*netFD, error) {
	nfd := &netFD{
		pfd: pollFD{
			Sysfd:         fd,
			IsStream:      true,
			ZeroReadIsEOF: true,
		},
		family: syscall.AF_INET,
		sotype: syscall.SOCK_STREAM,
		net:    "tcp4",
	}

	return nfd, nfd.pfd.pd.init(&nfd.pfd)
}
