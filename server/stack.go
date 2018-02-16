package server

// ConnectionStack is a LIFO structure to get the most recently used connection
type ConnectionStack struct {
	in   chan *Connection // Channel to add connections to the stack
	out  chan *Connection // Channel to get connections from the stack
	done chan struct{}
}

func newConnectionStack() (stack *ConnectionStack) {
	stack = new(ConnectionStack)
	stack.in = make(chan *Connection)
	stack.out = make(chan *Connection)
	stack.done = make(chan struct{})

	go stack.run()

	return stack
}

func (stack *ConnectionStack) run() {
	var connections []*Connection
	var top *Connection

	for {
		if top == nil {
			// There is no connection in the stack
			// wait for one to become available
			select {
			case top = <-stack.in:
			case <-stack.done:
				return
			}
		}

		select {
		case last := <-stack.in:
			// append the connection to the stack
			connections = append(connections, top)
			top = last
		case stack.out <- top:
			l := len(connections)
			if l > 0 {
				// remove the connection from the stack
				top = connections[l-1]
				connections = connections[:l-1]
			} else {
				// We just removed the last connection from the stack
				top = nil
			}
		case <-stack.done:
			return
		}
	}
}

func (stack *ConnectionStack) close() {
	close(stack.done)
}
