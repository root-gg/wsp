package client

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Pool manage a pool of connection to a remote Server
type Pool struct {
	client *Client
	target string

	connections []*Connection
	lock        sync.RWMutex

	done chan struct{}
}

// NewPool creates a new Pool
func NewPool(client *Client, target string) (pool *Pool) {
	pool = new(Pool)
	pool.client = client
	pool.target = target
	pool.connections = make([]*Connection, 0)
	pool.done = make(chan struct{})
	return
}

// Start connect to the remote Server
func (pool *Pool) Start() {
	pool.connector()
	go func() {
		ticker := time.Tick(time.Second)
		for {
			select {
			case <-pool.done:
				break
			case <-ticker:
				pool.connector()
			}
		}
	}()
}

// The garbage collector
func (pool *Pool) connector() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	poolSize := pool.Size()

	log.Printf("%s pool size : %v", pool.target, poolSize)

	// Create enough connection to fill the pool
	toCreate := pool.client.Config.PoolIdleSize - poolSize.idle

	// Create only one connection if the pool is empty
	if poolSize.total == 0 {
		toCreate = 1
	}

	// Ensure to open at most PoolMaxSize connections
	if poolSize.total+toCreate > pool.client.Config.PoolMaxSize {
		toCreate = pool.client.Config.PoolMaxSize - poolSize.total
	}

	//log.Printf("%v",toCreate)

	// Try to reach ideal pool size
	for i := 0; i < toCreate; i++ {
		conn := NewConnection(pool)
		pool.connections = append(pool.connections, conn)

		go func() {
			err := conn.Connect()
			if err != nil {
				log.Printf("Unable to connect to %s : %s", pool.target, err)

				pool.lock.Lock()
				defer pool.lock.Unlock()
				pool.remove(conn)
			}
		}()
	}
}

// Add a connection to the pool
func (pool *Pool) add(conn *Connection) {
	pool.connections = append(pool.connections, conn)
}

// Remove a connection from the pool
func (pool *Pool) remove(conn *Connection) {
	// This trick uses the fact that a slice shares the same backing array and capacity as the original,
	// so the storage is reused for the filtered slice. Of course, the original contents are modified.

	var filtered []*Connection // == nil
	for _, c := range pool.connections {
		if conn != c {
			filtered = append(filtered, c)
		}
	}
	pool.connections = filtered
}

// Shutdown close all connection in the pool
func (pool *Pool) Shutdown() {
	close(pool.done)
	for _, conn := range pool.connections {
		conn.Close()
	}
}

// PoolSize represent the number of open connections per status
type PoolSize struct {
	connecting int
	idle       int
	running    int
	total      int
}

func (poolSize *PoolSize) String() string {
	return fmt.Sprintf("Connecting %d, idle %d, running %d, total %d", poolSize.connecting, poolSize.idle, poolSize.running, poolSize.total)
}

// Size return the current state of the pool
func (pool *Pool) Size() (poolSize *PoolSize) {
	poolSize = new(PoolSize)
	poolSize.total = len(pool.connections)
	for _, connection := range pool.connections {
		switch connection.status {
		case CONNECTING:
			poolSize.connecting++
		case IDLE:
			poolSize.idle++
		case RUNNING:
			poolSize.running++
		}
	}

	return
}
