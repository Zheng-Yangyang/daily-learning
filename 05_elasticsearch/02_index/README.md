# 02 - 索引管理（创建 / 查看 / 删除）

## 功能描述

演示 ES 索引的生命周期管理：创建带 Mapping 的索引、查看索引结构、删除索引。

## Mapping 字段类型

```go
mapping := `{
    "mappings": {
        "properties": {
            "name":        { "type": "text" },      // 全文检索，会分词
            "brand":       { "type": "keyword" },   // 精确匹配，不分词
            "price":       { "type": "float" },
            "stock":       { "type": "integer" },
            "description": { "type": "text" },
            "created_at":  { "type": "date" }
        }
    }
}`
```

## 核心操作

```go
// 创建索引
es.Indices.Create(indexName,
    es.Indices.Create.WithBody(strings.NewReader(mapping)))

// 查看索引 Mapping
es.Indices.Get([]string{indexName})

// 删除索引
es.Indices.Delete([]string{indexName})
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `text` vs `keyword` | text 分词可全文搜索；keyword 不分词用于精确匹配、聚合、排序 |
| Mapping 设计 | 类似数据库 Schema，决定字段如何被索引和存储 |
| 动态 Mapping | 不指定 Mapping 时 ES 自动推断类型，生产环境建议显式定义 |
| `json.NewDecoder` | 解析 ES 返回的 JSON 响应体 |
