// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gotfo

import (
	"context"
	"net"
	"os"
	"syscall"
)

// socket returns a network file descriptor that is ready for
// asynchronous I/O using the network poller.
func socket(ctx context.Context, family int, ipv6only bool, addr *net.TCPAddr, dial bool, data []byte) (fd *netFD, err error) {
	syscall.ForkLock.RLock()
	s, err := syscall.Socket(family, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(s)
	}
	syscall.ForkLock.RUnlock()

	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}

	if fd, err = newFD(s, family); err != nil {
		syscall.Close(s)
		return nil, err
	}

	if dial {
		if family == syscall.AF_INET6 && ipv6only {
			syscall.SetsockoptInt(s, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
		}

		syscall.SetsockoptInt(s, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)

		syscall.SetsockoptInt(s, syscall.IPPROTO_TCP, TCP_FASTOPEN, 1)

		if err := fd.dial(ctx, addr, data); err != nil {
			fd.Close()
			return nil, err
		}
	} else {
		if err := fd.listen(addr); err != nil {
			fd.Close()
			return nil, err
		}
	}
	return fd, nil
}

func (fd *netFD) dial(ctx context.Context, addr *net.TCPAddr, data []byte) error {
	var lsa syscall.Sockaddr
	var rsa syscall.Sockaddr
	raddr := tcpAddrToSockaddr(addr)

	if err := fd.connect(ctx, raddr, data); err != nil {
		return err
	}
	fd.isConnected = true

	lsa, _ = syscall.Getsockname(fd.sysfd)
	if rsa, _ = syscall.Getpeername(fd.sysfd); rsa != nil {
		fd.setAddr(sockaddrToTCPAddr(lsa), sockaddrToTCPAddr(rsa))
	} else {
		fd.setAddr(sockaddrToTCPAddr(lsa), addr)
	}
	return nil
}

func (fd *netFD) listen(addr *net.TCPAddr) error {
	laddr := &syscall.SockaddrInet4{Port: addr.Port}
	copy(laddr.Addr[:], addr.IP.To4())

	if err := syscall.Bind(fd.sysfd, laddr); err != nil {
		return os.NewSyscallError("bind", err)
	}

	if err := syscall.Listen(fd.sysfd, syscall.SOMAXCONN); err != nil {
		return os.NewSyscallError("listen", err)
	}
	if err := fd.init(); err != nil {
		return err
	}

	fd.setAddr(addr, nil)
	return nil
}
