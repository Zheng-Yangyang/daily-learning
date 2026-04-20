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

	// 1. terms 聚合 — 按品牌分组计数
	termsAgg(es, ctx)

	// 2. stats 聚合 — 价格统计（最大/最小/平均/总和）
	statsAgg(es, ctx)

	// 3. range 聚合 — 按价格区间分桶
	rangeAgg(es, ctx)

	// 4. 嵌套聚合 — 每个品牌的价格统计
	nestedAgg(es, ctx)

	// 5. 聚合 + 查询 — 先过滤再聚合
	filteredAgg(es, ctx)
}

// ----------------------------------------
// 通用搜索
// ----------------------------------------
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
// 1. terms 聚合 — 按字段值分组，类似 GROUP BY
// ----------------------------------------
func termsAgg(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"size": 0,
		"aggs": {
			"by_brand": {
				"terms": {
					"field": "brand",
					"order": { "_count": "desc" }
				}
			}
		}
	}`
	// size:0 表示不返回原始文档，只返回聚合结果

	result := doSearch(es, ctx, query)
	aggs := result["aggregations"].(map[string]interface{})
	buckets := aggs["by_brand"].(map[string]interface{})["buckets"].([]interface{})

	fmt.Println("【1】terms 聚合 — 各品牌商品数量：")
	for _, b := range buckets {
		bucket := b.(map[string]interface{})
		fmt.Printf("   %-10s : %.0f 件\n", bucket["key"], bucket["doc_count"])
	}
	fmt.Println()
}

// ----------------------------------------
// 2. stats 聚合 — 数值字段的统计信息
// ----------------------------------------
func statsAgg(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"size": 0,
		"aggs": {
			"price_stats": {
				"stats": { "field": "price" }
			}
		}
	}`

	result := doSearch(es, ctx, query)
	aggs := result["aggregations"].(map[string]interface{})
	stats := aggs["price_stats"].(map[string]interface{})

	fmt.Println("【2】stats 聚合 — 全部商品价格统计：")
	fmt.Printf("   数量: %.0f\n", stats["count"])
	fmt.Printf("   最低: ¥%.0f\n", stats["min"])
	fmt.Printf("   最高: ¥%.0f\n", stats["max"])
	fmt.Printf("   均价: ¥%.2f\n", stats["avg"])
	fmt.Printf("   总额: ¥%.0f\n", stats["sum"])
	fmt.Println()
}

// ----------------------------------------
// 3. range 聚合 — 按数值区间分桶
// ----------------------------------------
func rangeAgg(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"size": 0,
		"aggs": {
			"price_range": {
				"range": {
					"field": "price",
					"ranges": [
						{ "key": "低价 (< 5000)",       "to": 5000 },
						{ "key": "中价 (5000~9999)",  "from": 5000, "to": 10000 },
						{ "key": "高价 (>= 10000)",   "from": 10000 }
					]
				}
			}
		}
	}`

	result := doSearch(es, ctx, query)
	aggs := result["aggregations"].(map[string]interface{})
	buckets := aggs["price_range"].(map[string]interface{})["buckets"].([]interface{})

	fmt.Println("【3】range 聚合 — 按价格区间分桶：")
	for _, b := range buckets {
		bucket := b.(map[string]interface{})
		fmt.Printf("   %-20s : %.0f 件\n", bucket["key"], bucket["doc_count"])
	}
	fmt.Println()
}

// ----------------------------------------
//  4. 嵌套聚合 — 每个品牌下再做价格统计
//     类似 SQL: SELECT brand, AVG(price), MAX(price) GROUP BY brand
//
// ----------------------------------------
func nestedAgg(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"size": 0,
		"aggs": {
			"by_brand": {
				"terms": { "field": "brand" },
				"aggs": {
					"avg_price": { "avg":  { "field": "price" } },
					"max_price": { "max":  { "field": "price" } },
					"min_price": { "min":  { "field": "price" } }
				}
			}
		}
	}`

	result := doSearch(es, ctx, query)
	aggs := result["aggregations"].(map[string]interface{})
	buckets := aggs["by_brand"].(map[string]interface{})["buckets"].([]interface{})

	fmt.Println("【4】嵌套聚合 — 每个品牌的价格统计：")
	fmt.Printf("   %-10s | %-10s | %-10s | %-10s\n", "品牌", "均价", "最高", "最低")
	fmt.Println("   " + strings.Repeat("-", 46))
	for _, b := range buckets {
		bucket := b.(map[string]interface{})
		brand := bucket["key"]
		avg := bucket["avg_price"].(map[string]interface{})["value"].(float64)
		max := bucket["max_price"].(map[string]interface{})["value"].(float64)
		min := bucket["min_price"].(map[string]interface{})["value"].(float64)
		fmt.Printf("   %-10s | ¥%-9.0f | ¥%-9.0f | ¥%-9.0f\n", brand, avg, max, min)
	}
	fmt.Println()
}

// ----------------------------------------
//  5. 聚合 + 查询 — 只统计手机类商品
//     query 先过滤，aggs 在过滤结果上聚合
//
// ----------------------------------------
func filteredAgg(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"size": 0,
		"query": {
			"match": { "description": "手机" }
		},
		"aggs": {
			"by_brand": {
				"terms": { "field": "brand" }
			},
			"avg_price": {
				"avg": { "field": "price" }
			}
		}
	}`

	result := doSearch(es, ctx, query)
	aggs := result["aggregations"].(map[string]interface{})

	// 品牌分布
	buckets := aggs["by_brand"].(map[string]interface{})["buckets"].([]interface{})
	avgPrice := aggs["avg_price"].(map[string]interface{})["value"].(float64)

	hits := result["hits"].(map[string]interface{})
	total := hits["total"].(map[string]interface{})["value"].(float64)

	fmt.Println("【5】聚合 + 查询 — 仅统计「手机」类商品：")
	fmt.Printf("   共 %.0f 件手机，均价 ¥%.2f\n", total, avgPrice)
	fmt.Println("   品牌分布：")
	for _, b := range buckets {
		bucket := b.(map[string]interface{})
		fmt.Printf("   %-10s : %.0f 件\n", bucket["key"], bucket["doc_count"])
	}
	fmt.Println()
}

// ----------------------------------------
// 初始化测试数据
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
