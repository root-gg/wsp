.PHONY: build-server build-client

build-server:
	go build ./cmd/wsp_server

build-client:
	go build ./cmd/wsp_client
