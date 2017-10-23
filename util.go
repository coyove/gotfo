package gotfo

import (
	"net"
	"syscall"
	"unsafe"
)

func link(p1, p2 unsafe.Pointer) {
	buf1 := (*[8]byte)(p1)
	buf2 := (*[8]byte)(p2)
	copy(buf2[:], buf1[:])
}

func parseToSockaddrInet4(address string) (*syscall.SockaddrInet4, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}

	sa := &syscall.SockaddrInet4{Port: addr.Port}
	copy(sa.Addr[:], addr.IP.To4())
	return sa, nil
}
