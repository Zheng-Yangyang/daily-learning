package main

import (
	"fmt"
	"math/rand"
	"time"
)

// ============================================================
// 模拟银行接口：可以控制它什么时候正常、什么时候故障
// ============================================================

type BankService struct {
	failMode  bool // true=故障模式，false=正常模式
	callCount int
}

func (b *BankService) Deduct(amount float64) error {
	b.callCount++
	// 模拟网络延迟
	time.Sleep(50 * time.Millisecond)

	if b.failMode {
		// 故障模式：模拟银行接口超时/报错
		return fmt.Errorf("银行接口超时")
	}
	// 正常模式：随机10%概率失败（模拟偶发错误）
	if rand.Float64() < 0.1 {
		return fmt.Errorf("银行接口偶发错误")
	}
	return nil
}

// ============================================================
// 支付服务：调用银行接口，外面包一层熔断器
// ============================================================

func payWithBreaker(cb *CircuitBreaker, bank *BankService, amount float64, reqID int) {
	err := cb.Execute(func() error {
		return bank.Deduct(amount)
	})

	if err == ErrCircuitOpen {
		fmt.Printf("  请求%3d: ⚡ 熔断器拦截，立刻返回失败（保护系统）\n", reqID)
	} else if err != nil {
		fmt.Printf("  请求%3d: ✗ 调用失败 - %v\n", reqID, err)
	} else {
		fmt.Printf("  请求%3d: ✓ 扣款成功 %.2f元\n", reqID, amount)
	}
}

func main() {
	// 熔断器配置：连续失败3次触发熔断，5秒后进入半开试探
	cb := NewCircuitBreaker("银行扣款", 3, 5*time.Second)
	bank := &BankService{}

	// ============================================================
	// 场景一：正常状态，发5个请求
	// ============================================================
	fmt.Println("========================================")
	fmt.Println(" 场景一：银行接口正常，发5个请求")
	fmt.Println("========================================")
	for i := 1; i <= 5; i++ {
		payWithBreaker(cb, bank, 100, i)
	}
	cb.Stats()

	// ============================================================
	// 场景二：银行接口故障，触发熔断
	// ============================================================
	fmt.Println("\n========================================")
	fmt.Println(" 场景二：银行接口故障，观察熔断触发过程")
	fmt.Println("========================================")
	bank.failMode = true // 模拟银行宕机

	for i := 6; i <= 15; i++ {
		payWithBreaker(cb, bank, 100, i)
		time.Sleep(100 * time.Millisecond)
	}
	cb.Stats()

	// ============================================================
	// 场景三：等待熔断器进入半开，银行还没恢复
	// ============================================================
	fmt.Println("\n========================================")
	fmt.Println(" 场景三：等5秒后熔断器试探，但银行还没恢复")
	fmt.Println("========================================")
	fmt.Println("  等待5秒...")
	time.Sleep(5 * time.Second)

	// 发一个请求触发半开试探
	payWithBreaker(cb, bank, 100, 16)
	cb.Stats()

	// ============================================================
	// 场景四：银行恢复，熔断器自动关闭
	// ============================================================
	fmt.Println("\n========================================")
	fmt.Println(" 场景四：再等5秒，银行恢复，熔断器自动关闭")
	fmt.Println("========================================")
	fmt.Println("  等待5秒...")
	time.Sleep(5 * time.Second)

	bank.failMode = false // 模拟银行恢复

	// 发一个请求触发半开试探，这次会成功
	payWithBreaker(cb, bank, 100, 17)

	// 继续发几个请求，确认已经恢复正常
	fmt.Println("\n  银行恢复后继续发5个请求：")
	for i := 18; i <= 22; i++ {
		payWithBreaker(cb, bank, 100, i)
	}
	cb.Stats()

	fmt.Printf("\n  银行接口实际被调用次数: %d 次（其余被熔断器拦截）\n", bank.callCount)
	fmt.Println("\n========================================")
	fmt.Println(" 关键收益：熔断期间银行接口0调用")
	fmt.Println(" 没有熔断器：每次请求都会等待超时")
	fmt.Println(" 有熔断器：立刻返回，系统不会被拖垮")
	fmt.Println("========================================")
}
