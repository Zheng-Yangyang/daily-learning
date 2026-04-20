package main

import (
	"fmt"
	"math"
	"strings"
)

// ========================================
// 策略模式 Strategy Pattern
//
// 场景：电商支付 + 促销折扣系统
//
// 解决的问题：
//   同一个行为有多种实现方式，且需要运行时切换
//   不用策略模式时，代码长这样：
//
//   func Pay(method string, amount float64) {
//       if method == "alipay" { ... }
//       else if method == "wechat" { ... }
//       else if method == "card" { ... }
//       // 每加一种支付方式就改这里，越来越长
//   }
//
//   用策略模式：把每种算法封装成独立的策略类
//   运行时传入哪个策略就用哪个，主流程完全不变
// ========================================

// ----------------------------------------
// 场景一：支付策略
// ----------------------------------------

type PaymentStrategy interface {
	Pay(amount float64) (string, error)
	Name() string
}

// 支付宝
type AlipayStrategy struct {
	userID string
}

func (s *AlipayStrategy) Name() string { return "支付宝" }
func (s *AlipayStrategy) Pay(amount float64) (string, error) {
	return fmt.Sprintf("✅ 支付宝扣款 ¥%.2f，账号: %s，流水号: ALI-%d",
		amount, s.userID, int(amount*100)), nil
}

// 微信支付
type WechatPayStrategy struct {
	openID string
}

func (s *WechatPayStrategy) Name() string { return "微信支付" }
func (s *WechatPayStrategy) Pay(amount float64) (string, error) {
	return fmt.Sprintf("✅ 微信扣款 ¥%.2f，openID: %s，流水号: WX-%d",
		amount, s.openID, int(amount*100)), nil
}

// 银行卡
type BankCardStrategy struct {
	cardNo string
}

func (s *BankCardStrategy) Name() string { return "银行卡" }
func (s *BankCardStrategy) Pay(amount float64) (string, error) {
	masked := s.cardNo[:4] + "****" + s.cardNo[len(s.cardNo)-4:]
	return fmt.Sprintf("✅ 银行卡扣款 ¥%.2f，卡号: %s，流水号: BANK-%d",
		amount, masked, int(amount*100)), nil
}

// 余额不足时模拟失败
type WalletStrategy struct {
	balance float64
}

func (s *WalletStrategy) Name() string { return "钱包" }
func (s *WalletStrategy) Pay(amount float64) (string, error) {
	if s.balance < amount {
		return "", fmt.Errorf("钱包余额不足，当前 ¥%.2f，需要 ¥%.2f", s.balance, amount)
	}
	s.balance -= amount
	return fmt.Sprintf("✅ 钱包扣款 ¥%.2f，剩余余额 ¥%.2f", amount, s.balance), nil
}

// Context：订单结算，持有策略并执行
type Checkout struct {
	strategy PaymentStrategy
}

func (c *Checkout) SetStrategy(s PaymentStrategy) {
	c.strategy = s
	fmt.Printf("  [切换支付方式] → %s\n", s.Name())
}

func (c *Checkout) Pay(amount float64) {
	fmt.Printf("  [结算] 金额 ¥%.2f，使用 %s\n", amount, c.strategy.Name())
	result, err := c.strategy.Pay(amount)
	if err != nil {
		fmt.Printf("  ❌ 支付失败: %v\n", err)
		return
	}
	fmt.Printf("  %s\n", result)
}

// ----------------------------------------
// 场景二：折扣策略（策略带参数计算）
// ----------------------------------------

type DiscountStrategy interface {
	Calculate(originalPrice float64) float64
	Describe() string
}

// 无折扣
type NoDiscount struct{}

func (d *NoDiscount) Calculate(price float64) float64 { return price }
func (d *NoDiscount) Describe() string                { return "原价" }

// 百分比折扣（九折、八折...）
type PercentDiscount struct {
	percent float64 // 0.9 = 九折
}

func (d *PercentDiscount) Calculate(price float64) float64 {
	return math.Round(price*d.percent*100) / 100
}
func (d *PercentDiscount) Describe() string {
	return fmt.Sprintf("%.0f折", d.percent*10)
}

// 满减（满200减50）
type FullReductionDiscount struct {
	threshold float64
	reduction float64
}

func (d *FullReductionDiscount) Calculate(price float64) float64 {
	if price >= d.threshold {
		return price - d.reduction
	}
	return price
}
func (d *FullReductionDiscount) Describe() string {
	return fmt.Sprintf("满%.0f减%.0f", d.threshold, d.reduction)
}

// 阶梯折扣（买的越多越便宜）
type TieredDiscount struct {
	tiers []struct {
		minQty   int
		discount float64
	}
}

