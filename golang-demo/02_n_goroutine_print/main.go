package main

import (
	"fmt"
	"sync"
)

func main() {
	const N = 3
	const MAX = 100

	// 创建 N 个 channel，每个 goroutine 监听自己的 channel
	chs := make([]chan struct{}, N)
	for i := 0; i < N; i++ {
		chs[i] = make(chan struct{})
	}

	var wg sync.WaitGroup
	wg.Add(N)

	for i := 0; i < N; i++ {
		i := i // 闭包捕获
		go func() {
			defer wg.Done()
			for num := i + 1; num <= MAX; num += N {
				<-chs[i] // 等待轮到自己
				fmt.Printf("goroutine%d: %d\n", i+1, num)
				next := (i + 1) % N
				if num+next < MAX+next { // 还有下一轮
					chs[next] <- struct{}{}
				}
			}
		}()
	}

	chs[0] <- struct{}{} // 启动第一个
	wg.Wait()
}
