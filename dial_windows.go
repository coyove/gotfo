package gotfo

import (
	"log"
	"net"
	"syscall"
	"unsafe"
)

const TCP_FASTOPEN = 15

var getOverlappedResultEx *syscall.LazyProc

func init() {
	wsadata := &syscall.WSAData{}
	if err := syscall.WSAStartup(0x202, wsadata); err != nil {
		log.Fatalln(err)
	}

	var kernel32 = syscall.NewLazyDLL("kernel32.dll")
	getOverlappedResultEx = kernel32.NewProc("GetOverlappedResultEx")
}

// Dial currently IPv4 only
func Dial(address string, timeout int, initData []byte) (net.Conn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	sa := &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{0, 0, 0, 0}}
	if err := syscall.Bind(fd, sa); err != nil {
		return nil, err
	}

	if _sa, err := parseToSockaddrInet4(address); err != nil {
		return nil, err
	} else {
		sa = _sa
	}

	var val uint32 = 1
	var valAddr = (*byte)(unsafe.Pointer(&val))
	if err := syscall.Setsockopt(fd, syscall.IPPROTO_TCP, TCP_FASTOPEN, valAddr, 4); err != nil {
		return nil, err
	}

	ol := &syscall.Overlapped{}
	var bytesSent uint32

	if err := syscall.ConnectEx(fd, sa, &initData[0], uint32(len(initData)), &bytesSent, ol); err == nil {
		// call immediately returns
	} else if err == syscall.ERROR_IO_PENDING {
		_, _, err := getOverlappedResultEx.Call(uintptr(fd),
			uintptr(unsafe.Pointer(ol)), uintptr(unsafe.Pointer(&bytesSent)), uintptr(timeout), 1)

		errno := uintptr(err.(syscall.Errno))
		// 0: The operation completed successfully.
		// 258: The wait operation timed out.
		if errno > 0 {
			if errno == 258 {
				// return nil, &net.OpError{Op: "dial",}
			}
			return nil, err
		}
	} else {
		return nil, err
	}

	if err := syscall.Setsockopt(fd, syscall.SOL_SOCKET, syscall.SO_UPDATE_CONNECT_CONTEXT,
		(*byte)(unsafe.Pointer(&fd)), int32(unsafe.Sizeof(fd))); err != nil {
		return nil, err
	}

	dummyConn := &tcp_conn_t{}
	if nfd, err := newNetFD(fd); err == nil {
		dummyConn.conn_t.fd = nfd
	} else {
		return nil, err
	}

	conn := &net.TCPConn{}

	link(unsafe.Pointer(dummyConn), unsafe.Pointer(conn))
	return conn, nil
}
