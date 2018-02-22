package priority_queue

import (
	"reflect"
	"fmt"
)

type Priority int

const (
	FIFO Priority = iota // FIRST IN FIRST OUT
	LIFO                 // LAST IN FIST OUT
)

// Function can be passed to the exec method to browse or modify the content of the stack
type Function func([]interface{}) []interface{}

// Used to compare the priority inside the stack
type Comparable interface {
	compare(other interface{}) int
}

// Message are sent to the run goroutine to communicate with the queue
type message interface{}
type offerMessage struct{ item interface{} }
type takeMessage struct{ request chan interface{} }
type priorityMessage struct{ priority Priority }
type closeMessage struct{}
type execMessage struct {
	f    Function
	done chan struct{}
}

// PriorityQueue is a thread-safe browsable priority queue that can be FIFO or LIFO.
// If it is offered Comparable objects it turns to a priority queue ( It it still FIFO / LIFO in case of equality )
// Unfortunately due to the lack of generics it's not type safe :'(
type PriorityQueue struct {
	channel chan message
}

func NewPriorityQueue(priority Priority) (queue *PriorityQueue) {
	queue = new(PriorityQueue)
	queue.channel = make(chan message)

	queue.run(priority)

	return queue
}

// Offer adds an item to the stack ( this call will never block but will panic if the stack has been closed )
func (queue *PriorityQueue) Offer(item interface{}) {
	queue.channel <- offerMessage{item}
}

// Take returns a channel that blocks until there is a connection
// The channel will be closed if there are too many goroutine waiting for a value or if the PriorityQueue has been closed
func (queue *PriorityQueue) Take() (request chan interface{}) {
	request = make(chan interface{}, 1) // Ensure we won't block the run gorouting by buffering the item
	queue.channel <- takeMessage{request}
	return request
}

// TakeSync get a value from the stack synchronously
// Ok will be set to false if there are too many goroutine waiting for a value or if the PriorityQueue has been closed
func (queue *PriorityQueue) TakeSync() (value interface{}, ok bool) {
	v, ok := <-queue.Take()
	return v, ok
}

// Exec a function that can browse or modify the content of the stack
// Beware : the head of the stack is at the end of the array
// The content of the PriorityQueue can be modified, reordering, adding and removing are safe
func (queue *PriorityQueue) Exec(function Function) (done chan struct{}) {
	done = make(chan struct{})
	queue.channel <- execMessage{function, done}
	return done
}

// ExecSync is the same of exec but synchronous
func (queue *PriorityQueue) ExecSync(f Function) {
	<-queue.Exec(f)
}

// Size returns the number of element in the queue
func (queue *PriorityQueue) Size() (length int) {
	queue.ExecSync(func(items []interface{}) []interface{} {
		length = len(items)
		return items
	})
	return length
}

// SetPriority at runtime because I know how crazy you are
func (queue *PriorityQueue) SetPriority(priority Priority) {
	queue.channel <- priorityMessage{priority}
}

func (queue *PriorityQueue) Close() {
	queue.channel <- closeMessage{}
}

func (queue *PriorityQueue) run(priority Priority) {
	go func() {
		// items in the queue
		var items   []interface{}

		//!\\ inflight requests
		//!\\ requests are always FIFO
		var requests []chan interface{}

		//!\\ len(requests) > 0 <=> len(queue.items) == 0

		// Get a item from the queue in FIFO or LIFO mode
		get := func() interface{} {
			if len(items) == 0 {
				panic("get called on an empty queue")
			}
			var item interface{}
			if priority == FIFO {
				// get and remove the first of the array
				item, items = items[0], items[1:]
			} else if priority == LIFO {
				// get and remove the last of the array
				item = items[len(items)-1]
				items = items[:len(items)-1]
			} else {
				panic("Invalid queue type")
			}
			return item
		}

	LOOP:
		for {
			msg := <-queue.channel
			switch message := msg.(type) {
			case offerMessage:
				// Offer(item) request

				if len(requests) > 0 {
					// There is already a Take() request waiting so no need to buffer the value
					// Requests are FIFO so get the oldest request at the beginning of the array
					var request chan interface{}
					request, requests = requests[0], requests[1:]

					// complete the request
					request <- message.item
				} else {
					// reorder the queue by priority
					if comparable, ok := message.item.(Comparable); ok {
						// Naive O(n) implementation
						for i, v := range items {
							var condition bool
							if priority == FIFO {
								// On equals the first in is still the first out
								condition = comparable.compare(v) > 0
							} else if priority == LIFO {
								// On equals the last in is still the first out
								condition = comparable.compare(v) < 0
							} else {
								panic("Invalid queue type")
							}

							if condition {
								// Insert a index i
								items = append(items, interface{}(nil))
								copy(items[i+1:], items[i:])
								items[i] = message.item
								continue LOOP
							}
						}
					}

					// append to the end of the array
					items = append(items, message.item)
				}
			case takeMessage:
				if len(items) > 0 {
					// There is at least one value available in the queue we can complete the request right now
					message.request <- get()
				} else {
					// buffer the request
					requests = append(requests, message.request)
				}
			case execMessage:
				// Exec() request
				items = message.f(items)
				close(message.done)

				// In case we added elements, complete as much waiting requests as possible
				for _, request := range requests {
					if len(items) > 0 {
						// complete the request
						request <- get()
					}
				}
			case priorityMessage:
				priority = message.priority
			case closeMessage:
				// unlock all in-flight requests
				for _, request := range requests {
					close(request)
				}

				// free resources ( needed ? )
				items = nil
				requests = nil
				break LOOP
			default:
				panic(fmt.Sprintf("invalid priority queue message %T", message))
			}
		}
	}()
}

// SPECIALIZATION TEST FOR FUN
// WHY THE FUCK YOU NO HAVE GENERIKZ GOLANG :'(((((

type IntQueue struct{ PriorityQueue }

func (queue *IntQueue) Offer(item int) {
	queue.PriorityQueue.Offer(item)
}

func (queue *IntQueue) Take() (c chan int) {
	c = make(chan int)
	go func() {
		v := <-queue.PriorityQueue.Take()
		c <- v.(int)
	}()
	return c
}

func (queue *IntQueue) TakeSync() (int) {
	v := <-queue.PriorityQueue.Take()
	return v.(int)
}

func (queue *IntQueue) Exec(f IntStackFunction) (done chan struct{}) {
	return queue.PriorityQueue.Exec(f.adapt())
}

func (queue *IntQueue) ExecSync(f IntStackFunction) {
	<-queue.Exec(f)
}

// YEAH GOLANG MADNESS
type IntStackFunction func([]int) []int

func (f IntStackFunction) adapt() Function {
	return func(a []interface{}) []interface{} {
		return toInterfaceArray(f(toIntArray(a)))
	}
}

// YEAH MORE GOLANG MADNESS
func toInterfaceArray(v interface{}) (res []interface{}) {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(v)
		res = make([]interface{}, s.Len())
		for i := 0; i < s.Len(); i++ {
			res[i] = s.Index(i).Interface()
		}
	default:
		panic(fmt.Sprintf("unexpected type %T", v))
	}
	return res
}

// YEAH EVEN MORE GOLANG MADNESS
func toIntArray(a []interface{}) (res []int) {
	for _, v := range a {
		res = append(res, v.(int))
	}
	return res
}