func NewTieredDiscount() *TieredDiscount {
	return &TieredDiscount{
		tiers: []struct {
			minQty   int
			discount float64
		}{
			{10, 0.95}, // 买10件以上 95折
			{20, 0.90}, // 买20件以上 90折
			{50, 0.85}, // 买50件以上 85折
		},
	}
}

func (d *TieredDiscount) CalculateWithQty(price float64, qty int) float64 {
	discount := 1.0
	for _, tier := range d.tiers {
		if qty >= tier.minQty {
			discount = tier.discount
		}
	}
	return math.Round(price*float64(qty)*discount*100) / 100
}

func (d *TieredDiscount) Calculate(price float64) float64 { return price }
func (d *TieredDiscount) Describe() string                { return "阶梯折扣" }

// 购物车：使用折扣策略
type Cart struct {
	items []struct {
		name  string
		price float64
		qty   int
	}
	discount DiscountStrategy
}

func NewCart() *Cart {
	return &Cart{discount: &NoDiscount{}}
}

func (c *Cart) AddItem(name string, price float64, qty int) {
	c.items = append(c.items, struct {
		name  string
		price float64
		qty   int
	}{name, price, qty})
}

func (c *Cart) SetDiscount(d DiscountStrategy) {
	c.discount = d
}

func (c *Cart) Checkout() {
	fmt.Printf("\n  购物车结算 [折扣策略: %s]\n", c.discount.Describe())
	fmt.Println("  " + strings.Repeat("-", 40))

	var total float64
	for _, item := range c.items {
		subtotal := item.price * float64(item.qty)
		total += subtotal
		fmt.Printf("  %-10s x%d  ¥%.2f × %d = ¥%.2f\n",
			item.name, item.qty, item.price, item.qty, subtotal)
	}

	discounted := c.discount.Calculate(total)
	fmt.Printf("  %s\n", strings.Repeat("-", 40))
	fmt.Printf("  原价合计: ¥%.2f\n", total)
	if discounted != total {
		fmt.Printf("  优惠金额: -¥%.2f\n", total-discounted)
	}
	fmt.Printf("  实付金额: ¥%.2f\n", discounted)
}

// ========================================
// main
// ========================================

func main() {
	section("=== 策略模式一：支付策略（运行时切换）===")

	checkout := &Checkout{}

	// 用支付宝
	checkout.SetStrategy(&AlipayStrategy{userID: "user@alipay.com"})
	checkout.Pay(299.00)

	fmt.Println()

	// 切换到微信支付
	checkout.SetStrategy(&WechatPayStrategy{openID: "ox7Dv0abc123"})
	checkout.Pay(299.00)

	fmt.Println()

	// 切换到银行卡
	checkout.SetStrategy(&BankCardStrategy{cardNo: "6222021234567890"})
	checkout.Pay(299.00)

	fmt.Println()

	// 钱包余额不足
	checkout.SetStrategy(&WalletStrategy{balance: 100.00})
	checkout.Pay(299.00)

	// 钱包余额够
	checkout.SetStrategy(&WalletStrategy{balance: 500.00})
	checkout.Pay(299.00)

	section("=== 策略模式二：折扣策略（算法替换）===")

	cart := NewCart()
	cart.AddItem("MacBook Pro", 14999.00, 1)
	cart.AddItem("iPhone 15", 6999.00, 1)
	cart.AddItem("AirPods", 1299.00, 2)

	// 原价
	cart.SetDiscount(&NoDiscount{})
	cart.Checkout()

	// 九折
	cart.SetDiscount(&PercentDiscount{percent: 0.9})
	cart.Checkout()

	// 满减
	cart.SetDiscount(&FullReductionDiscount{threshold: 10000, reduction: 1000})
	cart.Checkout()

	// 满减但不达标
	smallCart := NewCart()
	smallCart.AddItem("鼠标垫", 99.00, 1)
	smallCart.SetDiscount(&FullReductionDiscount{threshold: 200, reduction: 30})
	smallCart.Checkout()

	section("=== 策略 vs if-else 的本质区别 ===")
	fmt.Println(`
  if-else 版本：                 策略模式版本：
  ┌─────────────────────┐       ┌──────────────────────┐
  │ if alipay { ... }   │       │ strategy.Pay(amount) │
  │ if wechat { ... }   │  →    │                      │
  │ if card   { ... }   │       │ 新增支付方式：        │
  │ // 每次都改这里     │       │ 只加一个新 struct    │
  └─────────────────────┘       │ 主流程零改动  ✅     │
                                └──────────────────────┘`)
}

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("-", 50))
}
