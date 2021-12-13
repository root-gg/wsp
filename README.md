WS PROXY
========

This is a reverse HTTP proxy over websockets.
The aim is to securely make call to internal APIs from outside.

How does it works
-----------------

a WSP client runs in the internal network ( alongside the APIs )
and connects to a remote WSP server with HTTP websockets.

One issue HTTP requests to the WSP server with an extra
HTTP header 'X-PROXY-DESTINATION: "http://api.internal/resource"'
to the /request endpoint.

The WSP Server then forward the request to the WSP Client over the
one of the offered websockets. The WSP Client receive and execute
locally an HTTP request to the URL provided in X-PROXY-DESTINATION
and forwards the HTTP response back to the WSP server which in turn
forwards the response back to the client. Please note that no
buffering of any sort occurs.

If several WSP clients connect to a WSP server, requests will be spread
in a random way to all the WSP clients.

![wsp schema](https://cloud.githubusercontent.com/assets/6413246/24397653/3f2e4b30-13a7-11e7-820b-cde6e784382f.png)

Example
-------

- Start a test server

```bash
make run-test-server 
```

Build
-----

- Build client (wsp client)

```bash
make build-client
```

- Build server (wsp server)

```bash
make build-server
```

WSP server configuration
------------------------

```
# wsp_server.cfg
---
host : 127.0.0.1                     # Address to bind the HTTP server
port : 8080                          # Port to bind the HTTP server
timeout : 1000                       # Time to wait before acquiring a WS connection to forward the request (milliseconds)
idletimeout : 60000                  # Time to wait before closing idle connection when there is enough idle connections (milliseconds)
#blacklist :                         # Forbidden destination ( deny nothing if empty )
# - method : ".*"                    #   Applied in order before whitelist
#   url : "^http(s)?://google.*"     #   None must match
#   headers :                        #   Optinal header check
#     X-CUSTOM-HEADER : "^value$"    # 
#whitelist :                         # Allowed destinations  ( allow all if empty )
# - method : "^GET$"                 #   Applied in order after blacklist
#   url : "^http(s)?://.*$"          #   One must match
#   headers :                        #   Optinal header check
#     X-CUSTOM-HEADER : "^value$"    # 
# secretkey : ThisIsASecret          # secret key that must be set in clients configuration
```

```
$ ./wsp_server -config wsp_server.cfg
{
  "Host": "127.0.0.1",
  "Port": 8080
}
2016/11/22 15:31:39 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:31:40 Registering new connection from 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
2016/11/22 15:33:34 GET map[User-Agent:[curl/7.26.0] Accept:[*/*] X-Proxy-Destination:[https://google.fr]]
2016/11/22 15:33:34 proxy request to 7e2d8782-f893-4ff3-7e9d-299b4c0a518a
```

For now TLS setup should be implemented using an HTTP reverse proxy
like NGinx or Apache...

WSP proxy configuration
-----------------------

```
# wsp_client.cfg
---
targets :                            # Endpoints to connect to
 - ws://127.0.0.1:8080/register      #
poolidlesize : 10                    # Default number of concurrent open (TCP) connections to keep idle per WSP server
poolmaxsize : 100                    # Maximum number of concurrent open (TCP) connections per WSP server
#blacklist :                         # Forbidden destination ( deny nothing if empty )
# - method : ".*"                    #   Applied in order before whitelist
#   url : ".*forbidden.*"            #   None must match
#   headers :                        #   Optinal header check
#     X-CUSTOM-HEADER : "^value$"    # 
#whitelist :                         # Allowed destinations  ( allow all if empty )
# - method : "^GET$"                 #   Applied in order after blacklist
#   url : "http(s)?://.*$"           #   One must match
#   headers :                        #   Optinal header check
#     X-CUSTOM-HEADER : "^value$"    # 
# secretkey : ThisIsASecret          # secret key that must match the value set in servers configuration
```

- poolMinSize is the default number of opened TCP/HTTP/WS connections
 to open per WSP server. If there is a burst of simpultaneous requests
 the number of open connection will rise and then decrease back to this
 number.
- poolMinIdleSize is the number of connection to keep idle, meaning
 that if there is more than this number of simultaneous requests the
 WSP client will try to open more connections to keep idle connection.
- poolMaxSize is the maximum number of simultaneous connection that
 the proxy will ever initiate per WSP server.

```
$ ./wsp_client -config wsp_client.cfg
{
  "ID": "7e2d8782-f893-4ff3-7e9d-299b4c0a518a",
  "Targets": [
    "ws://127.0.0.1:8080/register"
  ],
  "PoolMinSize": 10,
  "PoolMinIdleSize": 5,
  "PoolMaxSize": 100
}
2016/11/22 15:31:39 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:31:40 Connecting to ws://127.0.0.1:8080/register
2016/11/22 15:33:34 got request : {"Method":"GET","URL":"https://google.fr","Header":{"Accept":["*/*"],"User-Agent":["curl/7.26.0"],"X-Proxy-Destination":["https://google.fr"]},"ContentLength":0}
```

Client
------

```
$ curl -H 'X-PROXY-DESTINATION: https://google.fr' http://127.0.0.1:8080/request
<!doctype html><html itemscope="" itemtype="http://schema.org/WebPage" lang="fr"><head><meta content="text/html; charset=UTF-8" http-equiv="Content-Type"><meta content="/images/branding/googleg/1x/googleg_standard_color_128dp.png" it...
```

