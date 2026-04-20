package main

import (
	"fmt"
	"sync"
)

func producer(id int, taskCh chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 1; i <= 3; i++ {
		task := fmt.Sprintf("task-%d-%d", id, i)
		taskCh <- task
		fmt.Printf("producer %d produced: %s\n", id, task)
	}
}

func consumer(id int, taskCh <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for task := range taskCh {
		fmt.Printf("consumer %d consumed: %s\n", id, task)
	}
}

func main() {
	const producerNum = 3
	const consumerNum = 2

	taskCh := make(chan string, 5) // 缓冲队列

	var prodWg sync.WaitGroup
	var consWg sync.WaitGroup

	// 启动生产者
	for i := 1; i <= producerNum; i++ {
		prodWg.Add(1)
		go producer(i, taskCh, &prodWg)
	}

	// 启动消费者
	for i := 1; i <= consumerNum; i++ {
		consWg.Add(1)
		go consumer(i, taskCh, &consWg)
	}

	// 等所有生产者完成后关闭 channel
	// 消费者用 range 监听，channel 关闭后自动退出
	prodWg.Wait()
	close(taskCh)

	consWg.Wait()
	fmt.Println("all done")
}
