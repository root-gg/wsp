package server

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Pool handle all connections from a remote Proxy
type Pool struct {
	server *Server
	id     string

	size int

	connections []*Connection
	idle        chan *Connection

	lock sync.RWMutex
}

// NewPool creates a new Pool
func NewPool(server *Server, id string) (pool *Pool) {
	pool = new(Pool)
	pool.server = server
	pool.id = id
	pool.idle = make(chan *Connection)
	return
}

// Register creates a new Connection and adds it to the pool
func (pool *Pool) Register(ws *websocket.Conn) (err error) {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	log.Printf("Registering new connection from %s", pool.id)
	connection := NewConnection(pool, ws)
	pool.connections = append(pool.connections, connection)
	return
}

// Offer an idle connection to the server
func (pool *Pool) Offer(connection *Connection) {
	pool.idle <- connection
}

// Look for dead connection in the pool
// This MUST be surrounded by pool.lock.Lock()
func (pool *Pool) clean() {
	idle := 0
	var connections []*Connection
	for _, connection := range pool.connections {
		// We need to be sur we'll never close a BUSY or soon to be BUSY connection
		connection.lock.Lock()
		if connection.status == IDLE {
			idle++
			if idle > pool.size {
				// We have enough idle connections in the pool.
				// Terminate the connection if it is idle since more that IdleTimeout
				if int(time.Now().Sub(connection.idleSince).Seconds())*1000 > pool.server.Config.IdleTimeout {
					connection.close()
				}
			}
		}
		connection.lock.Unlock()
		if connection.status == CLOSED {
			continue
		}
		connections = append(connections, connection)
	}
	pool.connections = connections
}

// Clean removes dead connection from the pool and terminate idle connections if needed
func (pool *Pool) Clean() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	pool.clean()
}

// IsEmpty clean the pool and return true if the pool is empty
func (pool *Pool) IsEmpty() bool {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	pool.clean()
	return len(pool.connections) == 0
}

// Shutdown closes every connections in the pool and cleans it
func (pool *Pool) Shutdown() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	for _, connection := range pool.connections {
		connection.Close()
	}
	pool.clean()

}
