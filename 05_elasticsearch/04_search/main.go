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

	// 准备测试数据
	setupTestData(es, ctx)

	fmt.Println("========================================")

	// 1. 查询所有文档
	searchAll(es, ctx)

	// 2. 全文搜索（match）
	matchSearch(es, ctx)

	// 3. 精确匹配（term）
	termSearch(es, ctx)

	// 4. 范围查询（range）
	rangeSearch(es, ctx)

	// 5. 多字段搜索（multi_match）
	multiMatchSearch(es, ctx)
}

// ----------------------------------------
// 插入测试数据
// ----------------------------------------
func setupTestData(es *elasticsearch.Client, ctx context.Context) {
	// 删除旧索引重建
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

	// 批量插入测试商品
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
		res, _ := es.Index(indexName,
			strings.NewReader(string(body)),
			es.Index.WithContext(ctx),
		)
		res.Body.Close()
	}

	// 手动刷新，确保数据可被搜索
	es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))
	fmt.Printf("✅ 测试数据插入完成，共 %d 条\n", len(products))
}

// ----------------------------------------
// 通用：打印搜索结果
// ----------------------------------------
func printResults(res map[string]interface{}) {
	hits := res["hits"].(map[string]interface{})
	total := hits["total"].(map[string]interface{})["value"].(float64)
	hitList := hits["hits"].([]interface{})

	fmt.Printf("   共命中 %.0f 条，返回 %d 条：\n", total, len(hitList))
	for _, hit := range hitList {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		fmt.Printf("   - %-20s | %-10s | ¥%.0f\n",
			source["name"], source["brand"], source["price"])
	}
	fmt.Println()
}

// ----------------------------------------
// 1. 查询所有文档 match_all
// ----------------------------------------
func searchAll(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": { "match_all": {} },
		"size": 10
	}`

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

	fmt.Println("【1】match_all — 查询所有：")
	printResults(result)
}

// ----------------------------------------
// 2. 全文搜索 match（会分词）
// ----------------------------------------
func matchSearch(es *elasticsearch.Client, ctx context.Context) {
	// match 会对搜索词分词后匹配，适合搜索 text 类型字段
	query := `{
		"query": {
			"match": {
				"description": "旗舰手机"
			}
		}
	}`

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

	fmt.Println("【2】match — 全文搜索 description 含「旗舰手机」：")
	printResults(result)
}

// ----------------------------------------
// 3. 精确匹配 term（不分词）
// ----------------------------------------
func termSearch(es *elasticsearch.Client, ctx context.Context) {
	// term 用于 keyword 类型的精确匹配，大小写敏感
	query := `{
		"query": {
			"term": {
				"brand": "Apple"
			}
		}
	}`

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

	fmt.Println("【3】term — 精确匹配 brand = Apple：")
	printResults(result)
}

// ----------------------------------------
// 4. 范围查询 range
// ----------------------------------------
func rangeSearch(es *elasticsearch.Client, ctx context.Context) {
	// 查询价格在 4000 ~ 7000 之间的商品
	query := `{
		"query": {
			"range": {
				"price": {
					"gte": 4000,
					"lte": 7000
				}
			}
		},
		"sort": [{ "price": "asc" }]
	}`

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

	fmt.Println("【4】range — 价格 4000 ≤ price ≤ 7000（按价格升序）：")
	printResults(result)
}

// ----------------------------------------
// 5. 多字段搜索 multi_match
// ----------------------------------------
func multiMatchSearch(es *elasticsearch.Client, ctx context.Context) {
	// 同时在 name 和 description 两个字段里搜索 "旗舰"
	query := `{
		"query": {
			"multi_match": {
				"query":  "旗舰",
				"fields": ["name", "description"]
			}
		}
	}`

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

	fmt.Println("【5】multi_match — 在 name/description 中搜索「旗舰」：")
	printResults(result)
}
