package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// ========== 检测工具 ==========

// countGoroutines 获取当前 goroutine 数量
func countGoroutines() int {
	return runtime.NumGoroutine()
}

// getGoroutineStacks 获取所有 goroutine 的堆栈信息
func getGoroutineStacks() string {
	buf := make([]byte, 1<<16) // 64KB
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// checkLeak 检测 goroutine 泄漏
func checkLeak(name string, fn func()) {
	before := countGoroutines()
	fn()
	time.Sleep(100 * time.Millisecond) // 等待 goroutine 稳定
	after := countGoroutines()
	leaked := after - before

	fmt.Printf("\n=== %s ===\n", name)
	fmt.Printf("before: %d goroutines\n", before)
	fmt.Printf("after:  %d goroutines\n", after)
	if leaked > 0 {
		fmt.Printf("leaked: %d goroutines ❌\n", leaked)
		// 打印泄漏的堆栈（只取前 3 个 goroutine 信息）
		stacks := getGoroutineStacks()
		lines := strings.Split(stacks, "\n")
		fmt.Println("--- leaked stacks (partial) ---")
		count := 0
		for i, line := range lines {
			if strings.HasPrefix(line, "goroutine") {
				count++
				if count > 3 {
					break
				}
				fmt.Println(lines[i])
			}
		}
	} else {
		fmt.Printf("leaked: 0 goroutines ✅\n")
	}
}

// ========== 泄漏场景演示 ==========

// 场景1：channel 未关闭，goroutine 永久阻塞
func leakByChannel() {
	ch := make(chan int)
	go func() {
		val := <-ch // 没人往 ch 发数据，也没人关闭 ch → 永久阻塞
		fmt.Println(val)
	}()
	// 函数返回，但 goroutine 还活着
}

// 场景1修复：用 done channel 通知退出
func fixedChannel() {
	ch := make(chan int)
	done := make(chan struct{})
	go func() {
		select {
		case val := <-ch:
			fmt.Println(val)
		case <-done: // 收到退出信号
			return
		}
	}()
	close(done) // 通知 goroutine 退出
	time.Sleep(10 * time.Millisecond)
}

// 场景2：goroutine 里死循环没有退出条件
func leakByLoop() {
	go func() {
		for { // 无限循环，没有退出条件
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// 场景2修复：用 context 控制退出
func fixedLoop() {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	close(done)
	time.Sleep(50 * time.Millisecond)
}

// 场景3：向已满且无人消费的 channel 发送数据
func leakBySend() {
	ch := make(chan int) // 无缓冲
	go func() {
		ch <- 1 // 没人接收 → 永久阻塞
	}()
}

// 场景3修复：确保有人消费
func fixedSend() {
	ch := make(chan int, 1) // 带缓冲，发送不阻塞
	go func() {
		ch <- 1
	}()
	time.Sleep(10 * time.Millisecond)
	<-ch // 消费掉
}

func main() {
	// 泄漏场景
	checkLeak("case1: channel 未关闭导致泄漏", leakByChannel)
	checkLeak("case2: 修复 channel 泄漏", fixedChannel)

	checkLeak("case3: 死循环无退出条件", leakByLoop)
	checkLeak("case4: 修复死循环泄漏", fixedLoop)

	checkLeak("case5: 发送阻塞导致泄漏", leakBySend)
	checkLeak("case6: 修复发送阻塞泄漏", fixedSend)

	fmt.Println("\n=== final goroutine count ===")
	fmt.Printf("total goroutines alive: %d\n", countGoroutines())
	fmt.Println("\nall done")
}
