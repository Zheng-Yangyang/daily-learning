# 01 - ES 连接与健康检查

## 功能描述

使用 go-elasticsearch v8 客户端连接 Elasticsearch，执行健康检查，查询集群健康状态（green/yellow/red）。

## 核心代码

```go
// 创建客户端
cfg := elasticsearch.Config{
    Addresses: []string{"http://localhost:9200"},
}
es, _ := elasticsearch.NewClient(cfg)

// 连接验证（INFO 接口）
res, _ := es.Info()
if res.IsError() { log.Fatal(...) }

// 查询集群健康状态
healthRes, _ := es.Cluster.Health(
    es.Cluster.Health.WithContext(ctx),
    es.Cluster.Health.WithPretty(),
)
```

## 集群状态说明

| 状态 | 含义 |
|------|------|
| 🟢 green | 所有主分片和副本分片都正常 |
| 🟡 yellow | 主分片正常，部分副本未分配（单节点常见） |
| 🔴 red | 部分主分片不可用，有数据丢失风险 |

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `elasticsearch.Config` | 配置客户端：地址、认证、TLS、超时等 |
| `es.Info()` | 等效于 GET / — 获取节点基本信息 |
| `es.Cluster.Health` | GET /_cluster/health — 集群健康概览 |
| `res.IsError()` | 响应状态码非 2xx 时返回 true |
| `defer res.Body.Close()` | ES 响应 Body 必须手动关闭，否则连接泄漏 |
