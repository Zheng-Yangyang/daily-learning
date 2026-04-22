# 14 - 分片与副本

## 功能描述

演示 ES 分片机制：创建 3 主分片 + 1 副本的索引，观察分片在节点上的分布，验证文档路由规则（hash(id) % 主分片数），自定义路由（routing）。

## 创建带分片配置的索引

```go
settings := `{
    "settings": {
        "number_of_shards":   3,  // 主分片数，创建后不能修改！
        "number_of_replicas": 1   // 副本数，可随时修改
    }
}`
// 3 主分片 + 3 副本 = 6 个分片总计
```

## 文档路由原理

```
路由公式：shard = hash(document_id) % number_of_shards

同一个 ID 每次计算结果相同 → 总是路由到同一个主分片
```

## 自定义路由（减少跨分片查询）

```go
// 按品牌路由，同品牌数据写入同一分片
es.Index(indexName, body,
    es.Index.WithRouting("Apple"),  // 指定 routing key
)
// 查询时也用同样的 routing，ES 只查一个分片而不是全部分片
```

## 查看分片分布

```go
es.Cat.Shards(
    es.Cat.Shards.WithIndex(indexName),
    es.Cat.Shards.WithH("shard", "prirep", "state", "docs", "node"),
    es.Cat.Shards.WithFormat("json"),
)
// prirep: p = 主分片, r = 副本
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| 主分片 | 数据写入目标，数量创建后不可改（重要！） |
| 副本分片 | 主分片的完整拷贝，可提高读取性能和容灾 |
| 写入流程 | 只写主分片 → ES 自动同步到副本 |
| 读取流程 | 主分片和副本都可响应查询（负载均衡） |
| `number_of_replicas: 0` | 单节点时设为 0，否则 yellow 状态（副本无法在同节点分配） |
