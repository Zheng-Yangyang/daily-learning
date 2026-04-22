# 10 - Reindex 与别名切换

## 功能描述

演示 ES 的 Reindex 工作流：从旧索引迁移数据到新索引（改 Mapping + 加新字段 + Painless 脚本转换），通过别名实现零停机切换。

## Reindex 使用场景

- 修改字段类型（如 text → keyword）
- 更换分词器（如 standard → ik_smart）
- 新增字段并根据现有数据自动填充

## 核心流程

```go
// 1. 创建新索引（新 Mapping：ik_smart + 新增 category 字段）
es.Indices.Create(newIndex, es.Indices.Create.WithBody(...))

// 2. 执行 Reindex（带 Painless 脚本动态设置 category）
reindexBody := `{
    "source": {"index": "products_v1"},
    "dest":   {"index": "products_v2"},
    "script": {
        "lang": "painless",
        "source": "if (ctx._source.brand == 'Apple') { ctx._source.category = 'apple_eco'; }"
    }
}`
es.Reindex(strings.NewReader(reindexBody))

// 3. 别名原子切换（零停机）
aliasBody := `{
    "actions": [
        {"remove": {"index": "products_v1", "alias": "products"}},
        {"add":    {"index": "products_v2", "alias": "products"}}
    ]
}`
es.Indices.UpdateAliases(strings.NewReader(aliasBody))
```

## 零停机切换原理

```
应用层始终访问别名 "products"
                 ↓
旧索引 products_v1 ──→ 切换 ──→ 新索引 products_v2
（别名在一次原子操作中完成切换，无停机时间）
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| Reindex | 数据迁移，支持 query 过滤 + script 转换 |
| Painless | ES 内置脚本语言，用于文档转换 |
| 索引别名 | 应用访问别名而非真实索引，解耦版本迭代 |
| 原子切换 | remove + add 在同一个 UpdateAliases 请求中，切换瞬间完成 |
