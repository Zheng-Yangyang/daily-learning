package main

import (
	"fmt"
	"strings"
)

// ========================================
// 责任链模式 Chain of Responsibility
//
// 场景：贷款审批系统
//
// 解决的问题：
//   一个请求需要经过多个处理节点
//   每个节点决定：自己处理 or 传给下一个
//   处理者之间解耦，可以灵活增删和重排顺序
//
// 和装饰器的区别：
//   装饰器：每层都会执行，增强功能，请求必然到达终点
//   责任链：某层处理后可以终止，请求不一定到达终点
// ========================================

// ----------------------------------------
// 贷款申请
// ----------------------------------------

type LoanRequest struct {
	ID            string
	ApplicantName string
	Amount        float64
	CreditScore   int     // 信用分 300-850
	HasCollateral bool    // 是否有抵押物
	Income        float64 // 月收入
}

func (r *LoanRequest) String() string {
	return fmt.Sprintf("申请人=%s 金额=¥%.0f 信用分=%d 月收入=¥%.0f 有抵押=%v",
		r.ApplicantName, r.Amount, r.CreditScore, r.Income, r.HasCollateral)
}

type ApprovalResult struct {
	Approved bool
	Handler  string
	Reason   string
}

func (r ApprovalResult) String() string {
	if r.Approved {
		return fmt.Sprintf("✅ 批准 by [%s]", r.Handler)
	}
	return fmt.Sprintf("❌ 拒绝 by [%s]：%s", r.Handler, r.Reason)
}

// ----------------------------------------
// 处理者接口
// ----------------------------------------

type Approver interface {
	SetNext(approver Approver) Approver // 返回 next，支持链式调用
	Approve(req *LoanRequest) ApprovalResult
	HandlerName() string
}

// ----------------------------------------
// 基础处理者：封装 next 逻辑，子类直接复用
// ----------------------------------------

type BaseApprover struct {
	next Approver
	name string
}

func (b *BaseApprover) SetNext(next Approver) Approver {
	b.next = next
	return next // 返回 next 支持链式：a.SetNext(b).SetNext(c)
}

func (b *BaseApprover) PassToNext(req *LoanRequest) ApprovalResult {
	if b.next != nil {
		fmt.Printf("    [%s] → 传递给下一级 [%s]\n", b.name, b.next.HandlerName())
		return b.next.Approve(req)
	}
	// 链尾还没人处理，默认拒绝
	return ApprovalResult{Approved: false, Handler: b.name, Reason: "超出所有审批人权限"}
}

func (b *BaseApprover) HandlerName() string { return b.name }

// ----------------------------------------
// 具体处理者
// ----------------------------------------

// 第一关：风控系统（自动化）—— 基础资质校验
type RiskControlSystem struct {
	BaseApprover
}

func NewRiskControl() *RiskControlSystem {
	return &RiskControlSystem{BaseApprover{name: "风控系统"}}
}

func (h *RiskControlSystem) Approve(req *LoanRequest) ApprovalResult {
	fmt.Printf("    [风控系统] 检查基础资质：信用分=%d 月收入=¥%.0f\n", req.CreditScore, req.Income)

	if req.CreditScore < 600 {
		return ApprovalResult{false, h.name, fmt.Sprintf("信用分 %d 不足 600", req.CreditScore)}
	}
	if req.Income < 3000 {
		return ApprovalResult{false, h.name, fmt.Sprintf("月收入 ¥%.0f 不足 ¥3000", req.Income)}
	}

	fmt.Printf("    [风控系统] ✓ 基础资质通过\n")
	return h.PassToNext(req)
}

// 第二关：客户经理 —— 小额贷款
type AccountManager struct {
	BaseApprover
	limit float64
}

func NewAccountManager(limit float64) *AccountManager {
	return &AccountManager{BaseApprover{name: "客户经理"}, limit}
}

func (h *AccountManager) Approve(req *LoanRequest) ApprovalResult {
	fmt.Printf("    [客户经理] 审批权限 ¥%.0f，申请金额 ¥%.0f\n", h.limit, req.Amount)

	if req.Amount <= h.limit {
		return ApprovalResult{true, h.name, ""}
	}

	return h.PassToNext(req)
}

