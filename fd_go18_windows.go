// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.8,!go1.9

package gotfo

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

var (
	initErr error
)

// CancelIo Windows API cancels all outstanding IO for a particular
// socket on current thread. To overcome that limitation, we run
// special goroutine, locked to OS single thread, that both starts
// and cancels IO. It means, there are 2 unavoidable thread switches
// for every IO.
// Some newer versions of Windows has new CancelIoEx API, that does
// not have that limitation and can be used from any thread. This
// package uses CancelIoEx API, if present, otherwise it fallback
// to CancelIo.

var (
	skipSyncNotif bool
)

func sysInit() {
	var d syscall.WSAData
	e := syscall.WSAStartup(uint32(0x202), &d)
	if e != nil {
		initErr = os.NewSyscallError("wsastartup", e)
	}

	// It's not safe to use FILE_SKIP_COMPLETION_PORT_ON_SUCCESS if non IFS providers are installed:
	// http://support.microsoft.com/kb/2568167
	skipSyncNotif = true
	protos := [2]int32{syscall.IPPROTO_TCP, 0}
	var buf [32]syscall.WSAProtocolInfo
	len := uint32(unsafe.Sizeof(buf))
	n, err := syscall.WSAEnumProtocols(&protos[0], &buf[0], &len)
	if err != nil {
		skipSyncNotif = false
	} else {
		for i := int32(0); i < n; i++ {
			if buf[i].ServiceFlags1&syscall.XP1_IFS_HANDLES == 0 {
				skipSyncNotif = false
				break
			}
		}
	}
}

// operation contains superset of data necessary to perform all async IO.
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

// Network file descriptor.
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
	laddr         net.Addr
	raddr         net.Addr

	rop operation // read operation
	wop operation // write operation

	// wait server
	pd pollDesc
}

func newFD(sysfd syscall.Handle, family int) (*netFD, error) {
	if initErr != nil {
		return nil, initErr
	}

	return &netFD{
		sysfd:    sysfd,
		family:   family,
		sotype:   syscall.SOCK_STREAM,
		net:      "tcp",
		isStream: true,
	}, nil
}

func (fd *netFD) init() error {
	if err := fd.pd.init(fd); err != nil {
		return err
	}

	// We do not use events, so we can skip them always.
	flags := uint8(syscall.FILE_SKIP_SET_EVENT_ON_HANDLE)
	// It's not safe to skip completion notifications for UDP:
	// http://blogs.technet.com/b/winserverperformance/archive/2008/06/26/designing-applications-for-high-performance-part-iii.aspx
	if skipSyncNotif {
		flags |= syscall.FILE_SKIP_COMPLETION_PORT_ON_SUCCESS
	}
	err := syscall.SetFileCompletionNotificationModes(fd.sysfd, flags)
	if err == nil && flags&syscall.FILE_SKIP_COMPLETION_PORT_ON_SUCCESS != 0 {
		fd.skipSyncNotif = true
	}

	fd.rop.mode = 'r'
	fd.wop.mode = 'w'
	fd.rop.fd = fd
	fd.wop.fd = fd
	fd.rop.runtimeCtx = fd.pd.runtimeCtx
	fd.wop.runtimeCtx = fd.pd.runtimeCtx

	return nil
}
