package client

// This listener is used to trigger the connector when a connection status has changed to
//  1 -> Remove the
//  2 -> Open new connections if needed
//  -> the connectionStatusChanged returns a channel that can be used to wait for a connection status change
type ConnectionStatusListner struct {
	c chan struct{}
}

func NewConnectionStatusListner() (listener *ConnectionStatusListner) {
	listener = new(ConnectionStatusListner)
	listener.c = make(chan struct{}, 1)
	return listener
}

// onConnectionStatusChanged has to be called when a connection status change
func (listener *ConnectionStatusListner) onConnectionStatusChanged() {
	select {
	case listener.c <- struct{}{}:
	default:
	}
}

// onConnectionStatusChanged return the channel that can be used *BY A SINGLE GOROUTINE*
// to wait for a connection status change
func (listener *ConnectionStatusListner) connectionStatusChanged() chan struct{} {
	return listener.c
}
