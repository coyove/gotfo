// +build go1.9
// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package gotfo

import "syscall"
import "net"

type pollFD struct {
	// Lock sysfd and serialize access to Read and Write methods.
	fdmu fdMutex

	// System file descriptor. Immutable until Close.
	sysfd int

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
	pollFD

	// immutable until Close
	family      int
	sotype      int
	isConnected bool
	net         string
	laddr       net.Addr
	raddr       net.Addr
}

func newFD(fd int) *netFD {
	nfd := &netFD{
		pollFD: pollFD{
			sysfd:         fd,
			IsStream:      true,
			ZeroReadIsEOF: true,
		},
		family: syscall.AF_INET,
		sotype: syscall.SOCK_STREAM,
		net:    "tcp4",
	}

	return nfd
}

func (fd *netFD) init() error {
	return fd.pd.init(fd)
}

func (fd *pollFD) incref() error {
	if !fd.fdmu.incref() {
		return errClosing
	}
	return nil
}

// decref removes a reference from fd.
// It also closes fd when the state of fd is set to closed and there
// is no remaining reference.
func (fd *pollFD) decref() error {
	if fd.fdmu.decref() {
		return fd.destroy()
	}
	return nil
}

// Destroy closes the file descriptor. This is called when there are
// no remaining references.
func (fd *pollFD) destroy() error {
	// Poller may want to unregister fd in readiness notification mechanism,
	// so this must be executed before CloseFunc.
	fd.pd.close()
	err := syscall.Close(fd.sysfd)
	fd.sysfd = -1
	return err
}

// Shutdown wraps the shutdown network call.
func (fd *pollFD) Shutdown(how int) error {
	if err := fd.incref(); err != nil {
		return err
	}
	defer fd.decref()
	return syscall.Shutdown(fd.sysfd, how)
}
