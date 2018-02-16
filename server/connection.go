package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"

	"github.com/root-gg/wsp/common"
)

// Status of a Connection
type ConnectionStatus int
type WSHandler func(reader io.Reader) error

const (
	IDLE ConnectionStatus = iota
	BUSY
	CLOSED
)

// Connection manage a single WebSocket connection from a WSP client
type Connection struct {
	id uint64
	ws *websocket.Conn

	status ConnectionStatus
	lock   deadlock.RWMutex

	nextReader chan func(io.Reader)

	releaser  func(conn *Connection)
	idleSince time.Time

	done chan struct{}
}

// newConnection return a new Connection
func newConnection(id uint64, ws *websocket.Conn, releaser func(conn *Connection)) (conn *Connection) {
	conn = new(Connection)
	conn.id = id
	conn.ws = ws
	conn.releaser = releaser
	conn.nextReader = make(chan func(io.Reader), 1)
	conn.done = make(chan struct{})

	go conn.read()

	return
}

// Get the status of the connection in a concurrently safe way
func (conn *Connection) getStatus() (ConnectionStatus, time.Time) {
	conn.lock.RLock()
	defer conn.lock.RUnlock()
	return conn.status, conn.idleSince
}

// Handle next message pass a function to process the next WebSocket message
// to the read goroutine. Only one message can be handled at a time.
// This method blocks until the handler has returned.
func (conn *Connection) handleNextMessage(h WSHandler) error {
	done := make(chan error)
	h2 := func(reader io.Reader) {
		done <- h(reader)
	}

	select {
	case conn.nextReader <- h2:
	case <-conn.done:
		return errors.New("connection closed")
	default:
		return errors.New("already reading")
	}

	select {
	case err := <-done:
		return err
	case <-conn.done:
		return errors.New("connection closed")
	}
}

// read the incoming message of the WebSocket connection
func (conn *Connection) read() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Websocket crash recovered : %s", r)
		}
		conn.close()
	}()

	for {
		// https://godoc.org/github.com/gorilla/websocket#hdr-Control_Messages
		//
		// We need to ensure :
		//  - no concurrent calls to ws.NextReader() / ws.ReadMessage()
		//  - only one reader exists at a time
		//  - wait for reader to be consumed before requesting the next one
		//  - always be reading on the socket to be able to process control messages ( ping / pong / closeNoLock )

		// We will block here until a message is received or the ws is closed
		_, ioReader, err := conn.ws.NextReader()
		if err != nil {
			if !conn.isClosed() {
				log.Printf("WebSocket error : %s", err)
			}
			break
		}

		status, _ := conn.getStatus()
		if status != BUSY {
			// We received a wild unexpected message
			log.Printf("Unexpected wild message received")
			break
		}

		select {
		case f := <-conn.nextReader:
			f(ioReader)
			// Ensure we have consumed the all the ioReader
			_, err = ioutil.ReadAll(ioReader)
			if err != nil {
				log.Printf("Unable to clean io reader")
				break
			}
		case <-conn.done:
			break
		}
	}
}

// Proxy a HTTP request through the Proxy over the WebSocket connection
func (conn *Connection) proxyRequest(w http.ResponseWriter, r *http.Request) (err error) {
	// Serialize HTTP request
	jsonReq, err := json.Marshal(common.NewHTTPRequest(r))
	if err != nil {
		return fmt.Errorf("Unable to serialize request : %s", err)
	}

	// Send the serialized HTTP request to the remote Proxy
	err = conn.ws.WriteMessage(websocket.TextMessage, jsonReq)
	if err != nil {
		return fmt.Errorf("Unable to write request : %s", err)
	}

	// Pipe the HTTP request body to the remote Proxy
	bodyWriter, err := conn.ws.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return fmt.Errorf("Unable to get request body writer : %s", err)
	}
	_, err = io.Copy(bodyWriter, r.Body)
	if err != nil {
		return fmt.Errorf("Unable to pipe request body : %s", err)
	}
	err = bodyWriter.Close()
	if err != nil {
		return fmt.Errorf("Unable to pipe request body (closeNoLock) : %s", err)
	}

	err = conn.handleNextMessage(func(reader io.Reader) (err error) {

		// Deserialize the HTTP Response
		httpResponse := new(common.HTTPResponse)
		err = json.NewDecoder(reader).Decode(httpResponse)
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

		return nil
	})
	if err != nil {
		return fmt.Errorf("Unable to handle request : %s", err)
	}

	err = conn.handleNextMessage(func(reader io.Reader) (err error) {
		// Pipe the HTTP response body right from the remote Proxy to the client
		_, err = io.Copy(w, reader)
		if err != nil {
			return fmt.Errorf("Unable to pipe response body : %s", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("Unable to handle request body : %s", err)
	}

	// Put the connection back in the pool
	conn.release()

	return nil
}

// Take notifies that this connection is going to be used
func (conn *Connection) take() bool {
	conn.lock.Lock()
	defer conn.lock.Unlock()

	if conn.isClosed() {
		return false
	}

	if conn.status != IDLE {
		return false
	}

	conn.status = BUSY

	return true
}

// Release notifies that this connection is ready to use again
func (conn *Connection) release() {
	conn.lock.Lock()
	defer conn.lock.Unlock()

	if conn.isClosed() {
		return
	}

	conn.idleSince = time.Now()
	conn.status = IDLE

	// Add the connection back to the pool
	conn.releaser(conn)
}

// IsClosed return true if the connection has been closed
func (conn *Connection) isClosed() bool {
	select {
	case <-conn.done:
		return true
	default:
		return false
	}
}

// Close the connection
func (conn *Connection) close() {
	conn.lock.Lock()
	defer conn.lock.Unlock()

	if conn.isClosed() {
		return
	}

	conn.status = CLOSED

	close(conn.done)

	// Close the underlying TCP conn
	conn.ws.Close()
}
