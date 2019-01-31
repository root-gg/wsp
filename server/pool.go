package server

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/root-gg/wsp/common"
)

// Pool handle all connections from a remote Proxy
type Pool struct {
	server *Server

	clientSettings *common.ClientSettings

	// This channel provides idle connection to the server
	// The server must then call Take() to make sure it is
	// still open and make it ready to use
	idle chan *Connection

	// This map is only here to provide a way to display statistics
	// about the connections in the pool
	connections     map[*Connection]struct{}
	connectionsLock sync.Mutex

	done chan struct{}
	lock sync.RWMutex
}

// NewPool creates a new Pool
func NewPool(server *Server, clientSettings *common.ClientSettings) (pool *Pool) {
	pool = new(Pool)
	pool.server = server
	pool.clientSettings = clientSettings
	pool.idle = make(chan *Connection)
	pool.done = make(chan struct{})
	pool.connections = make(map[*Connection]struct{})

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

	// Keep track of the connection to be able to display statistics
	pool.connectionsLock.Lock()
	pool.connections[connection] = struct{}{}
	pool.connectionsLock.Unlock()

	// Automatically remove connection from the map on close
	go func() {
		<-connection.done
		log.Printf("Connection %d from %s (%s) has been closed", id, pool.clientSettings.Name, pool.clientSettings.ID)
		pool.connectionsLock.Lock()
		delete(pool.connections, connection)
		pool.connectionsLock.Unlock()
	}()

	pool.offer(connection)

	return
}

// Offer an idle connection to the server
func (pool *Pool) offer(connection *Connection) {
	go func() {
		select {
		case pool.idle <- connection:
		case <-connection.done:
		case <-pool.done:
		}
	}()
}

// Clean tries to keep at most poolSize idle connection in the pool.
// Connections are left open for Config.IdleTimeout before being closed.
// Only the server is allowed to close connection to avoid the client
// to close a connection about to be used to proxy a request.
func (pool *Pool) clean() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	var connections []*Connection

LOOP:
	for {
		select {
		case conn := <-pool.idle:
			if conn.getStatus() != IDLE {
				continue
			}
			if len(connections) < pool.clientSettings.PoolSize {
				connections = append(connections, conn)
			} else {
				if time.Now().Sub(conn.idleSince) > pool.server.Config.IdleTimeout {
					conn.close()
				} else {
					connections = append(connections, conn)
				}
			}
		default:
			break LOOP
		}
	}

	for _, conn := range connections {
		pool.offer(conn)
	}

	return
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
		//log.Println("empty idle chan")
		select {
		case conn := <-pool.idle:
			conn.close()
		default:
			break LOOP
		}
	}
	log.Println("pool closed")
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
	pool.connectionsLock.Lock()
	defer pool.connectionsLock.Unlock()

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
