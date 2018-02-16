package common

import (
	"net/http"
	"net/url"
)

// HTTPRequest is a serializable version of net/http.Request ( with only usefull fields )
type HTTPRequest struct {
	Method        string
	URL           string
	Header        map[string][]string
	ContentLength int64
}

// NewHTTPRequest creates a new HTTPRequest from a net/http.Request instance
func NewHTTPRequest(req *http.Request) (r *HTTPRequest) {
	r = new(HTTPRequest)
	r.URL = req.URL.String()
	r.Method = req.Method
	r.Header = req.Header
	r.ContentLength = req.ContentLength
	return
}

// ToStdLibHTTPRequest creates a new net/http.Request from this HTTPRequest instance
func (req *HTTPRequest) ToStdLibHTTPRequest() (r *http.Request, err error) {
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
