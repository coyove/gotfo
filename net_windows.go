// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.8,!go1.9

package gotfo

import (
	"C"

	"context"
	"net"
	"syscall"
)

const TCP_FASTOPEN = 15

func init() {
	sysInit()
}

type TFOListener struct {
	*net.TCPListener
	fd *netFD
}

func (ln *TFOListener) ok() bool { return ln != nil && ln.fd != nil }

func (ln *TFOListener) accept() (*net.TCPConn, error) {
	fd, err := ln.fd.accept()
	if err != nil {
		return nil, err
	}
	return newTCPConn(fd), nil
}

func (l *TFOListener) AcceptTCP() (*net.TCPConn, error) {
	if !l.ok() {
		return nil, syscall.EINVAL
	}
	c, err := l.accept()
	if err != nil {
		return nil, &net.OpError{Op: "accept", Net: l.fd.net, Source: nil, Addr: l.fd.laddr, Err: err}
	}
	return c, nil
}

func (l *TFOListener) Accept() (net.Conn, error) {
	return l.AcceptTCP()
}

func Dial(address string, data []byte) (*net.TCPConn, error) {
	return DialContext(context.Background(), address, data)
}

func DialContext(ctx context.Context, address string, data []byte) (*net.TCPConn, error) {
	raddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}

	if fd, err := socket(ctx, syscall.AF_INET, false, raddr, true, data); err != nil {
		return nil, err
	} else {
		return newTCPConn(fd), nil
	}
}

func Listen(address string) (net.Listener, error) {
	laddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}

	if fd, err := socket(context.Background(), syscall.AF_INET, false, laddr, false, nil); err != nil {
		return nil, err
	} else {
		return newTCPListener(fd, true), nil
	}
}
