package tcp_server

import (
	"github.com/yjp19871013/event-bus/eventbus"
	"net"
	"sync"
	"tcp-server/utils"
	"time"
)

const (
	startBlockEventCount  = 10
	dealEventRoutingCount = 10
)

const (
	defaultReceiveBufferSize     = 4 * 1024
	defaultWriteRetryPeriodSec   = 10
	defaultReceiveRetryPeriodSec = 10
	writeChannelBuffer           = 1 * 1024
)

type tcpServerConfig struct {
	ReceiveBufferSize     uint
	WriteRetryCount       uint
	WriteRetryPeriodSec   uint
	ReceiveRetryCount     uint
	ReceiveRetryPeriodSec uint
}

func NewTCPServerConfig() *tcpServerConfig {
	return &tcpServerConfig{
		ReceiveBufferSize:     defaultReceiveBufferSize,
		WriteRetryCount:       0,
		WriteRetryPeriodSec:   defaultWriteRetryPeriodSec,
		ReceiveRetryCount:     0,
		ReceiveRetryPeriodSec: defaultReceiveRetryPeriodSec,
	}
}

// TCPServer TCPServer结构
type TCPServer struct {
	conf *tcpServerConfig

	listener net.Listener

	clientInstanceMap      map[net.Conn]*clientInstance
	clientInstanceMapMutex sync.RWMutex
}

// NewTCPServer 创建并启动TCPServer
func NewTCPServer(conf *tcpServerConfig, address string) (*TCPServer, error) {
	if conf == nil || utils.IsStringEmpty(address) {
		return nil, ErrParam
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	eventbus.InitEventBus(startBlockEventCount, dealEventRoutingCount)

	server := new(TCPServer)
	server.conf = conf
	server.clientInstanceMap = make(map[net.Conn]*clientInstance)
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

	eventbus.DestroyEventBus()
}

type clientInstance struct {
	writeChannel chan []byte
	stopChannel  chan interface{}
}

// WriteData 向客户端写数据
func (server *TCPServer) WriteData(conn net.Conn, data []byte) {
	if conn == nil || data == nil || len(data) == 0 {
		return
	}

	server.writeClientChannel(conn, data)
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
	ci := &clientInstance{
		writeChannel: make(chan []byte, writeChannelBuffer),
		stopChannel:  make(chan interface{}),
	}

	server.addClientChannel(conn, ci)

	go server.readConn(conn, server.conf.ReceiveRetryCount, server.conf.ReceiveRetryPeriodSec)
	go server.writeConn(conn, ci.writeChannel, server.conf.WriteRetryCount, server.conf.WriteRetryPeriodSec)

	eventbus.GetBus().Publish(&OnConnectEvent{
		Server: server,
		Conn:   conn,
	})

	for {
		select {
		case <-ci.stopChannel:
			break
		}
	}
}

func (server *TCPServer) readConn(conn net.Conn, retryCount uint, retryPeriodSec uint) {
	defer server.deleteClientChannel(conn)

	for {
		data := make([]byte, server.conf.ReceiveBufferSize)
		n, err := conn.Read(data)
		if err != nil {
			eventbus.GetBus().Publish(&OnReceiveErrorEvent{
				Server: server,
				Conn:   conn,
				Err:    err,
			})

			if retryCount != 0 {
				retryCount--
				time.Sleep(time.Second * time.Duration(retryPeriodSec))
				continue
			}

			break
		}

		eventbus.GetBus().Publish(&OnReceiveEvent{
			Server:   server,
			Conn:     conn,
			Data:     data,
			ReadSize: n,
		})
	}
}

func (server *TCPServer) writeConn(conn net.Conn, writeChan <-chan []byte, retryCount uint, retryPeriodSec uint) {
	defer server.deleteClientChannel(conn)

	for {
		data := <-writeChan
		n, err := conn.Write(data)
		if err != nil {
			eventbus.GetBus().Publish(&OnWriteErrorEvent{
				Server: server,
				Conn:   conn,
				Err:    err,
			})

			if retryCount != 0 {
				retryCount--
				time.Sleep(time.Second * time.Duration(retryPeriodSec))
				continue
			}

			break
		}

		eventbus.GetBus().Publish(&OnWriteEvent{
			Server:    server,
			Conn:      conn,
			WriteSize: n,
		})
	}
}

func (server *TCPServer) writeClientChannel(conn net.Conn, data []byte) {
	server.clientInstanceMapMutex.RLock()
	defer server.clientInstanceMapMutex.RUnlock()

	ci, ok := server.clientInstanceMap[conn]
	if !ok {
		return
	}

	ci.writeChannel <- data
}

func (server *TCPServer) addClientChannel(conn net.Conn, ci *clientInstance) {
	server.clientInstanceMapMutex.Lock()
	defer server.clientInstanceMapMutex.Unlock()

	server.clientInstanceMap[conn] = ci
}

func (server *TCPServer) deleteClientChannel(conn net.Conn) {
	server.clientInstanceMapMutex.Lock()

	ci, ok := server.clientInstanceMap[conn]
	if !ok {
		server.clientInstanceMapMutex.Unlock()
		return
	}

	delete(server.clientInstanceMap, conn)

	server.clientInstanceMapMutex.Unlock()

	close(ci.stopChannel)
	ci.stopChannel = nil

	close(ci.writeChannel)
	ci.writeChannel = nil

	_ = conn.Close()
}

func (server *TCPServer) deleteAllClientChannel() {
	server.clientInstanceMapMutex.Lock()
	defer server.clientInstanceMapMutex.Unlock()

	for conn, ci := range server.clientInstanceMap {
		close(ci.stopChannel)
		ci.stopChannel = nil

		close(ci.writeChannel)
		ci.writeChannel = nil

		_ = conn.Close()
		conn = nil
	}

	server.clientInstanceMap = nil
}
