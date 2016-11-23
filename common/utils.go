package common

import (
	"fmt"
	"log"
	"net/http"
)

// ProxyError log error and return a HTTP 526 error with the message
func ProxyError(w http.ResponseWriter, err error) {
	log.Println(err)
	http.Error(w, err.Error(), 526)
}

// ProxyErrorf log error and return a HTTP 526 error with the message
func ProxyErrorf(w http.ResponseWriter, format string, args ...interface{}) {
	ProxyError(w, fmt.Errorf(format, args...))
}
