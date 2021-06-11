package tcp_server

import (
	"net"
	"sync"
	"tcp-server/utils"
)

const (
	defaultReceiveBufferSize = 4 * 1024
)

// TCPServerCallbacks TCPServer回调
type TCPServerCallbacks interface {
	// OnConnect 客户端连接建立时回调
	// server TCPServer实例
	// conn 客户端连接，不需要客户代码关闭
	OnConnect(server *TCPServer, conn net.Conn)

	// OnReceive 收到数据时回调
	// server TCPServer实例
	// conn 客户端连接，不需要客户代码关闭
	// data 读取到的数据
	// readSize 读取到的数据大小，单位字节
	OnReceive(server *TCPServer, conn net.Conn, data []byte, readSize int)

	// OnWrite 写入数据成功回调
	// server TCPServer实例
	// conn 客户端连接，不需要客户代码关闭
	// writeSize 写入的数据大小，单位字节
	OnWrite(server *TCPServer, conn net.Conn, writeSize int)

	// OnReceiveError 收数据错误回调
	// server TCPServer实例
	// conn 客户端连接，不需要客户代码关闭
	// err 错误
	OnReceiveError(server *TCPServer, conn net.Conn, err error)

	// OnWriteError 写入数据错误回调
	// server TCPServer实例
	// conn 客户端连接，不需要客户代码关闭
	// err 错误
	OnWriteError(server *TCPServer, conn net.Conn, err error)
}

// TCPServer TCPServer结构
type TCPServer struct {
	receiveBufferSize uint

	listener net.Listener

	clientChannelMap      map[net.Conn]*clientChannel
	clientChannelMapMutex sync.RWMutex

	callbacks TCPServerCallbacks
}

type clientChannel struct {
	writeChannel chan []byte
	stopChannel  chan interface{}
}

// NewTCPServer 创建并启动TCPServer
func NewTCPServer(address string, receiveBufferSize uint, callbacks TCPServerCallbacks) (*TCPServer, error) {
	if utils.IsStringEmpty(address) {
		return nil, ErrParam
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	server := new(TCPServer)

	if receiveBufferSize == 0 {
		receiveBufferSize = defaultReceiveBufferSize
	}

	server.clientChannelMap = make(map[net.Conn]*clientChannel)
	server.receiveBufferSize = receiveBufferSize
	server.callbacks = callbacks
	server.listener = listener

	go server.run()

	return server, nil
}

// DestroyTCPServer 停止并销毁TCPServer
func DestroyTCPServer(server *TCPServer) {
	if server == nil {
		return
	}

	_ = server.listener.Close()
	server.deleteAllClientChannel()
	server = nil
}

// WriteData 向客户端写数据
func (server *TCPServer) WriteData(conn net.Conn, data []byte) {
	if conn == nil || data == nil || len(data) == 0 {
		return
	}

	cc := server.getClientChannel(conn)
	cc.writeChannel <- data
}

func (server *TCPServer) run() {
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			break
		}

		go server.handleConn(conn)
	}
}

func (server *TCPServer) handleConn(conn net.Conn) {
	if server.callbacks != nil {
		server.callbacks.OnConnect(server, conn)
	}

	cc := &clientChannel{
		writeChannel: make(chan []byte),
		stopChannel:  make(chan interface{}),
	}

	server.addClientChannel(conn, cc)

	go server.readConn(conn)
	go server.writeConn(conn, cc.writeChannel)

	for {
		select {
		case <-cc.stopChannel:
			break
		}
	}
}

func (server *TCPServer) readConn(conn net.Conn) {
	defer server.deleteClientChannel(conn)

	for {
		data := make([]byte, server.receiveBufferSize)
		n, err := conn.Read(data)
		if err != nil {
			if server.callbacks != nil {
				server.callbacks.OnReceiveError(server, conn, err)
			}

			break
		}

		if server.callbacks != nil {
			server.callbacks.OnReceive(server, conn, data, n)
		}
	}
}

func (server *TCPServer) writeConn(conn net.Conn, writeChan <-chan []byte) {
	defer server.deleteClientChannel(conn)

	for {
		data := <-writeChan
		n, err := conn.Write(data)
		if err != nil {
			if server.callbacks != nil {
				server.callbacks.OnWriteError(server, conn, err)
			}

			break
		}

		if server.callbacks != nil {
			server.callbacks.OnWrite(server, conn, n)
		}
	}
}

func (server *TCPServer) getClientChannel(conn net.Conn) *clientChannel {
	server.clientChannelMapMutex.RLock()
	defer server.clientChannelMapMutex.RUnlock()

	return server.clientChannelMap[conn]
}

func (server *TCPServer) addClientChannel(conn net.Conn, cc *clientChannel) {
	server.clientChannelMapMutex.Lock()
	defer server.clientChannelMapMutex.Unlock()

	server.clientChannelMap[conn] = cc
}

func (server *TCPServer) deleteClientChannel(conn net.Conn) {
	server.clientChannelMapMutex.Lock()

	cc := server.clientChannelMap[conn]
	delete(server.clientChannelMap, conn)

	server.clientChannelMapMutex.Unlock()

	close(cc.stopChannel)
	close(cc.writeChannel)
	_ = conn.Close()
}

func (server *TCPServer) deleteAllClientChannel() {
	server.clientChannelMapMutex.Lock()
	defer server.clientChannelMapMutex.Unlock()

	for conn, cc := range server.clientChannelMap {
		close(cc.stopChannel)
		close(cc.writeChannel)
		_ = conn.Close()
	}

	server.clientChannelMap = nil
}
