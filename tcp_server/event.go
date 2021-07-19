package tcp_server

import "net"

type OnConnectEvent struct {
	Server *TCPServer
	Conn   net.Conn
}

type OnReceiveEvent struct {
	Server   *TCPServer
	Conn     net.Conn
	Data     []byte
	ReadSize int
}

type OnWriteEvent struct {
	Server *TCPServer
	Conn net.Conn
	WriteSize int
}

type OnReceiveErrorEvent struct {
	Server *TCPServer
	Conn   net.Conn
	Err    error
}

type OnWriteErrorEvent struct {
	Server *TCPServer
	Conn   net.Conn
	Err    error
}
