package main

import (
	"container/heap"
	"fmt"
	"sync"
	"time"
)

// Item 延迟队列中的元素
type Item struct {
	value    string
	expireAt time.Time
	index    int // 在堆中的索引
}

// MinHeap 最小堆，按到期时间排序
type MinHeap []*Item

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i].expireAt.Before(h[j].expireAt) }
func (h MinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *MinHeap) Push(x any) {
	item := x.(*Item)
	item.index = len(*h)
	*h = append(*h, item)
}
func (h *MinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

// DelayQueue 延迟队列
type DelayQueue struct {
	mu     sync.Mutex
	h      MinHeap
	wakeup chan struct{} // 通知 worker 重新计算最近到期时间
}

func NewDelayQueue() *DelayQueue {
	dq := &DelayQueue{
		wakeup: make(chan struct{}, 1),
	}
	heap.Init(&dq.h)
	return dq
}

func (dq *DelayQueue) Push(value string, delay time.Duration) {
	item := &Item{
		value:    value,
		expireAt: time.Now().Add(delay),
	}
	dq.mu.Lock()
	heap.Push(&dq.h, item)
	dq.mu.Unlock()

	// 通知 worker 有新任务，可能需要更新定时器
	select {
	case dq.wakeup <- struct{}{}:
	default:
	}
}

func (dq *DelayQueue) Run(out chan<- string, done <-chan struct{}) {
	for {
		dq.mu.Lock()
		if dq.h.Len() == 0 {
			// 队列为空，等待新任务
			dq.mu.Unlock()
			select {
			case <-dq.wakeup:
				continue
			case <-done:
				return
			}
		}

		// 取堆顶（最近到期的任务）
		top := dq.h[0]
		delay := time.Until(top.expireAt)
		dq.mu.Unlock()

		if delay <= 0 {
			// 已到期，弹出并发送
			dq.mu.Lock()
			heap.Pop(&dq.h)
			dq.mu.Unlock()
			out <- top.value
			continue
		}

		// 等待到期，或被新任务打断
		select {
		case <-time.After(delay):
			// 到期，下一轮循环处理
		case <-dq.wakeup:
			// 有新任务进来，重新计算堆顶
		case <-done:
			return
		}
	}
}

func main() {
	dq := NewDelayQueue()
	out := make(chan string, 10)
	done := make(chan struct{})

	// 启动消费 worker
	go dq.Run(out, done)

	start := time.Now()

	// 并发 Push 任务
	tasks := []struct {
		name  string
		delay time.Duration
	}{
		{"task1", 3 * time.Second},
		{"task2", 1 * time.Second},
		{"task3", 2 * time.Second},
	}

	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		t := t
		go func() {
			defer wg.Done()
			dq.Push(t.name, t.delay)
			elapsed := time.Since(start)
			fmt.Printf("[%02d:%02d.%03d] pushed: %s, delay %v\n",
				int(elapsed.Minutes()),
				int(elapsed.Seconds())%60,
				elapsed.Milliseconds()%1000,
				t.name, t.delay,
			)
		}()
	}
	wg.Wait()

	// 收集所有到期任务
	for i := 0; i < len(tasks); i++ {
		val := <-out
		elapsed := time.Since(start)
		fmt.Printf("[%02d:%02d.%03d] expired: %s\n",
			int(elapsed.Minutes()),
			int(elapsed.Seconds())%60,
			elapsed.Milliseconds()%1000,
			val,
		)
	}

	close(done)
	fmt.Println("all done")
}
