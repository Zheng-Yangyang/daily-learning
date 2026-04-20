package main

import (
	"fmt"
	"sync"
	"time"
)

// generate 生成数据
func generate(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			out <- n
		}
	}()
	return out
}

// worker 处理单个任务，模拟耗时
func worker(id int, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			fmt.Printf("[fan-out] worker%d processing: %d\n", id, n)
			time.Sleep(100 * time.Millisecond) // 模拟耗时
			out <- n * 10
		}
	}()
	return out
}

// fanOut 将一个 input channel 复制分发给 n 个 worker
func fanOut(in <-chan int, workerNum int) []<-chan int {
	outs := make([]<-chan int, workerNum)
	for i := 0; i < workerNum; i++ {
		outs[i] = worker(i+1, in)
	}
	return outs
}

// fanIn 将多个 channel 合并成一个 channel
func fanIn(channels ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup

	// 每个 input channel 启动一个转发 goroutine
	forward := func(ch <-chan int) {
		defer wg.Done()
		for n := range ch {
			out <- n
		}
	}

	wg.Add(len(channels))
	for _, ch := range channels {
		go forward(ch)
	}

	// 等所有转发 goroutine 完成后关闭 out
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func main() {
	nums := []int{1, 2, 3, 4, 5}

	// ====== 串行处理，作为对比 ======
	fmt.Println("=== serial ===")
	start := time.Now()
	for _, n := range nums {
		time.Sleep(100 * time.Millisecond)
		_ = n * 10
	}
	fmt.Printf("serial cost: %v\n\n", time.Since(start).Round(time.Millisecond))

	// ====== 并行处理：Fan-out + Fan-in ======
	fmt.Println("=== parallel (fan-out + fan-in) ===")
	start = time.Now()

	// stage1：生成数据
	c := generate(nums...)

	// stage2：fan-out 到 3 个 worker 并行处理
	workerChans := fanOut(c, 3)

	// stage3：fan-in 合并结果
	results := fanIn(workerChans...)

	// 消费结果
	for result := range results {
		fmt.Printf("[fan-in]  result: %d\n", result)
	}

	fmt.Printf("parallel cost: %v\n\n", time.Since(start).Round(time.Millisecond))
	fmt.Println("all done")
}
