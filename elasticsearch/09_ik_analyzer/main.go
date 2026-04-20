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

	// 1. 对比 standard、ik_smart、ik_max_word 三种分词效果
	compareAnalyzers(es, ctx)

	// 2. 用 IK 创建索引并搜索
	setupTestData(es, ctx)
	fmt.Println("========================================")

	// 3. standard 分词器搜索（对照组）
	standardSearch(es, ctx)

	// 4. IK 分词器搜索（效果对比）
	ikSearch(es, ctx)

	// 5. 带高亮的 IK 搜索
	ikHighlightSearch(es, ctx)
}

// ----------------------------------------
// 1. 直接调用 analyze API 对比分词效果
// ----------------------------------------
func compareAnalyzers(es *elasticsearch.Client, ctx context.Context) {
	text := "苹果旗舰手机发布新品"

	analyzers := []string{"standard", "ik_smart", "ik_max_word"}

	fmt.Printf("原始文本：「%s」\n\n", text)
	fmt.Println("【1】三种分词器对比：")

	for _, analyzer := range analyzers {
		query := fmt.Sprintf(`{"analyzer": "%s", "text": "%s"}`, analyzer, text)

		res, err := es.Indices.Analyze(
			es.Indices.Analyze.WithContext(ctx),
			es.Indices.Analyze.WithBody(strings.NewReader(query)),
		)
		if err != nil || res.IsError() {
			log.Fatalf("分词失败: %s", res.String())
		}

		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		tokens := result["tokens"].([]interface{})
		words := make([]string, 0, len(tokens))
		for _, t := range tokens {
			token := t.(map[string]interface{})
			words = append(words, token["token"].(string))
		}

		fmt.Printf("   %-12s → %v\n", analyzer, words)
	}
	fmt.Println()
}

// ----------------------------------------
//  2. 创建索引：同一字段用两种分词器
//     fields 多字段映射，面试高频考点
//
// ----------------------------------------
func setupTestData(es *elasticsearch.Client, ctx context.Context) {
	es.Indices.Delete([]string{indexName}, es.Indices.Delete.WithContext(ctx))

	// 核心：用 fields 给同一个字段建两个索引
	// description         → standard 分词（对照）
	// description.ik      → ik_smart 分词（推荐）
	mapping := `{
		"mappings": {
			"properties": {
				"name":  { "type": "keyword" },
				"brand": { "type": "keyword" },
				"price": { "type": "float" },
				"description": {
					"type":     "text",
					"analyzer": "standard",
					"fields": {
						"ik": {
							"type":     "text",
							"analyzer": "ik_smart"
						}
					}
				}
			}
		}
	}`

	res, _ := es.Indices.Create(indexName,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()

	products := []Product{
		{Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999,
			Description: "苹果旗舰手机，搭载A17 Pro芯片，钛金属边框，支持USB-C快充"},
		{Name: "Galaxy S24 Ultra", Brand: "Samsung", Price: 9999,
			Description: "三星旗舰手机，骁龙8Gen3处理器，内置S Pen手写笔，超长续航"},
		{Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999,
			Description: "小米旗舰手机，骁龙8Gen3，徕卡专业影像系统，澎湃OS"},
		{Name: "MacBook Pro 14", Brand: "Apple", Price: 14999,
			Description: "苹果旗舰笔记本，M3 Pro芯片，专业级性能，长续航"},
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

// ----------------------------------------
// 3. standard 分词器搜索（对照组）
// ----------------------------------------
func standardSearch(es *elasticsearch.Client, ctx context.Context) {
	// 搜 description 字段（standard 分词）
	query := `{
		"query": {
			"match": { "description": "旗舰手机" }
		},
		"highlight": {
			"pre_tags":  ["<em>"],
			"post_tags": ["</em>"],
			"fields": { "description": {} }
		}
	}`

	result := doSearch(es, ctx, query)
	hits := result["hits"].(map[string]interface{})
	total := hits["total"].(map[string]interface{})["value"].(float64)
	hitList := hits["hits"].([]interface{})

	fmt.Printf("\n【3】standard 分词搜索「旗舰手机」，命中 %.0f 条：\n", total)
	for _, hit := range hitList {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		highlight := h["highlight"].(map[string]interface{})
		descHL := highlight["description"].([]interface{})
		fmt.Printf("   - %s\n     高亮：%s\n", source["name"], descHL[0])
	}
	fmt.Println()
}

// ----------------------------------------
// 4. IK 分词器搜索
// ----------------------------------------
func ikSearch(es *elasticsearch.Client, ctx context.Context) {
	// 搜 description.ik 字段（ik_smart 分词）
	query := `{
		"query": {
			"match": { "description.ik": "旗舰手机" }
		},
		"highlight": {
			"pre_tags":  ["<em>"],
			"post_tags": ["</em>"],
			"fields": { "description.ik": {} }
		}
	}`

	result := doSearch(es, ctx, query)
	hits := result["hits"].(map[string]interface{})
	total := hits["total"].(map[string]interface{})["value"].(float64)
	hitList := hits["hits"].([]interface{})

	fmt.Printf("【4】ik_smart 分词搜索「旗舰手机」，命中 %.0f 条：\n", total)
	for _, hit := range hitList {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		highlight := h["highlight"].(map[string]interface{})
		descHL := highlight["description.ik"].([]interface{})
		fmt.Printf("   - %s\n     高亮：%s\n", source["name"], descHL[0])
	}
	fmt.Println()
}

// ----------------------------------------
// 5. IK 搜索 + 高亮，搜单个词看分词精准度
// ----------------------------------------
func ikHighlightSearch(es *elasticsearch.Client, ctx context.Context) {
	keywords := []string{"旗舰", "续航", "影像"}

	fmt.Println("【5】IK 精准词搜索对比：")
	for _, kw := range keywords {
		query := fmt.Sprintf(`{
			"query": { "match": { "description.ik": "%s" } },
			"_source": ["name"]
		}`, kw)

		result := doSearch(es, ctx, query)
		hits := result["hits"].(map[string]interface{})
		total := hits["total"].(map[string]interface{})["value"].(float64)
		hitList := hits["hits"].([]interface{})

		names := make([]string, 0)
		for _, hit := range hitList {
			h := hit.(map[string]interface{})
			source := h["_source"].(map[string]interface{})
			names = append(names, source["name"].(string))
		}
		fmt.Printf("   搜「%-4s」命中 %.0f 条：%v\n", kw, total, names)
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
