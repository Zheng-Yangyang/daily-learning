package main

import (
	"fmt"
	"sync"
	"time"
)

type RateLimiter struct {
	tokenCh chan struct{}
	ticker  *time.Ticker
	done    chan struct{}
}

func NewRateLimiter(capacity int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokenCh: make(chan struct{}, capacity),
		ticker:  time.NewTicker(interval),
		done:    make(chan struct{}),
	}

	// 初始填满令牌桶
	for i := 0; i < capacity; i++ {
		rl.tokenCh <- struct{}{}
	}

	// 后台定时补充令牌
	go rl.refill()
	return rl
}

func (rl *RateLimiter) refill() {
	for {
		select {
		case <-rl.ticker.C:
			select {
			case rl.tokenCh <- struct{}{}: // 补充一个令牌
			default: // 桶已满，丢弃
			}
		case <-rl.done:
			rl.ticker.Stop()
			return
		}
	}
}

func (rl *RateLimiter) Acquire() {
	<-rl.tokenCh // 阻塞等待令牌
}

func (rl *RateLimiter) Stop() {
	close(rl.done)
}

func main() {
	const capacity = 3
	const requestNum = 10

	rl := NewRateLimiter(capacity, 500*time.Millisecond)
	defer rl.Stop()

	start := time.Now()
	var wg sync.WaitGroup

	for i := 1; i <= requestNum; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			rl.Acquire() // 阻塞直到拿到令牌
			elapsed := time.Since(start)
			fmt.Printf("[%02d:%02d.%03d] request %-2d acquired token, processing...\n",
				int(elapsed.Minutes()),
				int(elapsed.Seconds())%60,
				elapsed.Milliseconds()%1000,
				i,
			)
		}()
	}

	wg.Wait()
	fmt.Println("all requests done")
}
