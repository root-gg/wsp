package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/root-gg/wsp/common"
)

var id uint64 = 0

func getNextId() uint64 {
	return atomic.AddUint64(&id, uint64(1))
}

// ConnectionStatus of a Connection
type ConnectionStatus int

const (
	CONNECTING ConnectionStatus = iota
	IDLE
	RUNNING
	CLOSED
)

// Connection handle a single websocket (HTTP/TCP) connection to an Server
type Connection struct {
	clientSettings *common.ClientSettings

	ws *websocket.Conn

	status                  ConnectionStatus
	connectionStatusListner *ConnectionStatusListner

	lock sync.RWMutex
	done chan struct{}
}

// newConnection create a Connection object
func newConnection(clientSettings *common.ClientSettings, connectionStatusListner *ConnectionStatusListner) (conn *Connection) {
	conn = new(Connection)
	conn.clientSettings = clientSettings
	conn.clientSettings.ConnectionId = getNextId()
	conn.connectionStatusListner = connectionStatusListner
	conn.status = CONNECTING
	conn.done = make(chan struct{})
	return
}

// Set the status of the connection in a concurrently safe way
func (conn *Connection) setStatus(status ConnectionStatus) {
	conn.lock.Lock()
	defer conn.lock.Unlock()

	// Trigger a pool refresh to open new connections if needed
	defer conn.connectionStatusListner.onConnectionStatusChanged()
	conn.status = status
}

// Get the status of the connection in a concurrently safe way
func (conn *Connection) getStatus() ConnectionStatus {
	conn.lock.RLock()
	defer conn.lock.RUnlock()
	return conn.status
}

// Open a connection to the remote WSP Server
func (conn *Connection) connect(dialer *websocket.Dialer, target string, secretKey string) (err error) {
	conn.lock.Lock()
	defer conn.lock.Unlock()

	log.Printf("Connecting to %s", target)

	// Create a new TCP(/TLS) conn ( no use of net.http )
	conn.ws, _, err = dialer.Dial(target, http.Header{"X-SECRET-KEY": {secretKey}})
	if err != nil {
		return fmt.Errorf("dialer error : %s", err)
	}

	log.Printf("Connected to %s", target)

	return nil
}

// Send the client configuration to the remote WSP Server
func (conn *Connection) initialize() (err error) {
	message, err := conn.clientSettings.ToJson()
	if err != nil {
		return fmt.Errorf("connection initlization error, unable to serialize client settings : %s", err)
	}

	// Send the greeting message with proxy id and wanted pool size.
	err = conn.ws.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		return fmt.Errorf("connection initlization error : %s", err)
	}

	return nil
}

// the main loop :
//  - wait to receive HTTP requests from the Server
//  - execute HTTP requests
//  - send HTTP response back to the Server
//
// As in the server code there is no buffering of HTTP request/response body
// As is the server if any error occurs the connection is closed/throwed
func (conn *Connection) serve(httpClient *http.Client, validator *common.RequestValidator) {
	// Keep conn alive
	go func() {
		defer conn.close()

		for {
			if conn.isClosed() {
				break
			}
			time.Sleep(5 * time.Second)
			err := conn.ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
			if err != nil {
				//log.Printf("ping fail : %s", err)
				break
			}
		}
	}()

	for {
		conn.setStatus(IDLE)

		// Read request
		_, jsonRequest, err := conn.ws.ReadMessage()
		if err != nil {
			if !conn.isClosed() {
				log.Printf("Unable to read request :%s", err)
			}
			break
		}

		conn.setStatus(RUNNING)

		// Deserialize request
		httpRequest := new(common.HTTPRequest)
		err = json.Unmarshal(jsonRequest, httpRequest)
		if err != nil {
			conn.error(fmt.Sprintf("Unable to deserialize json http request : %s\n", err))
			break
		}

		// Get an executable net/http.Request
		req, err := httpRequest.ToStdLibHTTPRequest()
		if err != nil {
			conn.error(fmt.Sprintf("Unable to deserialize http request : %v\n", err))
			break
		}

		err = validator.Validate(req)
		if err != nil {
			// Discard the request body
			err2 := conn.discard()
			if err2 != nil {
				conn.error(err2.Error())
				break
			}
			err3 := conn.error(err.Error())
			if err3 != nil {
				break
			}
			continue
		}

		log.Printf("[%s] %s", req.Method, req.URL.String())

		// Pipe request body
		_, bodyReader, err := conn.ws.NextReader()
		if err != nil {
			log.Printf("Unable to get response body reader : %v", err)
			break
		}
		req.Body = ioutil.NopCloser(bodyReader)

		// Execute request
		resp, err := httpClient.Do(req)
		if err != nil {
			err = conn.error(fmt.Sprintf("Unable to execute request : %v\n", err))
			if err != nil {
				break
			}
			continue
		}

		// Serialize response
		jsonResponse, err := json.Marshal(common.SerializeHTTPResponse(resp))
		if err != nil {
			err = conn.error(fmt.Sprintf("Unable to serialize response : %v\n", err))
			if err != nil {
				break
			}
			continue
		}

		// Write response
		err = conn.ws.WriteMessage(websocket.TextMessage, jsonResponse)
		if err != nil {
			log.Printf("Unable to write response : %v", err)
			break
		}

		// Pipe response body
		bodyWriter, err := conn.ws.NextWriter(websocket.BinaryMessage)
		if err != nil {
			log.Printf("Unable to get response body writer : %v", err)
			break
		}
		_, err = io.Copy(bodyWriter, resp.Body)
		if err != nil {
			log.Printf("Unable to get pipe response body : %v", err)
			break
		}
		bodyWriter.Close()
	}
}

// Craft an error response to forward back to the WSP Server
func (conn *Connection) error(msg string) (err error) {
	resp := common.NewHTTPResponse()
	resp.StatusCode = 527

	log.Println(msg)

	resp.ContentLength = int64(len(msg))

	// Serialize response
	jsonResponse, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Unable to serialize response : %v", err)
		return
	}

	// Write response
	err = conn.ws.WriteMessage(websocket.TextMessage, jsonResponse)
	if err != nil {
		log.Printf("Unable to write response : %v", err)
		return
	}

	// Write response body
	err = conn.ws.WriteMessage(websocket.BinaryMessage, []byte(msg))
	if err != nil {
		log.Printf("Unable to write response body : %v", err)
		return
	}

	return
}

// Discard the next message
func (conn *Connection) discard() (err error) {
	mt, _, err := conn.ws.NextReader()
	if err != nil {
		return err
	}
	if mt != websocket.BinaryMessage {
		return errors.New("Invalid body message type")
	}
	return nil
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

// Close the ws/tcp connection and remove it from the pool
func (conn *Connection) close() {
	conn.lock.Lock()
	defer conn.lock.Unlock()
	if conn.isClosed() {
		return
	}

	conn.status = CLOSED
	close(conn.done)

	if conn.ws != nil {
		conn.ws.Close()
	}
}
