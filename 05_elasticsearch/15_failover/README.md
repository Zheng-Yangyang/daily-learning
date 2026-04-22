# 15 - 故障转移与自愈

## 功能描述

交互式演示 ES 集群的故障转移能力：停掉 es03 节点后集群状态变化、验证读写仍然可用、等待集群自愈（副本提升为主分片），恢复节点后观察重新平衡。

## 故障转移流程

```
正常状态（3节点，green）：
  es01: 主分片0 + 副本1
  es02: 主分片1 + 副本2
  es03: 主分片2 + 副本0

停掉 es03 后（2节点，yellow）：
  es03 的主分片2 不可用
  → ES 自动将 es01 或 es02 上的副本2 提升为主分片
  → 集群变为 yellow（主分片全部正常，副本不完整）
  → 读写仍然正常！
```

## 核心代码：等待自愈

```go
func waitForRecovery(es *elasticsearch.Client, ctx context.Context) {
    for i := 0; i < 10; i++ {
        time.Sleep(2 * time.Second)
        health := getClusterHealth(es, ctx)
        if health["status"] == "yellow" && health["unassigned_shards"] == 0 {
            fmt.Println("🟡 集群已自愈（主分片全部正常）")
            return
        }
    }
}
```

## 验证故障后仍可读写

```go
// 写入测试
res, _ := es.Index(indexName, strings.NewReader(`{"name":"故障测试"}`),
    es.Index.WithRefresh("true"))
// ✅ 写入成功！节点故障不影响写入

// 查询测试
res2, _ := es.Search(es.Search.WithIndex(indexName), ...)
// ✅ 查询成功！
```

## 状态变化时间线

```
docker stop es03   → 集群状态 red（短暂）→ yellow（副本提升完成）
docker start es03  → 等待 5s → 副本重新分配 → green（完全恢复）
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| 副本提升 | 主分片故障后，对应副本自动提升为新主分片 |
| yellow 状态 | 主分片全部正常，但副本不完整（不影响读写） |
| 自愈时间 | 默认等待 1 分钟再进行分片重分配（防止节点短暂波动触发不必要迁移） |
| `unassigned_shards` | 此值为 0 表示所有分片都已分配完毕 |
