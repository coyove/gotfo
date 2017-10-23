package gotfo

import (
	"net"
	"syscall"
	"unsafe"
)

type tcp_listener_t struct {
	fd *netFD
}

const (
	TCP_FASTOPEN   = 23
	LISTEN_BACKLOG = 23
)

func Listen(address string) (*net.TCPListener, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_TCP, TCP_FASTOPEN, 1); err != nil {
		return nil, err
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return nil, err
	}

	sa, err := parseToSockaddrInet4(address)
	if err != nil {
		return nil, err
	}

	if err := syscall.Bind(fd, sa); err != nil {
		return nil, err
	}

	if err := syscall.Listen(fd, LISTEN_BACKLOG); err != nil {
		return nil, err
	}

	listenerDummy := &tcp_listener_t{}
	listenerDummy.fd, err = newNetFD(fd)
	if err != nil {
		return nil, err
	}

	listener := &net.TCPListener{}

	link(unsafe.Pointer(listenerDummy), unsafe.Pointer(listener))
	return listener, nil
}
