package main

import (
	"fmt"
	"strings"
	"time"
)

// ========================================
// 模板方法模式 Template Method Pattern
//
// 场景：数据报表生成系统
// 需要支持：CSV报表、JSON报表、HTML报表
//
// 解决的问题：
//   多种报表的生成流程完全一样：
//     1. 连接数据源
//     2. 查询数据
//     3. 格式化数据   ← 每种报表不同
//     4. 输出报表     ← 每种报表不同
//     5. 关闭连接
//
//   把不变的流程定义在父类（模板），
//   把变化的步骤留给子类实现
//   子类不能改流程顺序，只能填充具体步骤
//
// 注意：Go 没有继承，用接口 + 组合来实现
// ========================================

// ----------------------------------------
// 数据模型
// ----------------------------------------

type SalesRecord struct {
	Date     string
	Product  string
	Quantity int
	Amount   float64
	Region   string
}

// ----------------------------------------
// 模板接口：定义可变步骤
// ----------------------------------------

type ReportFormatter interface {
	FormatHeader(title string) string
	FormatRow(record SalesRecord) string
	FormatFooter(total float64, count int) string
	FileExtension() string
}

// ----------------------------------------
// 模板方法：定义固定流程（不变的骨架）
// ----------------------------------------

type ReportGenerator struct {
	formatter ReportFormatter // 持有具体格式化器
}

func NewReportGenerator(f ReportFormatter) *ReportGenerator {
	return &ReportGenerator{formatter: f}
}

// Generate 是模板方法 —— 流程固定，步骤可变
func (g *ReportGenerator) Generate(title string, records []SalesRecord) string {
	fmt.Printf("  [模板] 开始生成报表: %s.%s\n", title, g.formatter.FileExtension())

	// 步骤1：固定 - 连接数据源
	g.connectDataSource()

	// 步骤2：固定 - 统计数据
	total, count := g.calcStats(records)

	// 步骤3：可变 - 各子类实现格式化
	var sb strings.Builder
	sb.WriteString(g.formatter.FormatHeader(title))
	for _, r := range records {
		sb.WriteString(g.formatter.FormatRow(r))
	}
	sb.WriteString(g.formatter.FormatFooter(total, count))

	// 步骤4：固定 - 关闭连接
	g.closeDataSource()

	fmt.Printf("  [模板] 报表生成完成，共 %d 条记录，合计 ¥%.2f\n\n", count, total)
	return sb.String()
}

// 固定步骤：所有报表共用
func (g *ReportGenerator) connectDataSource() {
	fmt.Println("  [模板] 连接数据源...")
	time.Sleep(5 * time.Millisecond)
}

func (g *ReportGenerator) closeDataSource() {
	fmt.Println("  [模板] 关闭数据源连接")
}

func (g *ReportGenerator) calcStats(records []SalesRecord) (total float64, count int) {
	for _, r := range records {
		total += r.Amount
		count++
	}
	return
}

// ----------------------------------------
// 具体格式化器一：CSV
// ----------------------------------------

type CSVFormatter struct{}

func (f *CSVFormatter) FileExtension() string { return "csv" }

func (f *CSVFormatter) FormatHeader(title string) string {
	return fmt.Sprintf("# %s\n日期,产品,数量,金额,地区\n", title)
}

func (f *CSVFormatter) FormatRow(r SalesRecord) string {
	return fmt.Sprintf("%s,%s,%d,%.2f,%s\n",
		r.Date, r.Product, r.Quantity, r.Amount, r.Region)
}

func (f *CSVFormatter) FormatFooter(total float64, count int) string {
	return fmt.Sprintf("# 合计：%d 条记录，总金额 ¥%.2f\n", count, total)
}

// ----------------------------------------
// 具体格式化器二：JSON
// ----------------------------------------

type JSONFormatter struct{}

func (f *JSONFormatter) FileExtension() string { return "json" }

func (f *JSONFormatter) FormatHeader(title string) string {
	return fmt.Sprintf("{\n  \"title\": \"%s\",\n  \"records\": [\n", title)
}

func (f *JSONFormatter) FormatRow(r SalesRecord) string {
	return fmt.Sprintf(
		"    {\"date\":\"%s\",\"product\":\"%s\",\"qty\":%d,\"amount\":%.2f,\"region\":\"%s\"},\n",
		r.Date, r.Product, r.Quantity, r.Amount, r.Region)
}

func (f *JSONFormatter) FormatFooter(total float64, count int) string {
	return fmt.Sprintf("  ],\n  \"summary\": {\"count\": %d, \"total\": %.2f}\n}\n", count, total)
}

