package common

import (
	"net/http"
	"net/url"
)

// HTTPRequest is a serializable version of http.Request ( with only usefull fields )
type HTTPRequest struct {
	Method        string
	URL           string
	Header        map[string][]string
	ContentLength int64
}

// SerializeHTTPRequest create a new HTTPRequest from a http.Request
func SerializeHTTPRequest(req *http.Request) (r *HTTPRequest) {
	r = new(HTTPRequest)
	r.URL = req.URL.String()
	r.Method = req.Method
	r.Header = req.Header
	r.ContentLength = req.ContentLength
	return
}

// UnserializeHTTPRequest create a new http.Request from a HTTPRequest
func UnserializeHTTPRequest(req *HTTPRequest) (r *http.Request, err error) {
	r = new(http.Request)
	r.Method = req.Method
	r.URL, err = url.Parse(req.URL)
	if err != nil {
		return
	}
	r.Header = req.Header
	r.ContentLength = req.ContentLength
	return
}
