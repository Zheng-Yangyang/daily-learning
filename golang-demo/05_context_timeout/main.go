package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func worker(ctx context.Context, id int, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("worker %d started\n", id)

	for {
		select {
		case <-ctx.Done(): // 监听取消信号
			fmt.Printf("worker %d received cancel, cleaning up...\n", id)
			return
		default:
			// 模拟处理任务
			fmt.Printf("worker %d processing...\n", id)
			time.Sleep(800 * time.Millisecond)
		}
	}
}

func main() {
	// 创建一个 3 秒超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel() // 养成好习惯，即使超时自动取消，也要 defer cancel 释放资源

	var wg sync.WaitGroup
	const workerNum = 2

	for i := 1; i <= workerNum; i++ {
		wg.Add(1)
		go worker(ctx, i, &wg)
	}

	wg.Wait()
	fmt.Println("all workers exited")
}
