package server

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"

	"github.com/root-gg/wsp/common"
)

// Server is a Reverse HTTP Proxy over WebSocket
// This is the Server part, Clients will offer websocket connections,
// those will be pooled to transfer HTTP Request and response
type Server struct {
	Config *Config

	validator  *common.RequestValidator
	upgrader   websocket.Upgrader
	httpServer *http.Server

	pools map[*Pool]struct{}
	lock  deadlock.RWMutex
	done  chan struct{}
}

// NewServer return a new Server instance
func NewServer(config *Config) (server *Server) {
	deadlock.Opts.Disable = false

	server = new(Server)
	server.Config = config

	server.validator = &common.RequestValidator{
		Whitelist: config.Whitelist,
		Blacklist: config.Blacklist,
	}
	err := server.validator.Initialize()
	if err != nil {
		log.Fatalf("Unable to initialize the request validator : %s", err)
	}

	server.upgrader = websocket.Upgrader{}

	server.pools = make(map[*Pool]struct{})
	server.done = make(chan struct{})
	return
}

// Start Server HTTP server
func (server *Server) Start() {
	server.lock.Lock()
	defer server.lock.Unlock()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-server.done:
				break
			case <-ticker.C:
				server.stats()
			}
		}
	}()

	r := http.NewServeMux()
	r.HandleFunc("/request", server.request)
	r.HandleFunc("/register", server.register)
	r.HandleFunc("/status", server.status)

	server.httpServer = &http.Server{Addr: server.Config.Host + ":" + strconv.Itoa(server.Config.Port), Handler: r}
	go func() { server.httpServer.ListenAndServe() }()
}

// clean remove empty Pools
func (server *Server) stats() {
	server.lock.Lock()
	defer server.lock.Unlock()

	idle := 0
	busy := 0

	for pool := range server.pools {
		poolSize := pool.Size()
		idle += poolSize.Idle
		busy += poolSize.Busy
	}

	log.Printf("%d pools, %d idle, %d busy", len(server.pools), idle, busy)
}

// Get a timeout timer to get a connection
func (server *Server) getTimeout() <-chan time.Time {
	if server.Config.Timeout > 0 {
		return time.After(server.Config.Timeout)
	}
	return nil
}

// Get a ws connection from one of the available pools
func (server *Server) getConnection() *Connection {
	timeout := server.getTimeout()
	for {
		server.lock.RLock()
		poolCount := len(server.pools)
		server.lock.RUnlock()

		if poolCount == 0 {
			// No connection pool available
			select {
			case <-timeout:
				// a timeout occurred
				return nil
			default:
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		// Build a select statement dynamically
		// This allows to wait on multiple connection pools for the next idle connection
		var cases []reflect.SelectCase

		// Add all pools idle connection channel
		// range makes a copy so no need to lock
		server.lock.RLock()
		for pool := range server.pools {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(pool.stack.out)})
		}
		server.lock.RUnlock()

		// Add a timeout channel or default case
		if timeout != nil {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(timeout)})
		} else {
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectDefault})
		}

		chosen, value, ok := reflect.Select(cases)

		if chosen == len(cases)-1 {
			// a timeout occurred
			return nil
		}
		if !ok {
			// a proxy pool has been removed, try again
			continue
		}
		connection, _ := value.Interface().(*Connection)

		// Verify that we can use this connection
		if connection.take() {
			return connection
		}
	}

	return nil
}

// This is the way for clients to execute HTTP requests through an Proxy
func (server *Server) request(resp http.ResponseWriter, req *http.Request) {
	// Parse destination URL
	dstURL := req.Header.Get("X-PROXY-DESTINATION")
	if dstURL == "" {
		common.ProxyErrorf(resp, "Missing X-PROXY-DESTINATION header")
		return
	}
	URL, err := url.Parse(dstURL)
	if err != nil {
		common.ProxyErrorf(resp, "Unable to parse X-PROXY-DESTINATION header")
		return
	}
	req.URL = URL

	log.Printf("[%s] %s", req.Method, req.URL.String())

	err = server.validator.Validate(req)
	if err != nil {
		common.ProxyErrorf(resp, fmt.Sprintf("Invalid request : %s", err.Error()))
		return
	}

	// Get a proxy connection
	connection := server.getConnection()
	if connection == nil {
		common.ProxyErrorf(resp, "Unable to get a proxy connection")
		return
	}

	// Send the request to the proxy
	err = connection.proxyRequest(resp, req)
	if err != nil {
		// An error occurred throw the connection away
		log.Println(err)
		connection.close()

		// Try to return an error to the client
		// This might fail if response headers have already been sent
		common.ProxyError(resp, err)
	}
}

// This is the way for wsp clients to offer websocket connections
func (server *Server) register(w http.ResponseWriter, r *http.Request) {
	secretKey := r.Header.Get("X-SECRET-KEY")
	if secretKey != server.Config.SecretKey {
		common.ProxyErrorf(w, "Invalid X-SECRET-KEY")
		return
	}

	ws, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		common.ProxyErrorf(w, "HTTP upgrade error : %v", err)
		return
	}

	// The first message should contains the remote Proxy name and size
	_, greeting, err := ws.ReadMessage()
	if err != nil {
		common.ProxyErrorf(w, "Unable to read  client settings : %s", err)
		ws.Close()
		return
	}

	// Parse the greeting message
	clientSettings, err := common.ClientSettingsFromJson(greeting)
	if err != nil {
		common.ProxyErrorf(w, "Unable to parse client settings : %s", err)
		ws.Close()
		return
	}

	server.lock.Lock()
	defer server.lock.Unlock()

	// Get that client's Pool
	var pool *Pool
	for p := range server.pools {
		if p.clientSettings.ID == clientSettings.ID {
			pool = p
			break
		}
	}
	if pool == nil {
		pool = NewPool(server, clientSettings)
		server.pools[pool] = struct{}{}

		// Automatically remove pool on close
		go func() {
			<-pool.done
			log.Printf("Removing empty connection pool : %s (%s)", pool.clientSettings.Name, pool.clientSettings.ID)
			server.lock.Lock()
			defer server.lock.Unlock()
			delete(server.pools, pool)
		}()
	}

	// Add the ws to the pool
	pool.register(clientSettings.ConnectionId, ws)
}

func (server *Server) status(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func (server *Server) IsClosed() bool {
	select {
	case <-server.done:
		return true
	default:
		return false
	}
}

// Close stop the WSP Server
func (server *Server) Shutdown() {
	server.lock.Lock()
	defer server.lock.Unlock()

	if server.IsClosed() {
		return
	}

	close(server.done)

	if server.httpServer != nil {
		server.httpServer.Close()
	}

	for pool := range server.pools {
		pool.close()
	}
}
