package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/root-gg/wsp"
)

// Status of a Connection
const (
	CONNECTING = iota
	IDLE
	RUNNING
)

// Connection handle a single websocket (HTTP/TCP) connection to an Server
type Connection struct {
	pool   *Pool
	ws     *websocket.Conn
	status int
}

// NewConnection create a Connection object
func NewConnection(pool *Pool) (conn *Connection) {
	conn = new(Connection)
	conn.pool = pool
	conn.status = CONNECTING
	return
}

// Connect to the IsolatorServer using a HTTP websocket
func (connection *Connection) Connect(ctx context.Context) (err error) {
	log.Printf("Connecting to %s", connection.pool.target)

	// Create a new TCP(/TLS) connection ( no use of net.http )
	connection.ws, _, err = connection.pool.client.dialer.DialContext(
		ctx,
		connection.pool.target,
		http.Header{"X-SECRET-KEY": {connection.pool.secretKey}},
	)

	if err != nil {
		return err
	}

	log.Printf("Connected to %s", connection.pool.target)

	// Send the greeting message with proxy id and wanted pool size.
	greeting := fmt.Sprintf(
		"%s_%d",
		connection.pool.client.Config.ID,
		connection.pool.client.Config.PoolIdleSize,
	)
	err = connection.ws.WriteMessage(websocket.TextMessage, []byte(greeting))
	if err != nil {
		log.Println("greeting error :", err)
		connection.Close()
		return
	}

	go connection.serve(ctx)

	return
}

// the main loop it :
//  - wait to receive HTTP requests from the Server
//  - execute HTTP requests
//  - send HTTP response back to the Server
//
// As in the server code there is no buffering of HTTP request/response body
// As is the server if any error occurs the connection is closed/throwed
func (connection *Connection) serve(ctx context.Context) {
	defer connection.Close()

	// Keep connection alive
	go func() {
		for {
			time.Sleep(30 * time.Second)
			err := connection.ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
			if err != nil {
				connection.Close()
			}
		}
	}()

	for {
		// Read request
		connection.status = IDLE
		_, jsonRequest, err := connection.ws.ReadMessage()
		if err != nil {
			log.Println("Unable to read request", err)
			break
		}

		connection.status = RUNNING

		// Trigger a pool refresh to open new connections if needed
		go connection.pool.connector(ctx)

		// Deserialize request
		httpRequest := new(wsp.HTTPRequest)
		err = json.Unmarshal(jsonRequest, httpRequest)
		if err != nil {
			connection.error(fmt.Sprintf("Unable to deserialize json http request : %s\n", err))
			break
		}

		req, err := wsp.UnserializeHTTPRequest(httpRequest)
		if err != nil {
			connection.error(fmt.Sprintf("Unable to deserialize http request : %v\n", err))
			break
		}

		log.Printf("[%s] %s", req.Method, req.URL.String())

		// Pipe request body
		_, bodyReader, err := connection.ws.NextReader()
		if err != nil {
			log.Printf("Unable to get response body reader : %v", err)
			break
		}
		req.Body = ioutil.NopCloser(bodyReader)

		// Execute request
		resp, err := connection.pool.client.client.Do(req)
		if err != nil {
			err = connection.error(fmt.Sprintf("Unable to execute request : %v\n", err))
			if err != nil {
				break
			}
			continue
		}

		// Serialize response
		jsonResponse, err := json.Marshal(wsp.SerializeHTTPResponse(resp))
		if err != nil {
			err = connection.error(fmt.Sprintf("Unable to serialize response : %v\n", err))
			if err != nil {
				break
			}
			continue
		}

		// Write response
		err = connection.ws.WriteMessage(websocket.TextMessage, jsonResponse)
		if err != nil {
			log.Printf("Unable to write response : %v", err)
			break
		}

		// Pipe response body
		bodyWriter, err := connection.ws.NextWriter(websocket.BinaryMessage)
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

func (connection *Connection) error(msg string) (err error) {
	resp := wsp.NewHTTPResponse()
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
	err = connection.ws.WriteMessage(websocket.TextMessage, jsonResponse)
	if err != nil {
		log.Printf("Unable to write response : %v", err)
		return
	}

	// Write response body
	err = connection.ws.WriteMessage(websocket.BinaryMessage, []byte(msg))
	if err != nil {
		log.Printf("Unable to write response body : %v", err)
		return
	}

	return
}

// Discard request body
func (connection *Connection) discard() (err error) {
	mt, _, err := connection.ws.NextReader()
	if err != nil {
		return nil
	}
	if mt != websocket.BinaryMessage {
		return errors.New("Invalid body message type")
	}
	return
}

// Close close the ws/tcp connection and remove it from the pool
func (connection *Connection) Close() {
	connection.pool.lock.Lock()
	defer connection.pool.lock.Unlock()

	connection.pool.remove(connection)
	connection.ws.Close()
}
