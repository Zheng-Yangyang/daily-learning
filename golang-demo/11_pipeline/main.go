package main

import (
	"fmt"
)

// stage1：生成数据，返回只读 channel
func generate(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			fmt.Printf("stage1 generated: %d\n", n)
			out <- n
		}
	}()
	return out
}

// stage2：过滤偶数，返回只读 channel
func filter(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			if n%2 == 0 {
				fmt.Printf("stage2 filtered: %d\n", n)
				out <- n
			}
		}
	}()
	return out
}

// stage3：乘以 10，返回只读 channel
func multiply(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			result := n * 10
			fmt.Printf("stage3 processed: %d\n", result)
			out <- result
		}
	}()
	return out
}

func main() {
	// 串联三个 stage，构建流水线
	c1 := generate(1, 2, 3, 4, 5)
	c2 := filter(c1)
	c3 := multiply(c2)

	// 消费最终结果
	for result := range c3 {
		_ = result
	}

	fmt.Println("all done")
}
