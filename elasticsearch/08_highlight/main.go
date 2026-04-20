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

	// 1. 基础高亮
	basicHighlight(es, ctx)

	// 2. 自定义高亮标签
	customTagHighlight(es, ctx)

	// 3. 多字段高亮
	multiFieldHighlight(es, ctx)
}

// ----------------------------------------
// 1. 基础高亮 — 默认用 <em> 标签包裹
// ----------------------------------------
func basicHighlight(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"match": { "description": "旗舰" }
		},
		"highlight": {
			"fields": {
				"description": {}
			}
		}
	}`

	result := doSearch(es, ctx, query)
	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})

	fmt.Println("【1】基础高亮（默认 <em> 标签）：")
	for _, hit := range hits {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		name := source["name"]

		// 高亮结果在 highlight 字段里，不在 _source 里
		highlight := h["highlight"].(map[string]interface{})
		descHighlight := highlight["description"].([]interface{})

		fmt.Printf("\n   商品：%s\n", name)
		fmt.Printf("   高亮：%s\n", descHighlight[0])
	}
	fmt.Println()
}

// ----------------------------------------
// 2. 自定义高亮标签
// ----------------------------------------
func customTagHighlight(es *elasticsearch.Client, ctx context.Context) {
	// 实际前端开发中，用自定义标签方便 CSS 样式控制
	query := `{
		"query": {
			"match": { "description": "旗舰" }
		},
		"highlight": {
			"pre_tags":  ["<span class='highlight'>"],
			"post_tags": ["</span>"],
			"fields": {
				"description": {}
			}
		}
	}`

	result := doSearch(es, ctx, query)
	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})

	fmt.Println("【2】自定义高亮标签（<span class='highlight'>）：")
	for _, hit := range hits {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		name := source["name"]

		highlight := h["highlight"].(map[string]interface{})
		descHighlight := highlight["description"].([]interface{})

		fmt.Printf("\n   商品：%s\n", name)
		fmt.Printf("   高亮：%s\n", descHighlight[0])
	}
	fmt.Println()
}

// ----------------------------------------
// 3. 多字段高亮
// ----------------------------------------
func multiFieldHighlight(es *elasticsearch.Client, ctx context.Context) {
	query := `{
		"query": {
			"multi_match": {
				"query":  "手机",
				"fields": ["name", "description"]
			}
		},
		"highlight": {
			"pre_tags":  [">>>"],
			"post_tags": ["<<<"],
			"fields": {
				"name":        { "number_of_fragments": 0 },
				"description": { "fragment_size": 50, "number_of_fragments": 1 }
			}
		}
	}`
	// number_of_fragments: 0  → 返回整个字段内容（不截断），适合短字段如 name
	// fragment_size: 50       → 每个高亮片段最多 50 个字符，适合长字段如 description
	// number_of_fragments: 1  → 最多返回 1 个高亮片段

	result := doSearch(es, ctx, query)
	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})

	fmt.Println("【3】多字段高亮（name + description）：")
	for _, hit := range hits {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		fmt.Printf("\n   商品：%s\n", source["name"])

		highlight, ok := h["highlight"].(map[string]interface{})
		if !ok {
			continue
		}

		// name 字段高亮（不一定每条都有）
		if nameHL, ok := highlight["name"].([]interface{}); ok {
			fmt.Printf("   名称高亮：%s\n", nameHL[0])
		}

		// description 字段高亮
		if descHL, ok := highlight["description"].([]interface{}); ok {
			fmt.Printf("   描述高亮：%s\n", descHL[0])
		}
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
				"description": { "type": "text" }
			}
		}
	}`
	res, _ := es.Indices.Create(indexName,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()

	products := []Product{
		{Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999,
			Description: "苹果旗舰手机，搭载A17 Pro芯片，钛金属边框，支持USB-C"},
		{Name: "Galaxy S24 Ultra", Brand: "Samsung", Price: 9999,
			Description: "三星旗舰手机，骁龙8Gen3处理器，内置S Pen手写笔"},
		{Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999,
			Description: "小米旗舰手机，骁龙8Gen3，徕卡专业影像，澎湃OS"},
		{Name: "MacBook Pro 14", Brand: "Apple", Price: 14999,
			Description: "苹果旗舰笔记本，M3 Pro芯片，专业级性能"},
		{Name: "iPhone 15", Brand: "Apple", Price: 5999,
			Description: "苹果手机，A16芯片，铝合金边框，支持USB-C"},
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
