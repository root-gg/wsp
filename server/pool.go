package server

import (
	"errors"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// ErrPoolFull is returned when there is enough connection in the pool
var ErrPoolFull = errors.New("Pool is full")

// Pool handle all connections from a remove Proxy
type Pool struct {
	id string

	size        int
	connections []*Connection
	lock        sync.RWMutex

	done chan (struct{})
}

// NewPool creates a new Pool
func NewPool(name string) (pool *Pool) {
	pool = new(Pool)
	pool.id = name
	pool.connections = make([]*Connection, 0)
	return
}

// Register creates a new Connection and adds it to the pool
func (pool *Pool) Register(ws *websocket.Conn) (err error) {
	log.Printf("Registering new connection from %s", pool.id)
	pc := NewConnection(pool, ws)
	err = pool.Offer(pc)
	return
}

// Offer adds a connection to the pool
func (pool *Pool) Offer(connection *Connection) (err error) {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	size := pool.clean()
	if size < pool.size {
		pool.connections = append(pool.connections, connection)
	} else {
		err = ErrPoolFull
		log.Printf("Discarding connection from %s because pool is full", pool.id)
		connection.Close()
	}

	return
}

// Take borrow a connection from the pool
func (pool *Pool) Take() (pc *Connection) {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	size := pool.clean()
	if size == 0 {
		return
	}

	// Shift
	pc, pool.connections = pool.connections[0], pool.connections[1:]

	return
}

// Look for dead connection in the pool
// This MUST be surrounded by pool.lock.Lock()
func (pool *Pool) clean() int {
	if len(pool.connections) == 0 {
		return 0
	}

	var connections []*Connection // == nil
	for _, pc := range pool.connections {
		if pc.status == IDLE {
			connections = append(connections, pc)
		}
	}
	pool.connections = connections

	return len(pool.connections)
}

// Look for dead connection in the pool and return true if the pool is empty
func (pool *Pool) isEmpty() bool {
	pool.lock.Lock()
	defer pool.lock.Unlock()
	return pool.clean() == 0
}

// Shutdown closes every connections in the pool and cleans it
func (pool *Pool) Shutdown() {
	for _, connection := range pool.connections {
		connection.Close()
	}
	pool.clean()
}
