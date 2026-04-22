# 09 - IK 中文分词器

## 功能描述

对比 standard、ik_smart、ik_max_word 三种分词器的效果，演示 fields 多字段映射（同一字段建两个索引），验证 IK 分词在中文搜索场景下的优势。

## 三种分词器对比

```
原文：「苹果旗舰手机发布新品」

standard  → [苹, 果, 旗, 舰, 手, 机, 发, 布, 新, 品]  ← 单字切分，中文效果差
ik_smart  → [苹果, 旗舰, 手机, 发布, 新品]              ← 最粗粒度，优先长词
ik_max_word → [苹果, 旗舰, 手机, 发布, 新品, 品]        ← 最细粒度，穷举组合
```

## fields 多字段映射（核心技巧）

```go
// 同一个 description 字段建两个索引，满足不同搜索需求
mapping := `{
    "properties": {
        "description": {
            "type":     "text",
            "analyzer": "standard",    // description 字段用 standard
            "fields": {
                "ik": {
                    "type":     "text",
                    "analyzer": "ik_smart"  // description.ik 用 IK 分词
                }
            }
        }
    }
}`

// 搜索时指定具体子字段
{"query": {"match": {"description.ik": "旗舰手机"}}}
```

## 分词 API 直接调用

```go
query := `{"analyzer": "ik_smart", "text": "苹果旗舰手机发布新品"}`
es.Indices.Analyze(es.Indices.Analyze.WithBody(strings.NewReader(query)))
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `ik_smart` | 最粗粒度，减少冗余索引，推荐搜索时使用 |
| `ik_max_word` | 最细粒度，穷举所有可能词，适合索引时使用 |
| `fields` 多字段 | 一个字段同时建多种索引，不同查询场景走不同索引 |
| Analyze API | 直接测试分词效果，调试分词配置的利器 |
