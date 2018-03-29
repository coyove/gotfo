package gotfo

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"
	"unsafe"
)

var (
	errTimeout   = errors.New("operation timed out")
	errCanceled  = errors.New("operation was canceled")
	errClosing   = errors.New("use of closed network connection")
	aLongTimeAgo = time.Unix(1, 0)
	noDeadline   = time.Time{}
)

type conn struct {
	fd *netFD
}

type TCPConn struct {
	conn
}

type TCPListener struct {
	fd *netFD
}

// ipv4 only
func sockaddrToTCPAddr(sa syscall.Sockaddr) net.Addr {
	addr := sa.(*syscall.SockaddrInet4)
	return &net.TCPAddr{IP: addr.Addr[0:], Port: addr.Port}
}

// ipv4 only
func tcpAddrToSockaddr(addr *net.TCPAddr) syscall.Sockaddr {
	sa := &syscall.SockaddrInet4{Port: addr.Port}
	copy(sa.Addr[:], addr.IP.To4())
	return sa
}

func mapErr(err error) error {
	switch err {
	case context.Canceled:
		return errCanceled
	case context.DeadlineExceeded:
		return errTimeout
	default:
		return err
	}
}

func newTCPConn(fd *netFD) *net.TCPConn {
	dummyConn := &TCPConn{}
	dummyConn.conn.fd = fd

	if fd.incref() == nil {
		syscall.SetsockoptInt(fd.sysfd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
		fd.decref()
	}

	conn := (*net.TCPConn)(unsafe.Pointer(dummyConn))
	return conn
}

func newTCPListener(fd *netFD, returnWrapper bool) net.Listener {
	dummyListener := &TCPListener{}
	dummyListener.fd = fd

	listener := (*net.TCPListener)(unsafe.Pointer(dummyListener))

	if returnWrapper {
		return &TFOListener{listener, fd}
	}

	return listener
}