// ----------------------------------------
// 具体格式化器三：HTML
// ----------------------------------------

type HTMLFormatter struct{}

func (f *HTMLFormatter) FileExtension() string { return "html" }

func (f *HTMLFormatter) FormatHeader(title string) string {
	return fmt.Sprintf(`<html><body>
<h2>%s</h2>
<table border="1">
<tr><th>日期</th><th>产品</th><th>数量</th><th>金额</th><th>地区</th></tr>
`, title)
}

func (f *HTMLFormatter) FormatRow(r SalesRecord) string {
	return fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>¥%.2f</td><td>%s</td></tr>\n",
		r.Date, r.Product, r.Quantity, r.Amount, r.Region)
}

func (f *HTMLFormatter) FormatFooter(total float64, count int) string {
	return fmt.Sprintf(`<tr><td colspan="5"><b>合计：%d 条，¥%.2f</b></td></tr>
</table></body></html>
`, count, total)
}

// ----------------------------------------
// Hook 钩子方法：子类可选择性覆盖
// 模板方法的扩展点，不强制实现
// ----------------------------------------

type AuditableFormatter interface {
	ReportFormatter
	OnBeforeFormat() string // 钩子：格式化前执行（可选）
	OnAfterFormat() string  // 钩子：格式化后执行（可选）
}

type AuditCSVFormatter struct {
	CSVFormatter
	operator string
}

func (f *AuditCSVFormatter) OnBeforeFormat() string {
	return fmt.Sprintf("# 操作人: %s  时间: %s\n", f.operator, time.Now().Format("2006-01-02 15:04:05"))
}

func (f *AuditCSVFormatter) OnAfterFormat() string {
	return "# 报表已加密，仅限内部使用\n"
}

// 带钩子的生成器
type AuditReportGenerator struct {
	formatter AuditableFormatter
}

func (g *AuditReportGenerator) Generate(title string, records []SalesRecord) string {
	var sb strings.Builder
	sb.WriteString(g.formatter.OnBeforeFormat()) // 钩子前置
	sb.WriteString(g.formatter.FormatHeader(title))
	for _, r := range records {
		sb.WriteString(g.formatter.FormatRow(r))
	}
	var total float64
	for _, r := range records {
		total += r.Amount
	}
	sb.WriteString(g.formatter.FormatFooter(total, len(records)))
	sb.WriteString(g.formatter.OnAfterFormat()) // 钩子后置
	return sb.String()
}

// ========================================
// main
// ========================================

func main() {
	records := []SalesRecord{
		{"2024-01-15", "MacBook Pro", 2, 29998.00, "华东"},
		{"2024-01-16", "iPhone 15", 5, 34995.00, "华南"},
		{"2024-01-17", "AirPods", 10, 12990.00, "华北"},
	}

	section("=== 模板方法：同一流程，不同格式输出 ===")

	// CSV 报表
	csvGen := NewReportGenerator(&CSVFormatter{})
	csvOutput := csvGen.Generate("2024年1月销售报表", records)
	fmt.Println("--- CSV 输出 ---")
	fmt.Print(csvOutput)

	// JSON 报表
	jsonGen := NewReportGenerator(&JSONFormatter{})
	jsonOutput := jsonGen.Generate("2024年1月销售报表", records)
	fmt.Println("--- JSON 输出 ---")
	fmt.Print(jsonOutput)

	// HTML 报表
	htmlGen := NewReportGenerator(&HTMLFormatter{})
	htmlOutput := htmlGen.Generate("2024年1月销售报表", records)
	fmt.Println("--- HTML 输出 ---")
	fmt.Print(htmlOutput)

	section("=== Hook 钩子方法：可选扩展点 ===")
	auditGen := &AuditReportGenerator{
		formatter: &AuditCSVFormatter{operator: "admin"},
	}
	fmt.Print(auditGen.Generate("审计报表", records))

	section("=== 核心结构总结 ===")
	fmt.Println(`
  ReportGenerator（模板）          ReportFormatter（接口）
  ┌─────────────────────┐         ┌──────────────────────┐
  │ Generate()          │ 持有    │ FormatHeader()       │
  │   connectDB()  ←固定│ ──────→ │ FormatRow()          │
  │   formatter.Header()←可变│    │ FormatFooter()       │
  │   formatter.Row()  ←可变│     └──────────────────────┘
  │   formatter.Footer()←可变│           ↑ 实现
  │   closeDB()    ←固定│      CSV  JSON  HTML  ...
  └─────────────────────┘
  流程骨架在模板里写死，变化的步骤由具体类填充`)
}

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("-", 55))
}
