package wsp

import (
	"fmt"
	"log"
	"net/http"
)

// HTTPResponse is a serializable version of http.Response ( with only useful fields )
type HTTPResponse struct {
	StatusCode    int
	Header        http.Header
	ContentLength int64
}

// SerializeHTTPResponse create a new HTTPResponse from a http.Response
func SerializeHTTPResponse(resp *http.Response) *HTTPResponse {
	r := new(HTTPResponse)
	r.StatusCode = resp.StatusCode
	r.Header = resp.Header
	r.ContentLength = resp.ContentLength
	return r
}

// NewHTTPResponse creates a new HTTPResponse
func NewHTTPResponse() (r *HTTPResponse) {
	r = new(HTTPResponse)
	r.Header = make(http.Header)
	return
}

// ProxyError log error and return a HTTP 526 error with the message
func ProxyError(w http.ResponseWriter, err error) {
	log.Println(err)
	http.Error(w, err.Error(), 526)
}

// ProxyErrorf log error and return a HTTP 526 error with the message
func ProxyErrorf(w http.ResponseWriter, format string, args ...interface{}) {
	ProxyError(w, fmt.Errorf(format, args...))
}
