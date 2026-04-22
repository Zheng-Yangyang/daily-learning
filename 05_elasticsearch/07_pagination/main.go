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
	Name  string  `json:"name"`
	Brand string  `json:"brand"`
	Price float64 `json:"price"`
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

	// 1. from/size 基础分页
	fromSizePagination(es, ctx)

	// 2. from/size 深度分页问题演示
	fromSizeDeepProblem(es, ctx)

	// 3. search_after 游标分页（推荐）
	searchAfterPagination(es, ctx)
}

// ----------------------------------------
// 1. from/size — 基础分页
// ----------------------------------------
func fromSizePagination(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【1】from/size 基础分页：")

	for page := 0; page < 3; page++ {
		from := page * 5

		query := fmt.Sprintf(`{
			"query": { "match_all": {} },
			"sort":  [ { "price": "asc" }, { "name": "asc" } ],
			"from": %d,
			"size": 5
		}`, from)

		result := doSearch(es, ctx, query)
		hits := result["hits"].(map[string]interface{})
		total := hits["total"].(map[string]interface{})["value"].(float64)
		hitList := hits["hits"].([]interface{})

		fmt.Printf("\n   第 %d 页（from=%d, size=5），总数据量 %.0f 条：\n", page+1, from, total)
		for _, hit := range hitList {
			h := hit.(map[string]interface{})
			source := h["_source"].(map[string]interface{})
			fmt.Printf("   - %-25s | %-10s | ¥%.0f\n",
				source["name"], source["brand"], source["price"])
		}
	}
	fmt.Println()
}

// ----------------------------------------
// 2. from/size 深度分页的问题
// ----------------------------------------
func fromSizeDeepProblem(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【2】from/size 深度分页问题演示：")

	query := `{
		"query": { "match_all": {} },
		"from": 9999,
		"size": 1
	}`

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(indexName),
		es.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil {
		log.Fatalf("搜索失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		fmt.Println("   ❌ 报错了！ES 返回：")
		fmt.Println("   " + res.String())
		fmt.Println()
		fmt.Println("   原因：from + size 超过 max_result_window(10000) 限制")
		fmt.Println("   ES 需要在每个分片上取 from+size 条数据再合并排序")
		fmt.Println("   深度分页时内存和CPU开销极大，所以 ES 直接禁止")
		fmt.Println()
		return
	}

	fmt.Println("   ⚠️  数据量较少未触发限制，实际生产环境数据超过10000条时会报错")
	fmt.Println()
}

// ----------------------------------------
// 3. search_after — 游标分页
// ----------------------------------------
func searchAfterPagination(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【3】search_after 游标分页：")

	var searchAfter []interface{}

	for page := 1; page <= 3; page++ {
		var query string

		if searchAfter == nil {
			query = `{
				"query": { "match_all": {} },
				"sort":  [ { "price": "asc" }, { "name": "asc" } ],
				"size":  5
			}`
		} else {
			cursorJSON, _ := json.Marshal(searchAfter)
			query = fmt.Sprintf(`{
				"query":        { "match_all": {} },
				"sort":         [ { "price": "asc" }, { "name": "asc" } ],
				"size":         5,
				"search_after": %s
			}`, string(cursorJSON))
		}

		result := doSearch(es, ctx, query)
		hits := result["hits"].(map[string]interface{})
		hitList := hits["hits"].([]interface{})

		if len(hitList) == 0 {
			fmt.Println("   没有更多数据了")
			break
		}

		fmt.Printf("\n   第 %d 页：\n", page)
		for _, hit := range hitList {
			h := hit.(map[string]interface{})
			source := h["_source"].(map[string]interface{})
			searchAfter = h["sort"].([]interface{})
			fmt.Printf("   - %-25s | %-10s | ¥%.0f | 游标: %v\n",
				source["name"], source["brand"], source["price"], searchAfter)
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

	// name 改为 keyword 才能参与排序
	mapping := `{
		"mappings": {
			"properties": {
				"name":  { "type": "keyword" },
				"brand": { "type": "keyword" },
				"price": { "type": "float" }
			}
		}
	}`
	res, _ := es.Indices.Create(indexName,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()

	products := []Product{
		{Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999},
		{Name: "iPhone 15", Brand: "Apple", Price: 5999},
		{Name: "iPhone 14", Brand: "Apple", Price: 4999},
		{Name: "iPhone 14 Pro", Brand: "Apple", Price: 6999},
		{Name: "MacBook Pro 14", Brand: "Apple", Price: 14999},
		{Name: "Galaxy S24 Ultra", Brand: "Samsung", Price: 9999},
		{Name: "Galaxy S24", Brand: "Samsung", Price: 6999},
		{Name: "Galaxy S23", Brand: "Samsung", Price: 4999},
		{Name: "Galaxy A54", Brand: "Samsung", Price: 2999},
		{Name: "Galaxy Tab S9", Brand: "Samsung", Price: 7999},
		{Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999},
		{Name: "小米14", Brand: "Xiaomi", Price: 3999},
		{Name: "小米13", Brand: "Xiaomi", Price: 3499},
		{Name: "红米Note13", Brand: "Xiaomi", Price: 1999},
		{Name: "小米平板6", Brand: "Xiaomi", Price: 2999},
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
