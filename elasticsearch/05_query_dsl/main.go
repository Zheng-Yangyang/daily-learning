package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
)

type Product struct {
	Name        string  `json:"name"`
	Brand       string  `json:"brand"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Description string  `json:"description"`
	CreatedAt   string  `json:"created_at"`
}

const indexName = "products"

func main() {
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}
	ctx := context.Background()

	setupTestData(es, ctx)
	fmt.Println("========================================")

	// 1. must（AND）：品牌是 Apple 且 价格 < 10000
	boolMust(es, ctx)

	// 2. should（OR）：品牌是 Apple 或 Samsung
	boolShould(es, ctx)

	// 3. must_not（NOT）：排除品牌 Apple
	boolMustNot(es, ctx)

	// 4. 组合：手机类 + 价格范围 + 排除某品牌
	boolCombined(es, ctx)

	// 5. filter：不影响评分的过滤（比 must 性能更好）
	boolFilter(es, ctx)
}

// ----------------------------------------
// 通用打印
// ----------------------------------------
func printResults(label string, res map[string]interface{}) {
	hits := res["hits"].(map[string]interface{})
	total := hits["total"].(map[string]interface{})["value"].(float64)
	hitList := hits["hits"].([]interface{})

	fmt.Printf("%s\n   命中 %.0f 条：\n", label, total)
	for _, hit := range hitList {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		score := h["_score"]
		fmt.Printf("   - %-20s | %-10s | ¥%.0f | score: %v\n",
			source["name"], source["brand"], source["price"], score)
	}
	fmt.Println()
}

func doSearch(es *elasticsearch.Client, ctx context.Context, query string) map[string]interface{} {
	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(indexName),
		es.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil || res.IsError() {
		log.Fatalf("搜索失败: %s", res.String())
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)
	return result
}

// ----------------------------------------
// 1. must — 所有条件都必须满足（AND）
// ----------------------------------------
func boolMust(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"bool": {
				"must": [
					{ "term":  { "brand": "Apple" } },
					{ "range": { "price": { "lt": 10000 } } }
				]
			}
		}
	}`
	result := doSearch(es, ctx, query)
	printResults("【1】must（AND）— Apple 品牌 且 价格 < 10000：", result)
}

// ----------------------------------------
//  2. should — 满足任意条件即可（OR）
//     minimum_should_match: 至少匹配几个
//
// ----------------------------------------
func boolShould(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"bool": {
				"should": [
					{ "term": { "brand": "Apple" } },
					{ "term": { "brand": "Samsung" } }
				],
				"minimum_should_match": 1
			}
		}
	}`
	result := doSearch(es, ctx, query)
	printResults("【2】should（OR）— Apple 或 Samsung：", result)
}

// ----------------------------------------
// 3. must_not — 排除匹配的文档（NOT）
// ----------------------------------------
func boolMustNot(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"bool": {
				"must_not": [
					{ "term": { "brand": "Apple" } }
				]
			}
		}
	}`
	result := doSearch(es, ctx, query)
	printResults("【3】must_not（NOT）— 排除 Apple：", result)
}

// ----------------------------------------
//  4. 组合查询 — 实际业务常用写法
//     搜索：描述含「手机」+ 价格 3000~9000 + 排除 Samsung
//
// ----------------------------------------
func boolCombined(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"bool": {
				"must": [
					{ "match": { "description": "手机" } }
				],
				"filter": [
					{ "range": { "price": { "gte": 3000, "lte": 9000 } } }
				],
				"must_not": [
					{ "term": { "brand": "Samsung" } }
				]
			}
		},
		"sort": [{ "price": "asc" }]
	}`
	result := doSearch(es, ctx, query)
	printResults("【4】组合查询 — 描述含「手机」+ 价格3000~9000 + 排除Samsung：", result)
}

// ----------------------------------------
//  5. filter — 纯过滤，不计算相关性评分
//     适合做条件筛选，性能优于 must（结果会被缓存）
//
// ----------------------------------------
func boolFilter(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"bool": {
				"filter": [
					{ "term":  { "brand": "Xiaomi" } },
					{ "range": { "stock": { "gte": 100 } } }
				]
			}
		}
	}`
	result := doSearch(es, ctx, query)
	// filter 模式下 score 全为 null，因为不参与评分计算
	printResults("【5】filter — Xiaomi 且库存 >= 100（score 为 null）：", result)
}

// ----------------------------------------
// 初始化测试数据（同上一节）
// ----------------------------------------
func setupTestData(es *elasticsearch.Client, ctx context.Context) {
	es.Indices.Delete([]string{indexName}, es.Indices.Delete.WithContext(ctx))
	mapping := `{
		"mappings": {
			"properties": {
				"name":        { "type": "text" },
				"brand":       { "type": "keyword" },
				"price":       { "type": "float" },
				"stock":       { "type": "integer" },
				"description": { "type": "text" },
				"created_at":  { "type": "date" }
			}
		}
	}`
	res, _ := es.Indices.Create(indexName,
		es.Indices.Create.WithBody(strings.NewReader(mapping)),
	)
	res.Body.Close()

	products := []Product{
		{Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999, Stock: 50, Description: "苹果旗舰手机 A17芯片 钛金属边框", CreatedAt: "2024-01-01"},
		{Name: "iPhone 15", Brand: "Apple", Price: 5999, Stock: 120, Description: "苹果手机 A16芯片 铝合金边框", CreatedAt: "2024-01-01"},
		{Name: "MacBook Pro 14", Brand: "Apple", Price: 14999, Stock: 30, Description: "苹果笔记本 M3芯片 高性能", CreatedAt: "2024-02-01"},
		{Name: "Galaxy S24 Ultra", Brand: "Samsung", Price: 9999, Stock: 60, Description: "三星旗舰手机 骁龙8Gen3 S Pen", CreatedAt: "2024-01-15"},
		{Name: "Galaxy S24", Brand: "Samsung", Price: 6999, Stock: 80, Description: "三星手机 骁龙8Gen3 轻薄设计", CreatedAt: "2024-01-15"},
		{Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999, Stock: 200, Description: "小米旗舰手机 骁龙8Gen3 徕卡影像", CreatedAt: "2024-01-20"},
		{Name: "小米14", Brand: "Xiaomi", Price: 3999, Stock: 300, Description: "小米手机 骁龙8Gen3 性价比之王", CreatedAt: "2024-01-20"},
	}
	for _, p := range products {
		body, _ := json.Marshal(p)
		res, _ := es.Index(indexName, strings.NewReader(string(body)),
			es.Index.WithContext(ctx))
		res.Body.Close()
	}
	es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))
	fmt.Printf("✅ 测试数据插入完成，共 %d 条\n", len(products))
}