// 第三关：支行行长 —— 中额贷款
type BranchManager struct {
	BaseApprover
	limit float64
}

func NewBranchManager(limit float64) *BranchManager {
	return &BranchManager{BaseApprover{name: "支行行长"}, limit}
}

func (h *BranchManager) Approve(req *LoanRequest) ApprovalResult {
	fmt.Printf("    [支行行长] 审批权限 ¥%.0f，申请金额 ¥%.0f\n", h.limit, req.Amount)

	if req.Amount <= h.limit {
		// 行长额外要求：中额贷款需要有抵押物
		if !req.HasCollateral {
			return ApprovalResult{false, h.name, "贷款超过 ¥10万，需要提供抵押物"}
		}
		return ApprovalResult{true, h.name, ""}
	}

	return h.PassToNext(req)
}

// 第四关：总行审委会 —— 大额贷款
type HeadOfficeCommittee struct {
	BaseApprover
	limit float64
}

func NewHeadOffice(limit float64) *HeadOfficeCommittee {
	return &HeadOfficeCommittee{BaseApprover{name: "总行审委会"}, limit}
}

func (h *HeadOfficeCommittee) Approve(req *LoanRequest) ApprovalResult {
	fmt.Printf("    [总行审委会] 审批权限 ¥%.0f，申请金额 ¥%.0f\n", h.limit, req.Amount)

	if req.Amount <= h.limit {
		if !req.HasCollateral {
			return ApprovalResult{false, h.name, "大额贷款必须有抵押物"}
		}
		if req.CreditScore < 750 {
			return ApprovalResult{false, h.name, fmt.Sprintf("大额贷款信用分需达到 750，当前 %d", req.CreditScore)}
		}
		return ApprovalResult{true, h.name, ""}
	}

	return h.PassToNext(req)
}

// ----------------------------------------
// 构建责任链的工厂函数
// ----------------------------------------

func BuildLoanChain() Approver {
	risk := NewRiskControl()
	manager := NewAccountManager(50000)  // 5万以内
	branch := NewBranchManager(500000)   // 50万以内
	headOffice := NewHeadOffice(5000000) // 500万以内

	// 链式设置：risk → manager → branch → headOffice
	risk.SetNext(manager).SetNext(branch).SetNext(headOffice)

	return risk // 返回链头
}

// ========================================
// main
// ========================================

func main() {
	chain := BuildLoanChain()

	testCases := []*LoanRequest{
		{
			ID: "L001", ApplicantName: "张三",
			Amount: 30000, CreditScore: 720,
			Income: 8000, HasCollateral: false,
		},
		{
			ID: "L002", ApplicantName: "李四",
			Amount: 200000, CreditScore: 780,
			Income: 20000, HasCollateral: true,
		},
		{
			ID: "L003", ApplicantName: "王五",
			Amount: 1000000, CreditScore: 800,
			Income: 50000, HasCollateral: true,
		},
		{
			ID: "L004", ApplicantName: "赵六",
			Amount: 20000, CreditScore: 550, // 信用分不足
			Income: 6000, HasCollateral: false,
		},
		{
			ID: "L005", ApplicantName: "孙七",
			Amount: 200000, CreditScore: 760,
			Income: 15000, HasCollateral: false, // 无抵押物
		},
		{
			ID: "L006", ApplicantName: "周八",
			Amount:      8000000, // 超出所有人权限
			CreditScore: 850, Income: 100000, HasCollateral: true,
		},
	}

	for _, req := range testCases {
		section(fmt.Sprintf("=== 申请 %s：%s ===", req.ID, req))
		result := chain.Approve(req)
		fmt.Printf("  最终结果：%s\n", result)
	}

	section("=== 责任链 vs 装饰器 核心区别 ===")
	fmt.Println(`
  装饰器：                        责任链：
  每层都执行，请求必达终点         某层可拦截，请求不一定到终点

  Logging → Auth → Handler        风控 → 经理 → 行长 → 总行
     ↓         ↓        ↓           ↓
  全部执行                        信用分不足 → 直接拒绝，后续不执行

  目的：增强功能                  目的：逐级处理/拦截`)
}

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("-", 60))
}
