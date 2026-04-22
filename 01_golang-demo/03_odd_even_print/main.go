package main

import (
	"fmt"
	"sync"
)

func main() {
	const MAX = 10

	oddCh := make(chan struct{}, 1)  // 通知奇数 goroutine
	evenCh := make(chan struct{}, 1) // 通知偶数 goroutine

	var wg sync.WaitGroup
	wg.Add(2)

	// goroutine 1：打印奇数
	go func() {
		defer wg.Done()
		for i := 1; i <= MAX; i += 2 {
			<-oddCh
			fmt.Printf("odd:  %d\n", i)
			evenCh <- struct{}{}
		}
	}()

	// goroutine 2：打印偶数
	go func() {
		defer wg.Done()
		for i := 2; i <= MAX; i += 2 {
			<-evenCh
			fmt.Printf("even: %d\n", i)
			if i < MAX {
				oddCh <- struct{}{}
			}
		}
	}()

	oddCh <- struct{}{} // 点火，奇数先打
	wg.Wait()
}
