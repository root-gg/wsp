package server

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/root-gg/wsp/common"
)

// Pool handle all connections from a remote Proxy
type Pool struct {
	server *Server

	clientSettings *common.ClientSettings

	stack *Stack

	done chan struct{}
	lock sync.RWMutex
}

// NewPool creates a new Pool
func NewPool(server *Server, clientSettings *common.ClientSettings) (pool *Pool) {
	pool = new(Pool)
	pool.server = server
	pool.clientSettings = clientSettings
	pool.stack = newConnectionStack(clientSettings.PoolSize, server.Config.Timeout)
	pool.done = make(chan struct{})
	return
}

// Register creates a new Connection and adds it to the pool
func (pool *Pool) register(id uint64, ws *websocket.Conn) {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	// Ensure we never add a connection to a pool we have garbage collected
	if pool.isClosed() {
		return
	}

	log.Printf("Registering new connection %d from %s (%s)", id, pool.clientSettings.Name, pool.clientSettings.ID)

	connection := newConnection(id, ws, pool.offer)

	// Automatically remove connection from the map on close
	go func() {
		<-connection.done
		log.Printf("Connection %d from %s (%s) has been closed", id, pool.clientSettings.Name, pool.clientSettings.ID)
	}()

	pool.offer(connection)

	return
}

// Offer an idle connection to the server
func (pool *Pool) offer(connection *Connection) {
	pool.stack.offer(connection)
}

// isClosed returns true if the pool had been closed
func (pool *Pool) isClosed() bool {
	select {
	case <-pool.done:
		return true
	default:
		return false
	}
}

// Close every connections in the pool and clean it
func (pool *Pool) close() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	if pool.isClosed() {
		log.Println("pool alreadey closed")
		return
	}

	close(pool.done)

LOOP:
	for {
		select {
		case conn := <-pool.stack.out:
			conn.close()
		default:
			break LOOP
		}
	}
}

// PoolSize is the number of connection in each state in the pool
type PoolSize struct {
	Idle   int
	Busy   int
	Closed int
	Total  int
}

// Size return the number of connection in each state in the pool
func (pool *Pool) Size() (ps *PoolSize) {
	ps = new(PoolSize)
	for connection := range pool.connections {
		status := connection.getStatus()
		if status == IDLE {
			ps.Idle++
		} else if status == BUSY {
			ps.Busy++
		} else if status == CLOSED {
			ps.Closed++
		}
		ps.Total++
	}

	return
}
