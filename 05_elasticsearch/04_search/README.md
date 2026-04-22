# 04 - 基础搜索

## 功能描述

演示五种常用搜索方式：全量查询、全文搜索（match）、精确匹配（term）、范围查询（range）、多字段搜索（multi_match）。

## 五种搜索对比

```go
// 1. match_all — 查询所有文档
{"query": {"match_all": {}}}

// 2. match — 全文搜索，会分词后匹配（适合 text 字段）
{"query": {"match": {"description": "旗舰手机"}}}

// 3. term — 精确匹配，不分词（适合 keyword 字段）
{"query": {"term": {"brand": "Apple"}}}

// 4. range — 范围查询 + 排序
{"query": {"range": {"price": {"gte": 4000, "lte": 7000}}},
 "sort": [{"price": "asc"}]}

// 5. multi_match — 同时在多个字段搜索
{"query": {"multi_match": {"query": "旗舰", "fields": ["name", "description"]}}}
```

## match vs term 核心区别

| 查询类型 | 分词 | 适用字段 | 场景 |
|---------|------|---------|------|
| `match` | ✅ 是 | text | 全文搜索、用户输入 |
| `term` | ❌ 否 | keyword | 品牌、状态、枚举值精确匹配 |

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `hits.total.value` | 命中总数 |
| `hits.hits[*]._source` | 文档原始内容 |
| `size` 参数 | 返回文档数量，默认 10 |
| 索引刷新 | 批量写入后需 `Indices.Refresh` 才能立即搜索到 |
