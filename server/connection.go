package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/root-gg/wsp/common"
)

// Status of a Connection
const (
	IDLE = iota
	BUSY
	CLOSED
)

// Connection manage a single websocket connection from
type Connection struct {
	pool         *Pool
	ws           *websocket.Conn
	status       int
	idleSince    time.Time
	lock         sync.Mutex
	nextResponse chan chan io.Reader
}

// NewConnection return a new Connection
func NewConnection(pool *Pool, ws *websocket.Conn) (connection *Connection) {
	connection = new(Connection)
	connection.pool = pool
	connection.ws = ws
	connection.nextResponse = make(chan chan io.Reader)

	connection.Release()

	go connection.read()

	return
}

// read the incoming message of the connection
func (connection *Connection) read() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Websocket crash recovered : %s", r)
		}
		connection.Close()
	}()

	for {
		if connection.status == CLOSED {
			break
		}

		// https://godoc.org/github.com/gorilla/websocket#hdr-Control_Messages
		//
		// We need to ensure :
		//  - no concurrent calls to ws.NextReader() / ws.ReadMessage()
		//  - only one reader exists at a time
		//  - wait for reader to be consumed before requesting the next one
		//  - always be reading on the socket to be able to process control messages ( ping / pong / close )

		// We will block here until a message is received or the ws is closed
		_, reader, err := connection.ws.NextReader()
		if err != nil {
			break
		}

		if connection.status != BUSY {
			// We received a wild unexpected message
			break
		}

		// We received a message from the proxy
		// It is expected to be either a HttpResponse or a HttpResponseBody
		// We wait for proxyRequest to send a channel to get the message
		c := <-connection.nextResponse
		if c == nil {
			// We have been unlocked by Close()
			break
		}

		// Send the reader back to proxyRequest
		c <- reader

		// Wait for proxyRequest to close the channel
		// this notify that it is done with the reader
		<-c
	}
}

// Proxy a HTTP request through the Proxy over the websocket connection
func (connection *Connection) proxyRequest(w http.ResponseWriter, r *http.Request) (err error) {
	log.Printf("proxy request to %s", connection.pool.id)

	// Serialize HTTP request
	jsonReq, err := json.Marshal(common.SerializeHTTPRequest(r))
	if err != nil {
		return fmt.Errorf("Unable to serialize request : %s", err)
	}

	// Send the serialized HTTP request to the remote Proxy
	err = connection.ws.WriteMessage(websocket.TextMessage, jsonReq)
	if err != nil {
		return fmt.Errorf("Unable to write request : %s", err)
	}

	// Pipe the HTTP request body to the remote Proxy
	bodyWriter, err := connection.ws.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return fmt.Errorf("Unable to get request body writer : %s", err)
	}
	_, err = io.Copy(bodyWriter, r.Body)
	if err != nil {
		return fmt.Errorf("Unable to pipe request body : %s", err)
	}
	err = bodyWriter.Close()
	if err != nil {
		return fmt.Errorf("Unable to pipe request body (close) : %s", err)
	}

	// Get the serialized HTTP Response from the remote Proxy
	// To do so send a new channel to the read() goroutine
	// to get the next message reader
	responseChannel := make(chan (io.Reader))
	connection.nextResponse <- responseChannel
	responseReader, more := <-responseChannel
	if responseReader == nil {
		if more {
			// If more is false the channel is already closed
			close(responseChannel)
		}
		return fmt.Errorf("Unable to get http response reader : %s", err)
	}

	// Read the HTTP Response
	jsonResponse, err := ioutil.ReadAll(responseReader)
	if err != nil {
		close(responseChannel)
		return fmt.Errorf("Unable to read http response : %s", err)
	}

	// Notify the read() goroutine that we are done reading the response
	close(responseChannel)

	// Deserialize the HTTP Response
	httpResponse := new(common.HTTPResponse)
	err = json.Unmarshal(jsonResponse, httpResponse)
	if err != nil {
		return fmt.Errorf("Unable to unserialize http response : %s", err)
	}

	// Write response headers back to the client
	for header, values := range httpResponse.Header {
		for _, value := range values {
			w.Header().Add(header, value)
		}
	}
	w.WriteHeader(httpResponse.StatusCode)

	// Get the HTTP Response body from the remote Proxy
	// To do so send a new channel to the read() goroutine
	// to get the next message reader
	responseBodyChannel := make(chan (io.Reader))
	connection.nextResponse <- responseBodyChannel
	responseBodyReader, more := <-responseBodyChannel
	if responseBodyReader == nil {
		if more {
			// If more is false the channel is already closed
			close(responseChannel)
		}
		return fmt.Errorf("Unable to get http response body reader : %s", err)
	}

	// Pipe the HTTP response body right from the remote Proxy to the client
	_, err = io.Copy(w, responseBodyReader)
	if err != nil {
		close(responseBodyChannel)
		return fmt.Errorf("Unable to pipe response body : %s", err)
	}

	// Notify read() that we are done reading the response body
	close(responseBodyChannel)

	connection.Release()

	return
}

// Take notifies that this connection is going to be used
func (connection *Connection) Take() bool {
	connection.lock.Lock()
	defer connection.lock.Unlock()

	if connection.status == CLOSED {
		return false
	}

	if connection.status == BUSY {
		return false
	}

	connection.status = BUSY
	return true
}

// Release notifies that this connection is ready to use again
func (connection *Connection) Release() {
	connection.lock.Lock()
	defer connection.lock.Unlock()

	if connection.status == CLOSED {
		return
	}

	connection.idleSince = time.Now()
	connection.status = IDLE

	go connection.pool.Offer(connection)
}

// Close the connection
func (connection *Connection) Close() {
	connection.lock.Lock()
	defer connection.lock.Unlock()

	connection.close()
}

// Close the connection ( without lock )
func (connection *Connection) close() {
	if connection.status == CLOSED {
		return
	}

	log.Printf("Closing connection from %s", connection.pool.id)

	// This one will be executed *before* lock.Unlock()
	defer func() { connection.status = CLOSED }()

	// Unlock a possible read() wild message
	close(connection.nextResponse)

	// Close the underlying TCP connection
	connection.ws.Close()
}
