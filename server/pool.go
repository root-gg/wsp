package server

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"

	"github.com/root-gg/wsp/common"
)

// Pool handle all connections from a remote Proxy
type Pool struct {
	server *Server

	clientSettings *common.ClientSettings

	// This map is only here to provide a way to display statistics
	// about the connections in the pool
	connections map[*Connection]struct{}

	stack *ConnectionStack

	done chan struct{}
	lock deadlock.Mutex
}

// NewPool creates a new Pool
func NewPool(server *Server, clientSettings *common.ClientSettings) (pool *Pool) {
	pool = new(Pool)
	pool.server = server
	pool.clientSettings = clientSettings
	pool.stack = newConnectionStack()
	pool.done = make(chan struct{})
	pool.connections = make(map[*Connection]struct{})

	ticker := time.NewTicker(time.Second)
	go func() {
	LOOP:
		for {
			select {
			case <-ticker.C:
				pool.clean()
			case <-pool.done:
				break LOOP
			}
		}
	}()

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

	// Keep track of the connections in the pool
	pool.connections[connection] = struct{}{}

	// Automatically remove connection from the pool on close
	go func() {
		<-connection.done
		log.Printf("Connection %d from %s (%s) has been closed", id, pool.clientSettings.Name, pool.clientSettings.ID)
		pool.lock.Lock()
		defer pool.lock.Unlock()
		delete(pool.connections, connection)
	}()

	// Jump that lock
	pool.offer(connection)

	return
}

// Offer an idle connection to the server
func (pool *Pool) offer(connection *Connection) {
	pool.stack.in <- connection
}

// Clean tries to keep at most poolSize idle connection in the pool.
// Connections are left open for Config.IdleTimeout before being closed.
// Only the server is allowed to close connection to avoid that the client
// closes a connection about to be used to proxy a request.
func (pool *Pool) clean() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	if len(pool.connections) == 0 {
		// Close an empty pool
		close(pool.done)
		pool.stack.close()
		return
	}

	idle := 0
	for conn := range pool.connections {
		// Remove CLOSED connections
		status, idleSince := conn.getStatus()
		if status != IDLE {
			continue
		}
		idle++
		// Keep at most PoolSize IDLE connection
		if idle <= pool.clientSettings.PoolSize {
			continue
		}
		// Grace period before closing connection > PoolSize
		if time.Since(idleSince) > pool.server.Config.IdleTimeout {
			conn.close()
		}
	}
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

	pool.stack.close()

	for conn := range pool.connections {
		conn.close()
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
	pool.lock.Lock()
	defer pool.lock.Unlock()

	ps = new(PoolSize)
	for connection := range pool.connections {
		status, _ := connection.getStatus()
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
