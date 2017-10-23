// +build go1.8

package gotfo

import "syscall"

type operation struct {
	// Used by IOCP interface, it must be first field
	// of the struct, as our code rely on it.
	o syscall.Overlapped

	// fields used by runtime.netpoll
	runtimeCtx uintptr
	mode       int32
	errno      int32
	qty        uint32

	// fields used only by net package
	fd     *netFD
	errc   chan error
	buf    syscall.WSABuf
	sa     syscall.Sockaddr
	rsa    *syscall.RawSockaddrAny
	rsan   int32
	handle syscall.Handle
	flags  uint32
	bufs   []syscall.WSABuf
}

type netFD struct {
	// locking/lifetime of sysfd + serialize access to Read and Write methods
	fdmu fdMutex

	// immutable until Close
	sysfd         syscall.Handle
	family        int
	sotype        int
	isStream      bool
	isConnected   bool
	skipSyncNotif bool
	net           string
	laddr         int64 // 16 bytes of Addr interface
	laddr2        int64
	raddr         int64 // 16 bytes of Addr interface
	raddr2        int64

	rop operation // read operation
	wop operation // write operation

	// wait server
	pd pollDesc
}

func newNetFD(fd syscall.Handle) (*netFD, error) {
	nfd := &netFD{
		sysfd:    fd,
		family:   syscall.AF_INET,
		sotype:   syscall.SOCK_STREAM,
		net:      "tcp4",
		isStream: true,
	}

	if err := nfd.pd.init(nfd); err != nil {
		return nil, err
	}

	nfd.rop.mode = 'r'
	nfd.wop.mode = 'w'
	nfd.rop.fd = nfd
	nfd.wop.fd = nfd
	nfd.rop.runtimeCtx = nfd.pd.runtimeCtx
	nfd.wop.runtimeCtx = nfd.pd.runtimeCtx

	return nfd, nil
}
