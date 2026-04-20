package main

import (
	"fmt"
	"sync"
)

func main() {
	numCh := make(chan struct{})  // 通知数字 goroutine 打印
	charCh := make(chan struct{}) // 通知字母 goroutine 打印
	var wg sync.WaitGroup
	wg.Add(2)

	// goroutine 1：打印数字
	go func() {
		defer wg.Done()
		for i := 1; i <= 26; i++ {
			<-numCh // 等待轮到自己
			fmt.Printf("%d", i)
			charCh <- struct{}{} // 通知字母 goroutine
		}
	}()

	// goroutine 2：打印字母
	go func() {
		defer wg.Done()
		for i := 0; i < 26; i++ {
			<-charCh // 等待轮到自己
			fmt.Printf("%c ", 'A'+i)
			if i < 25 {
				numCh <- struct{}{} // 通知数字 goroutine（最后一轮不再通知）
			}
		}
	}()

	numCh <- struct{}{} // 启动：先让数字 goroutine 开始
	wg.Wait()
	fmt.Println()
}
