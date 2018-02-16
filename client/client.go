package client

import (
	"crypto/tls"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/root-gg/wsp/common"
	"log"
)

// Client connects to one or more Server using HTTP WebSocket
// The Server can then send HTTP requests to execute
type Client struct {
	Config    *Config
	validator *common.RequestValidator

	httpClient *http.Client
	dialer     *websocket.Dialer
	pools      map[string]*Pool
}

// NewClient creates a new Proxy
func NewClient(config *Config) (client *Client) {
	client = new(Client)
	client.Config = config

	client.validator = &common.RequestValidator{
		Whitelist: config.Whitelist,
		Blacklist: config.Blacklist,
	}
	err := client.validator.Initialize()
	if err != nil {
		log.Fatalf("Unable to initialize the request validator : %s", err)
	}

	// WebSocket tcp dialer to connect to the remote WSP servers
	client.dialer = &websocket.Dialer{}

	// HTTP client to execute HTTP requests received by the WebSocket tunnels
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: client.Config.InsecureSkipVerify},
	}
	client.httpClient = &http.Client{Transport: tr}

	client.pools = make(map[string]*Pool)

	return
}

// Start the Proxy
func (client *Client) Start() {
	for _, target := range client.Config.Targets {
		pool := NewPool(client, target)
		client.pools[target] = pool
		pool.start()
	}
}

// Shutdown the Proxy
func (client *Client) Shutdown() {
	for _, pool := range client.pools {
		pool.close()
	}

}
