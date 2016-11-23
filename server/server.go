package server

import (
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/root-gg/wsp/common"
)

// Server is a Reverse HTTP Proxy over WebSocket
// This is the Server part IzolatorProxies will offer websocket connections,
// those will be pooled to transfer HTTP Request and response
type Server struct {
	Config *Config

	upgrader websocket.Upgrader

	// Remote IzolatorProxies
	pools []*Pool
	lock  sync.RWMutex
	done  chan struct{}

	server *http.Server
}

// NewServer return a new Server instance
func NewServer(config *Config) (server *Server) {
	rand.Seed(time.Now().Unix())

	server = new(Server)
	server.Config = config
	server.upgrader = websocket.Upgrader{}

	server.pools = make([]*Pool, 0)
	server.done = make(chan struct{})

	return
}

// Start Server HTTP server
func (server *Server) Start() {
	go func() {
		for {
			select {
			case <-server.done:
				break
			case <-time.After(30 * time.Second):
				server.clean()
			}
		}
	}()

	r := http.NewServeMux()
	r.HandleFunc("/request", server.request)
	r.HandleFunc("/register", server.register)
	r.HandleFunc("/status", server.status)

	server.server = &http.Server{Addr: server.Config.Host + ":" + strconv.Itoa(server.Config.Port), Handler: r}
	go func() { log.Fatal(server.server.ListenAndServe()) }()
}

// clean remove empty Pools
func (server *Server) clean() int {
	server.lock.Lock()
	defer server.lock.Unlock()

	if len(server.pools) == 0 {
		return 0
	}

	var pools []*Pool // == nil
	for _, pool := range server.pools {
		if pool.clean() > 0 {
			pools = append(pools, pool)
		} else {
			log.Printf("Removing empty connection pool : %s", pool.id)
		}
	}
	server.pools = pools

	return len(server.pools)
}

// This is the way for clients to execute HTTP requests through an Proxy
func (server *Server) request(w http.ResponseWriter, r *http.Request) {
	// Parse destination URL
	dstURL := r.Header.Get("X-PROXY-DESTINATION")
	if dstURL == "" {
		common.ProxyErrorf(w, "Missing X-PROXY-DESTINATION header")
		return
	}
	URL, err := url.Parse(dstURL)
	if err != nil {
		common.ProxyErrorf(w, "Unable to parse X-PROXY-DESTINATION header")
		return
	}
	r.URL = URL

	log.Printf("[%s] %s", r.Method, r.URL.String())

	// Apply blacklist

	if len(server.Config.Blacklist) > 0 {
		for _, rule := range server.Config.Blacklist {
			if rule.Match(r) {
				common.ProxyErrorf(w, "Destination is forbidden")
				return
			}
		}
	}

	// Apply whitelist

	if len(server.Config.Whitelist) > 0 {
		allowed := false
		for _, rule := range server.Config.Whitelist {
			if rule.Match(r) {
				allowed = true
				break
			}
		}
		if !allowed {
			common.ProxyErrorf(w, "Destination is not allowed")
			return
		}
	}

	if len(server.pools) == 0 {
		common.ProxyErrorf(w, "No proxy available")
		return
	}

	start := time.Now()

	// Randomly select a pool ( other load-balancing strategies could be implemented )
	// and try to acquire a connection. This should be refactored to use channels
	index := rand.Intn(len(server.pools))
	for {
		if time.Now().Sub(start).Seconds()*float64(1000) > float64(server.Config.Timeout) {
			break
		}

		// Get a pool
		server.lock.RLock()
		index = (index + 1) % len(server.pools)
		pool := server.pools[index]
		server.lock.RUnlock()

		// Get a connection
		if connection := pool.Take(); connection != nil {
			if connection.status != IDLE && time.Now().Sub(start).Seconds() < 1 {
				continue
			}

			// Send the request to the proxy
			err := connection.proxyRequest(w, r)
			if err == nil {
				// Everything went well we can reuse the connection
				pool.Offer(connection)
			} else {
				// An error occurred throw the connection away
				log.Println(err)
				connection.Close()

				// Try to return an error to the client
				// This might fail if response headers have already been sent
				common.ProxyError(w, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	common.ProxyErrorf(w, "Unable to get a proxy connection")
}

// This is the way for IzolatorProxies to offer websocket connections
func (server *Server) register(w http.ResponseWriter, r *http.Request) {
	ws, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		common.ProxyErrorf(w, "HTTP upgrade error : %v", err)
		return
	}

	// The first message should contains the remote Proxy name and size
	_, greeting, err := ws.ReadMessage()
	if err != nil {
		common.ProxyErrorf(w, "Unable to read greeting message : %s", err)
		ws.Close()
		return
	}

	// Parse the greeting message
	split := strings.Split(string(greeting), "_")
	id := split[0]
	size, err := strconv.Atoi(split[1])
	if err != nil {
		common.ProxyErrorf(w, "Unable to parse greeting message : %s", err)
		ws.Close()
		return
	}

	server.lock.Lock()
	defer server.lock.Unlock()

	// Get that Proxy websocket pool
	var pool *Pool
	for _, p := range server.pools {
		if p.id == id {
			pool = p
			break
		}
	}
	if pool == nil {
		pool = NewPool(id)
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
	for _, pool := range server.pools {
		pool.Shutdown()
	}
	server.clean()
}
