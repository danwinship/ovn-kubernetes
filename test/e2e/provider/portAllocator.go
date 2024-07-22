package provider

import "sync"

type portAllocator struct {
	start int32
	end   int32
	next  int32
	mu    sync.Mutex
}

func newPortAllocator(start, end int32) *portAllocator {
	return &portAllocator{start: start, end: end, next: start, mu: sync.Mutex{}}
}

func (pr *portAllocator) allocate() int32 {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	val := pr.next
	pr.next += 1
	if pr.next > pr.end {
		panic("port allocation limit reached")
	}
	return val
}
