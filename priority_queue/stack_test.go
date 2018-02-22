package priority_queue

import (
	"testing"
	"fmt"
	"time"
	"sync"
	"strings"
)

var queueDumper = func(queue []interface{}) []interface{} {
	fmt.Printf("%v\n",queue)
	return queue
}

func TestExecEmpty(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	queue.Exec(queueDumper)
}

func TestOfferFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
		queue.ExecSync(queueDumper)
	}
}

func TestOfferLIFO(t *testing.T) {
	queue := NewPriorityQueue(LIFO)
	count := 5
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
		queue.ExecSync(queueDumper)
	}
}

func TestTakeFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}
	for i := 0 ; i < count ; i ++ {
		queue.ExecSync(queueDumper)
		fmt.Printf( "got %d\n", <- queue.Take())
	}
	queue.ExecSync(queueDumper)
}

func TestTakeLIFO(t *testing.T) {
	queue := NewPriorityQueue(LIFO)
	count := 5
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}
	for i := 0 ; i < count ; i ++ {
		queue.ExecSync(queueDumper)
		fmt.Printf( "got %d\n", <- queue.Take())
	}
	queue.ExecSync(queueDumper)
}

func TestSlowProducerFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	wg := &sync.WaitGroup{}
	count := 5
	wg.Add(count)
	go func() {
		for {
			fmt.Printf("got %d\n", <-queue.Take())
			wg.Done()
		}
	}()
	go func() {
		for i := 0 ; i < count ; i ++ {
			time.Sleep(100 * time.Millisecond)
			queue.Offer(i)
		}
	}()
	wg.Wait()
}

func TestSlowProducerLIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	wg := &sync.WaitGroup{}
	count := 5
	wg.Add(count)
	go func() {
		for {
			fmt.Printf("got %d\n", <-queue.Take())
			wg.Done()
		}
	}()
	go func() {
		for i := 0 ; i < count ; i ++ {
			time.Sleep(100 * time.Millisecond)
			queue.Offer(i)
		}
	}()
	wg.Wait()
}

func TestSlowConsumerFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	wg := &sync.WaitGroup{}
	count := 5
	wg.Add(count)
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("got %d\n", <-queue.Take())
			wg.Done()
		}
	}()
	go func() {
		for i := 0 ; i < count ; i ++ {
			queue.Offer(i)
		}
	}()
	wg.Wait()
}

func TestSlowConsumerLIFO(t *testing.T) {
	queue := NewPriorityQueue(LIFO)
	wg := &sync.WaitGroup{}
	count := 5
	wg.Add(count)
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("got %d\n", <-queue.Take())
			wg.Done()
		}
	}()
	go func() {
		for i := 0 ; i < count ; i ++ {
			queue.Offer(i)
		}
	}()
	wg.Wait()
}

func TestParallelConsumersFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5
	var chans []chan interface{}

	// Multiple take() in //
	for i := 0 ; i < count ; i ++ {
		chans = append(chans, queue.Take())
	}

	// Generate
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}

	// Consume
	for i, c := range chans {
		v, more := <- c
		if more {
			fmt.Printf("%d got %d\n", i, v)
		} else {
			fmt.Printf("%d got closed\n", i)
		}
	}
}

func TestParallelConsumersLIFO(t *testing.T) {
	queue := NewPriorityQueue(LIFO)
	count := 5
	var chans []chan interface{}

	// Multiple take() in //
	for i := 0 ; i < count ; i ++ {
		chans = append(chans, queue.Take())
	}

	// Generate
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}

	// Consume
	for i, c := range chans {
		v, more := <- c
		if more {
			fmt.Printf("%d got %d\n", i, v)
		} else {
			fmt.Printf("%d got closed\n", i)
		}
	}
}

func TestExecutorFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5

	// generator
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}

	queue.ExecSync(queueDumper)
	queue.ExecSync(func(a []interface{}) []interface{} {
		// remove the last 2 elements
		return a[:len(a)-2]
	})
	queue.ExecSync(queueDumper)

	for i := 0 ; i < 3 ; i ++ {
		fmt.Printf( "got %d\n", <- queue.Take())
	}
}

func TestExecutorLIFO(t *testing.T) {
	queue := NewPriorityQueue(LIFO)
	count := 5

	// generator
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}

	queue.ExecSync(queueDumper)
	queue.ExecSync(func(a []interface{}) []interface{} {
		// remove the last 2 elements
		return a[2:]
	})
	queue.ExecSync(queueDumper)

	for i := 0 ; i < 3 ; i ++ {
		fmt.Printf( "got %d\n", <- queue.Take())
	}
}

func TestSize(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5

	// generator
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}

	fmt.Printf("%d\n",queue.Size())
}

func TestExecutorAdd(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5
	var chans []chan interface{}

	// Multiple take() in //
	for i := 0 ; i < count ; i ++ {
		chans = append(chans, queue.Take())
	}

	queue.Exec(func(a []interface{}) []interface{} {
		a = []interface{}{}
		for i := 0 ; i < count ; i ++ {
			a = append(a,i)
		}
		return a
	})

	// Consume
	for i, c := range chans {
		v, more := <- c
		if more {
			fmt.Printf("%d got %d\n", i, v)
		} else {
			fmt.Printf("%d got closed\n", i)
		}
	}
}

func TestClose(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	count := 5
	var chans []chan interface{}

	// Multiple take() in //
	for i := 0 ; i < count ; i ++ {
		chans = append(chans, queue.Take())
	}

	// Generate
	for i := 0 ; i < count ; i ++ {
		queue.Offer(i)
	}

	queue.Close()
}

type ComparableItem struct {
	id int
	str string
}

func (value ComparableItem) compare(other interface{}) int {
	return strings.Compare(value.str, other.(ComparableItem).str)
}

func TestPriorityFIFO(t *testing.T) {
	queue := NewPriorityQueue(FIFO)
	queue.Offer(ComparableItem{1,"aaa"})
	queue.Offer(ComparableItem{2, "ccc"})
	queue.Offer(ComparableItem{3, "ccc"})
	queue.Offer(ComparableItem{4,"ddd"})
	queue.Offer(ComparableItem{5,"bbb"})
	queue.ExecSync(queueDumper)
	for queue.Size() > 0 {
		fmt.Printf( "got %v\n", <- queue.Take())
	}
}

func TestPriorityLIFO(t *testing.T) {
	queue := NewPriorityQueue(LIFO)
	queue.Offer(ComparableItem{1,"aaa"})
	queue.Offer(ComparableItem{2, "ccc"})
	queue.Offer(ComparableItem{3, "ccc"})
	queue.Offer(ComparableItem{4,"ddd"})
	queue.Offer(ComparableItem{5,"bbb"})
	queue.ExecSync(queueDumper)
	for queue.Size() > 0 {
		fmt.Printf( "got %v\n", <- queue.Take())
	}
}