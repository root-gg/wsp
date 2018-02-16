package client

import (
	"fmt"
	"github.com/root-gg/wsp/common"
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

	deadline *time.Time

	connectionStatusListner *ConnectionStatusListner
	done                    chan struct{}
}

// NewPool creates a new Pool
func NewPool(client *Client, target string) (pool *Pool) {
	pool = new(Pool)
	pool.client = client
	pool.target = target
	pool.connections = make([]*Connection, 0)
	pool.connectionStatusListner = NewConnectionStatusListner()
	pool.done = make(chan struct{})
	return
}

// Start connect to the remote Server
func (pool *Pool) start() {

	// Try to open new connections to reach the desired pool size as fast as possible
	// Normally the pool is filled right away by the go conn.pool.connector()
	// triggered when a connection is about to be used but te ticker is here to speed things up if needed

	go func() {
		// Bootstrap
		pool.connector()

		ticker := time.Tick(time.Second)
		for {
			select {
			case <-pool.done:
				break
			case <-ticker:
				pool.connector()
			case <-pool.connectionStatusListner.connectionStatusChanged():
				pool.connector()
			}
		}
	}()
}

// The connector is responsible to open the connections to the remote WSP Server
// It tries to keep Config.PoolIdleSize connecting/idle connections besides the running
// ones and will take care to never exceed Config.PoolMaxSize open connections
func (pool *Pool) connector() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	if pool.isClosed() {
		return
	}

	// Remove closed connections
	pool.clean()

	poolSize := pool.size()
	log.Printf("%s pool size : %v", pool.target, poolSize)

	// Number of missing idle connection to reach the ideal pool size
	missing := pool.client.Config.PoolIdleSize - poolSize.idle

	// If the pool is empty only try to create one single connection
	isEmpty := poolSize.idle+poolSize.running == 0
	if isEmpty {
		missing = 1

		//// ratelimit connection
		//if pool.deadline != nil {
		//	time.Sleep(pool.deadline.Sub(time.Now()))
		//}
		//deadline := time.Now().Add(1000 * time.Millisecond)
		//pool.deadline = &deadline
	}

	// Ensure to open at most PoolMaxSize connections
	if poolSize.total+missing > pool.client.Config.PoolMaxSize {
		missing = pool.client.Config.PoolMaxSize - poolSize.total
	}

	// Remove already in-flight connections
	toCreate := missing - poolSize.connecting

	// Try to reach ideal pool size
	for i := 0; i < toCreate; i++ {
		clientSettings := &common.ClientSettings{
			ID:       pool.client.Config.ID,
			Name:     pool.client.Config.Name,
			PoolSize: pool.client.Config.PoolIdleSize,
		}
		conn := newConnection(clientSettings, pool.connectionStatusListner)

		// Append connection to the pool before trying to connect
		// so in-flight connection can appear in poolSize
		// Anyway nobody will ever get a connection from this pool,
		// this is the only way to add a connection to the pool and
		// the only way to remove a connection from the pool is
		// pool.clean() which is called the at the beginning of this method.
		pool.connections = append(pool.connections, conn)

		go func() {
			defer conn.close()

			err := conn.connect(pool.client.dialer, pool.target, pool.client.Config.SecretKey)
			if err != nil {
				log.Printf("Unable to establish connection %d to %s : %s", conn.clientSettings.ConnectionId, pool.target, err)
				return
			}

			err = conn.initialize()
			if err != nil {
				log.Printf("Unable to connection %d to %s: %s", conn.clientSettings.ConnectionId, pool.target, err)
				return
			}

			// This call blocks
			conn.serve(pool.client.httpClient, pool.client.validator)
		}()
	}
}

// Remove closed connections from the pool
func (pool *Pool) clean() {
	var filtered []*Connection
	for _, conn := range pool.connections {
		if conn.getStatus() != CLOSED {
			filtered = append(filtered, conn)
		}
	}
	pool.connections = filtered
}

// Check if the pool has been closed
func (pool *Pool) isClosed() bool {
	select {
	case <-pool.done:
		return true
	default:
		return false
	}
}

// Close all connection in the pool and be sure we don't use it again
func (pool *Pool) close() {
	pool.lock.Lock()
	defer pool.lock.Unlock()

	close(pool.done)
	for _, conn := range pool.connections {
		conn.close()
	}
}

// PoolSize represent the number of open connections per status
type PoolSize struct {
	connecting int
	idle       int
	running    int
	closed     int
	total      int
}

func (poolSize *PoolSize) String() string {
	return fmt.Sprintf("Connecting %d, idle %d, running %d, closed %d, total %d", poolSize.connecting, poolSize.idle, poolSize.running, poolSize.closed, poolSize.total)
}

// Size return the current state of the pool
func (pool *Pool) size() (poolSize *PoolSize) {
	poolSize = new(PoolSize)
	poolSize.total = len(pool.connections)
	for _, connection := range pool.connections {
		switch connection.getStatus() {
		case CONNECTING:
			poolSize.connecting++
		case IDLE:
			poolSize.idle++
		case RUNNING:
			poolSize.running++
		case CLOSED:
			poolSize.closed++
		}
	}

	return
}
