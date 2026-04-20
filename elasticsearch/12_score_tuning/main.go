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
	Sales       int     `json:"sales"`
	Rating      float64 `json:"rating"`
	Description string  `json:"description"`
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

	// 1. 默认评分（BM25）
	defaultScore(es, ctx)

	// 2. boost 字段权重调分
	boostScore(es, ctx)

	// 3. function_score 销量加权
	functionScoreSales(es, ctx)

	// 4. function_score 综合评分（销量+评分+新品）
	functionScoreCombined(es, ctx)
}

// ----------------------------------------
// 1. 默认 BM25 评分
// ----------------------------------------
func defaultScore(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"match": {
				"description": "旗舰手机"
			}
		}
	}`

	result := doSearch(es, ctx, query)
	fmt.Println("【1】默认 BM25 评分搜索「旗舰手机」：")
	printResults(result)
}

// ----------------------------------------
// 2. boost — 提升某个字段的权重
// ----------------------------------------
func boostScore(es *elasticsearch.Client, ctx context.Context) {
	// name 字段命中的权重是 description 的 3 倍
	query := `{
		"query": {
			"multi_match": {
				"query": "旗舰手机",
				"fields": ["name^3", "description"]
			}
		}
	}`

	result := doSearch(es, ctx, query)
	fmt.Println("【2】boost 权重调分（name^3 权重是 description 的3倍）：")
	printResults(result)
}

// ----------------------------------------
// 3. function_score — 按销量加权
// ----------------------------------------
func functionScoreSales(es *elasticsearch.Client, ctx context.Context) {
	// field_value_factor: 用某个字段的值参与评分计算
	// score = BM25分 * log(1 + sales)
	query := `{
		"query": {
			"function_score": {
				"query": {
					"match": { "description": "手机" }
				},
				"field_value_factor": {
					"field":    "sales",
					"modifier": "log1p",
					"factor":   1.0
				},
				"boost_mode": "multiply"
			}
		}
	}`

	result := doSearch(es, ctx, query)
	fmt.Println("【3】function_score 销量加权（score = BM25 * log(1+sales)）：")
	printResults(result)
}

// ----------------------------------------
//  4. function_score 综合评分
//     结合：相关性 + 销量 + 用户评分 + 新品加权
//
// ----------------------------------------
func functionScoreCombined(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"function_score": {
				"query": {
					"match": { "description": "手机" }
				},
				"functions": [
					{
						"field_value_factor": {
							"field":    "sales",
							"modifier": "log1p",
							"factor":   0.5
						}
					},
					{
						"field_value_factor": {
							"field":    "rating",
							"modifier": "none",
							"factor":   2.0
						}
					},
					{
						"filter": { "range": { "sales": { "gte": 10000 } } },
						"weight": 3
					}
				],
				"score_mode": "sum",
				"boost_mode": "replace"
			}
		},
		"sort": [
			{ "_score": "desc" }
		]
	}`
	// score_mode:  functions 之间如何合并 → sum/avg/max/multiply
	// boost_mode: function分 和 query分 如何合并 → replace/multiply/sum

	result := doSearch(es, ctx, query)
	fmt.Println("【4】综合评分（销量 + 用户评分 + 爆款加权）：")
	printResultsWithDetail(result)
}

// ----------------------------------------
// 通用打印
// ----------------------------------------
func printResults(result map[string]interface{}) {
	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})
	for _, hit := range hits {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		score := h["_score"].(float64)
		fmt.Printf("   score=%-8.4f | %-20s | 销量:%-6.0f | 评分:%.1f\n",
			score,
			source["name"],
			source["sales"],
			source["rating"],
		)
	}
	fmt.Println()
}

func printResultsWithDetail(result map[string]interface{}) {
	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})
	for i, hit := range hits {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		score := h["_score"].(float64)
		fmt.Printf("   #%d score=%-8.4f | %-20s | 销量:%-6.0f | 评分:%.1f\n",
			i+1,
			score,
			source["name"],
			source["sales"],
			source["rating"],
		)
	}
	fmt.Println()
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
				"sales":       { "type": "integer" },
				"rating":      { "type": "float" },
				"description": { "type": "text" }
			}
		}
	}`
	res, _ := es.Indices.Create(indexName,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()

	products := []Product{
		{
			Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999,
			Stock: 50, Sales: 15000, Rating: 4.8,
			Description: "苹果旗舰手机 A17芯片 钛金属边框",
		},
		{
			Name: "Galaxy S24 Ultra", Brand: "Samsung", Price: 9999,
			Stock: 60, Sales: 8000, Rating: 4.7,
			Description: "三星旗舰手机 骁龙8Gen3 S Pen手写笔",
		},
		{
			Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999,
			Stock: 200, Sales: 32000, Rating: 4.6,
			Description: "小米旗舰手机 骁龙8Gen3 徕卡影像",
		},
		{
			Name: "iPhone 15", Brand: "Apple", Price: 5999,
			Stock: 120, Sales: 28000, Rating: 4.7,
			Description: "苹果手机 A16芯片 铝合金边框",
		},
		{
			Name: "红米Note13", Brand: "Xiaomi", Price: 1999,
			Stock: 500, Sales: 55000, Rating: 4.5,
			Description: "红米手机 高性价比 大电池续航",
		},
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
