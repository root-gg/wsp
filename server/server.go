package server

import (
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/root-gg/wsp"
)

// Server is a Reverse HTTP Proxy over WebSocket
// This is the Server part, Clients will offer websocket connections,
// those will be pooled to transfer HTTP Request and response
type Server struct {
	Config *Config

	upgrader websocket.Upgrader

	pools []*Pool
	lock  sync.RWMutex
	done  chan struct{}

	dispatcher chan *ConnectionRequest

	server *http.Server
}

// ConnectionRequest is used to request a proxy connection from the dispatcher
type ConnectionRequest struct {
	connection chan *Connection
	timeout    <-chan time.Time
}

// NewConnectionRequest creates a new connection request
func NewConnectionRequest(timeout time.Duration) (cr *ConnectionRequest) {
	cr = new(ConnectionRequest)
	cr.connection = make(chan *Connection)
	if timeout > 0 {
		cr.timeout = time.After(timeout)
	}
	return
}

// NewServer return a new Server instance
func NewServer(config *Config) (server *Server) {
	rand.Seed(time.Now().Unix())

	server = new(Server)
	server.Config = config
	server.upgrader = websocket.Upgrader{}

	server.done = make(chan struct{})
	server.dispatcher = make(chan *ConnectionRequest)
	return
}

// Start Server HTTP server
func (server *Server) Start() {
	go func() {
		for {
			select {
			case <-server.done:
				break
			case <-time.After(5 * time.Second):
				server.clean()
			}
		}
	}()

	r := http.NewServeMux()
	r.HandleFunc("/register", server.register)
	r.HandleFunc("/request", server.request)
	r.HandleFunc("/status", server.status)

	go server.dispatchConnections()

	server.server = &http.Server{
		Addr:    server.Config.GetAddr(),
		Handler: r,
	}
	go func() { log.Fatal(server.server.ListenAndServe()) }()
}

// clean remove empty Pools
func (server *Server) clean() {
	server.lock.Lock()
	defer server.lock.Unlock()

	if len(server.pools) == 0 {
		return
	}

	idle := 0
	busy := 0

	var pools []*Pool
	for _, pool := range server.pools {
		if pool.IsEmpty() {
			log.Printf("Removing empty connection pool : %s", pool.id)
			pool.Shutdown()
		} else {
			pools = append(pools, pool)
		}

		ps := pool.Size()
		idle += ps.Idle
		busy += ps.Busy
	}

	log.Printf("%d pools, %d idle, %d busy", len(pools), idle, busy)

	server.pools = pools
}

// Dispatch connection from available pools to clients requests
func (server *Server) dispatchConnections() {
	for {
		// A client requests a connection
		request, ok := <-server.dispatcher
		if !ok {
			// Shutdown
			break
		}

		for {
			server.lock.RLock()

			if len(server.pools) == 0 {
				// No connection pool available
				server.lock.RUnlock()
				break
			}

			// Build a select statement dynamically
			cases := make([]reflect.SelectCase, len(server.pools)+1)

			// Add all pools idle connection channel
			for i, ch := range server.pools {
				cases[i] = reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(ch.idle)}
			}

			// Add timeout channel
			if request.timeout != nil {
				cases[len(cases)-1] = reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(request.timeout)}
			} else {
				cases[len(cases)-1] = reflect.SelectCase{
					Dir: reflect.SelectDefault}
			}

			server.lock.RUnlock()

			// This call blocks
			chosen, value, ok := reflect.Select(cases)

			if chosen == len(cases)-1 {
				// a timeout occured
				break
			}
			if !ok {
				// a proxy pool has been removed, try again
				continue
			}
			connection, _ := value.Interface().(*Connection)

			// Verify that we can use this connection
			if connection.Take() {
				request.connection <- connection
				break
			}
		}

		close(request.connection)
	}
}

// This is the way for clients to execute HTTP requests through an Proxy
func (server *Server) request(w http.ResponseWriter, r *http.Request) {
	// Parse destination URL
	dstURL := r.Header.Get("X-PROXY-DESTINATION")
	if dstURL == "" {
		wsp.ProxyErrorf(w, "Missing X-PROXY-DESTINATION header")
		return
	}
	URL, err := url.Parse(dstURL)
	if err != nil {
		wsp.ProxyErrorf(w, "Unable to parse X-PROXY-DESTINATION header")
		return
	}
	r.URL = URL

	log.Printf("[%s] %s", r.Method, r.URL.String())

	if len(server.pools) == 0 {
		wsp.ProxyErrorf(w, "No proxy available")
		return
	}

	// Get a proxy connection
	request := NewConnectionRequest(time.Duration(server.Config.Timeout) * time.Millisecond)
	server.dispatcher <- request
	connection := <-request.connection
	if connection == nil {
		wsp.ProxyErrorf(w, "Unable to get a proxy connection")
		return
	}

	// Send the request to the proxy
	err = connection.proxyRequest(w, r)
	if err != nil {
		// An error occurred throw the connection away
		log.Println(err)
		connection.Close()

		// Try to return an error to the client
		// This might fail if response headers have already been sent
		wsp.ProxyError(w, err)
	}
}

// This is the way for wsp clients to offer websocket connections
func (server *Server) register(w http.ResponseWriter, r *http.Request) {
	secretKey := r.Header.Get("X-SECRET-KEY")
	if secretKey != server.Config.SecretKey {
		wsp.ProxyErrorf(w, "Invalid X-SECRET-KEY")
		return
	}

	ws, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		wsp.ProxyErrorf(w, "HTTP upgrade error : %v", err)
		return
	}

	// The first message should contains the remote Proxy name and size
	_, greeting, err := ws.ReadMessage()
	if err != nil {
		wsp.ProxyErrorf(w, "Unable to read greeting message : %s", err)
		ws.Close()
		return
	}

	// Parse the greeting message
	split := strings.Split(string(greeting), "_")
	id := split[0]
	size, err := strconv.Atoi(split[1])
	if err != nil {
		wsp.ProxyErrorf(w, "Unable to parse greeting message : %s", err)
		ws.Close()
		return
	}

	server.lock.Lock()
	defer server.lock.Unlock()

	// Get that client's Pool
	var pool *Pool
	for _, p := range server.pools {
		if p.id == id {
			pool = p
			break
		}
	}
	if pool == nil {
		pool = NewPool(server, id)
		server.pools = append(server.pools, pool)
	}

	// update pool size
	pool.size = size

	// Add the ws to the pool
	pool.Register(ws)
}

func (server *Server) status(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

// Shutdown stop the Server
func (server *Server) Shutdown() {
	close(server.done)
	close(server.dispatcher)
	for _, pool := range server.pools {
		pool.Shutdown()
	}
	server.clean()
}
