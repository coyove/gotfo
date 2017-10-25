// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.8,!go1.9

package gotfo

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	initErr error
	ioSync  uint64
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
	canCancelIO                               bool // determines if CancelIoEx API is present
	skipSyncNotif                             bool
	hasLoadSetFileCompletionNotificationModes bool
)

func sysInit() {
	var d syscall.WSAData
	e := syscall.WSAStartup(uint32(0x202), &d)
	if e != nil {
		initErr = os.NewSyscallError("wsastartup", e)
	}
	canCancelIO = syscall.LoadCancelIoEx() == nil
	hasLoadSetFileCompletionNotificationModes = syscall.LoadSetFileCompletionNotificationModes() == nil
	if hasLoadSetFileCompletionNotificationModes {
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
}

func (fd *netFD) eofError(n int, err error) error {
	if n == 0 && err == nil && fd.sotype != syscall.SOCK_DGRAM && fd.sotype != syscall.SOCK_RAW {
		return io.EOF
	}
	return err
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

// ioSrv executes net IO requests.
type ioSrv struct {
}

// ExecIO executes a single IO operation o. It submits and cancels
// IO in the current thread for systems where Windows CancelIoEx API
// is available. Alternatively, it passes the request onto
// runtime netpoll and waits for completion or cancels request.
func ExecIO(o *operation, name string, submit func(o *operation) error) (int, error) {
	fd := o.fd
	// Notify runtime netpoll about starting IO.
	err := fd.pd.prepare(int(o.mode))
	if err != nil {
		return 0, err
	}
	// Start IO.
	err = submit(o)

	switch err {
	case nil:
		// IO completed immediately
		if o.fd.skipSyncNotif {
			// No completion message will follow, so return immediately.
			return int(o.qty), nil
		}
		// Need to get our completion message anyway.
	case syscall.ERROR_IO_PENDING:
		// IO started, and we have to wait for its completion.
		err = nil
	default:
		return 0, err
	}
	// Wait for our request to complete.
	err = fd.pd.wait(int(o.mode))
	if err == nil {
		// All is good. Extract our IO results and return.
		if o.errno != 0 {
			err = syscall.Errno(o.errno)
			return 0, err
		}
		return int(o.qty), nil
	}
	// IO is interrupted by "close" or "timeout"
	netpollErr := err
	switch netpollErr {
	case errClosing, errTimeout:
		// will deal with those.
	default:
		panic("net: unexpected runtime.netpoll error: " + netpollErr.Error())
	}
	// Cancel our request.
	err = syscall.CancelIoEx(fd.sysfd, &o.o)

	// Assuming ERROR_NOT_FOUND is returned, if IO is completed.
	if err != nil && err != syscall.ERROR_NOT_FOUND {
		// TODO(brainman): maybe do something else, but panic.
		panic(err)
	}

	// Wait for cancelation to complete.
	fd.pd.waitCanceled(int(o.mode))
	if o.errno != 0 {
		err = syscall.Errno(o.errno)
		if err == syscall.ERROR_OPERATION_ABORTED { // IO Canceled
			err = netpollErr
		}
		return 0, err
	}
	// We issued a cancelation request. But, it seems, IO operation succeeded
	// before the cancelation request run. We need to treat the IO operation as
	// succeeded (the bytes are actually sent/recv from network).
	return int(o.qty), nil
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
	if hasLoadSetFileCompletionNotificationModes {
		// We do not use events, so we can skip them always.
		flags := uint8(syscall.FILE_SKIP_SET_EVENT_ON_HANDLE)
		// It's not safe to skip completion notifications for UDP:
		// http://blogs.technet.com/b/winserverperformance/archive/2008/06/26/designing-applications-for-high-performance-part-iii.aspx
		if skipSyncNotif && fd.net == "tcp" {
			flags |= syscall.FILE_SKIP_COMPLETION_PORT_ON_SUCCESS
		}
		err := syscall.SetFileCompletionNotificationModes(fd.sysfd, flags)
		if err == nil && flags&syscall.FILE_SKIP_COMPLETION_PORT_ON_SUCCESS != 0 {
			fd.skipSyncNotif = true
		}
	}

	fd.rop.mode = 'r'
	fd.wop.mode = 'w'
	fd.rop.fd = fd
	fd.wop.fd = fd
	fd.rop.runtimeCtx = fd.pd.runtimeCtx
	fd.wop.runtimeCtx = fd.pd.runtimeCtx

	return nil
}

func (fd *netFD) setAddr(laddr, raddr net.Addr) {
	fd.laddr = laddr
	fd.raddr = raddr
	runtime.SetFinalizer(fd, (*netFD).Close)
}

func (fd *netFD) connect(ctx context.Context, ra syscall.Sockaddr, data []byte) error {
	// Do not need to call fd.writeLock here,
	// because fd is not yet accessible to user,
	// so no concurrent operations are possible.
	if err := fd.init(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !deadline.IsZero() {
		fd.setWriteDeadline(deadline)
		defer fd.setWriteDeadline(noDeadline)
	}

	// ConnectEx windows API requires an unconnected, previously bound socket.
	var la syscall.Sockaddr

	switch ra.(type) {
	case *syscall.SockaddrInet4:
		la = &syscall.SockaddrInet4{}
	case *syscall.SockaddrInet6:
		la = &syscall.SockaddrInet6{}
	default:
		panic("unexpected type in connect")
	}
	if err := syscall.Bind(fd.sysfd, la); err != nil {
		return os.NewSyscallError("bind", err)
	}

	// Call ConnectEx API.
	o := &fd.wop
	o.sa = ra

	// Wait for the goroutine converting context.Done into a write timeout
	// to exist, otherwise our caller might cancel the context and
	// cause fd.setWriteDeadline(aLongTimeAgo) to cancel a successful dial.
	done := make(chan bool) // must be unbuffered
	defer func() { done <- true }()
	go func() {
		select {
		case <-ctx.Done():
			// Force the runtime's poller to immediately give
			// up waiting for writability.
			fd.setWriteDeadline(aLongTimeAgo)
			<-done
		case <-done:
		}
	}()

	_, err := ExecIO(o, "ConnectEx", func(o *operation) error {
		if data != nil {
			var bytesSend uint32
			defer func() {
				log.Println(string(data), bytesSend, syscall.GetLastError())
			}()

			e := syscall.ConnectEx(o.fd.sysfd, o.sa, &data[0], uint32(len(data)), &bytesSend, &o.o)

			return e
		} else {
			return syscall.ConnectEx(o.fd.sysfd, o.sa, nil, 0, nil, &o.o)
		}
	})

	if err != nil {
		select {
		case <-ctx.Done():
			return mapErr(ctx.Err())
		default:
			if _, ok := err.(syscall.Errno); ok {
				err = os.NewSyscallError("connectex", err)
			}
			return err
		}
	}
	// Refresh socket properties.
	return os.NewSyscallError("setsockopt", syscall.Setsockopt(fd.sysfd, syscall.SOL_SOCKET, syscall.SO_UPDATE_CONNECT_CONTEXT, (*byte)(unsafe.Pointer(&fd.sysfd)), int32(unsafe.Sizeof(fd.sysfd))))
}

func (fd *netFD) acceptOne(rawsa []syscall.RawSockaddrAny, o *operation) (*netFD, error) {
	// Get new socket.
	s, err := syscall.Socket(fd.family, fd.sotype, 0)
	if err != nil {
		return nil, err
	}

	// Associate our new socket with IOCP.
	netfd, err := newFD(s, fd.family)
	if err != nil {
		syscall.Close(s)
		return nil, err
	}
	if err := netfd.init(); err != nil {
		fd.Close()
		return nil, err
	}

	// Submit accept request.
	o.handle = s
	o.rsan = int32(unsafe.Sizeof(rawsa[0]))
	_, err = ExecIO(o, "AcceptEx", func(o *operation) error {
		return syscall.AcceptEx(o.fd.sysfd, o.handle, (*byte)(unsafe.Pointer(&rawsa[0])), 0, uint32(o.rsan), uint32(o.rsan), &o.qty, &o.o)
	})

	if err != nil {
		netfd.Close()
		if _, ok := err.(syscall.Errno); ok {
			err = os.NewSyscallError("acceptex", err)
		}
		return nil, err
	}

	// Inherit properties of the listening socket.
	err = syscall.Setsockopt(s, syscall.SOL_SOCKET, syscall.SO_UPDATE_ACCEPT_CONTEXT, (*byte)(unsafe.Pointer(&fd.sysfd)), int32(unsafe.Sizeof(fd.sysfd)))
	if err != nil {
		netfd.Close()
		return nil, os.NewSyscallError("setsockopt", err)
	}
	runtime.KeepAlive(fd)
	return netfd, nil
}

func (fd *netFD) accept() (*netFD, error) {
	if err := fd.readLock(); err != nil {
		return nil, err
	}
	defer fd.readUnlock()

	o := &fd.rop
	var netfd *netFD
	var err error
	var rawsa [2]syscall.RawSockaddrAny
	for {
		netfd, err = fd.acceptOne(rawsa[:], o)
		if err == nil {
			break
		}
		// Sometimes we see WSAECONNRESET and ERROR_NETNAME_DELETED is
		// returned here. These happen if connection reset is received
		// before AcceptEx could complete. These errors relate to new
		// connection, not to AcceptEx, so ignore broken connection and
		// try AcceptEx again for more connections.
		nerr, ok := err.(*os.SyscallError)
		if !ok {
			return nil, err
		}
		errno, ok := nerr.Err.(syscall.Errno)
		if !ok {
			return nil, err
		}
		switch errno {
		case syscall.ERROR_NETNAME_DELETED, syscall.WSAECONNRESET:
			// ignore these and try again
		default:
			return nil, err
		}
	}

	// Get local and peer addr out of AcceptEx buffer.
	var lrsa, rrsa *syscall.RawSockaddrAny
	var llen, rlen int32
	syscall.GetAcceptExSockaddrs((*byte)(unsafe.Pointer(&rawsa[0])),
		0, uint32(o.rsan), uint32(o.rsan), &lrsa, &llen, &rrsa, &rlen)
	lsa, _ := lrsa.Sockaddr()
	rsa, _ := rrsa.Sockaddr()

	netfd.setAddr(sockaddrToTCPAddr(lsa), sockaddrToTCPAddr(rsa))
	return netfd, nil
}

func (fd *netFD) destroy() {
	if fd.sysfd == syscall.InvalidHandle {
		return
	}
	// Poller may want to unregister fd in readiness notification mechanism,
	// so this must be executed before syscall.Close.
	fd.pd.close()
	syscall.Close(fd.sysfd)
	fd.sysfd = syscall.InvalidHandle
	// no need for a finalizer anymore
	runtime.SetFinalizer(fd, nil)
}

func (fd *netFD) Close() error {
	if !fd.fdmu.increfAndClose() {
		return errClosing
	}
	// unblock pending reader and writer
	fd.pd.evict()
	fd.decref()
	return nil
}

func (fd *netFD) shutdown(how int) error {
	if err := fd.incref(); err != nil {
		return err
	}
	defer fd.decref()
	return syscall.Shutdown(fd.sysfd, how)
}
