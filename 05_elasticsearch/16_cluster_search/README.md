# 16 - 集群分布式搜索

## 功能描述

深入演示分布式搜索的两阶段原理（Query + Fetch），验证路由规则，搜索偏好（preference）控制，以及多节点负载均衡验证。

## 两阶段搜索原理

```
Query 阶段（Scatter-Gather）：
  1. 客户端 → 任意节点（协调节点）
  2. 协调节点 → 广播到所有主/副本分片
  3. 每个分片本地搜索 → 返回 [文档ID + _score]
  4. 协调节点合并所有分片结果 → 全局排序 → 取 Top N 个 ID

Fetch 阶段：
  5. 协调节点 → 根据 ID 去对应分片拉取完整文档内容
  6. 汇总返回客户端
```

## 自定义路由（减少分片数）

```go
// 写入时指定 routing，同品牌数据在同一分片
es.Index(indexName, body, es.Index.WithRouting("Apple"))

// 查询时也带 routing，只查一个分片（而非全部3个）
es.Search(es.Search.WithRouting("Apple"))
```

## 搜索偏好（preference）

```go
// 默认：随机选择主分片或副本（负载均衡）
es.Search(es.Search.WithIndex(indexName), ...)

// 本地优先：查询本节点的分片（减少网络跳转）
es.Search(es.Search.WithPreference("_local"), ...)

// 会话一致性：同一个 preference 值总是查同一组分片
es.Search(es.Search.WithPreference("user_123"), ...)
```

## 负载均衡验证

```go
// 直接请求不同节点，每个节点都能作为协调节点
for _, port := range []string{"9200", "9201", "9202"} {
    client, _ := elasticsearch.NewClient(elasticsearch.Config{
        Addresses: []string{"http://localhost:" + port},
    })
    res, _ := client.Search(client.Search.WithIndex(indexName), ...)
    // 三个节点都能返回完整结果
}
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| 协调节点 | 任意节点都可作为协调节点，接收请求并汇总结果 |
| `WithExplain(true)` | 返回评分计算详情，用于调试 |
| `_local` preference | 减少网络跳转，但可能导致某节点压力大 |
| 自定义 routing | 把相关数据路由到同一分片，查询时指定 routing 可跳过不相关分片 |
| 深度分页问题 | from/size 在分布式环境下更严重（每个分片都要取 from+size 条） |
