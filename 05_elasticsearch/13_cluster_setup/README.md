# 13 - 集群搭建与基础信息

## 功能描述

连接三节点 ES 集群，查询集群基本信息、节点详情（角色/内存/CPU），以及集群健康状态的详细含义。

## 多节点客户端配置

```go
// go-elasticsearch 会自动在多个节点间负载均衡
cfg := elasticsearch.Config{
    Addresses: []string{
        "http://localhost:9200",  // es01 (master)
        "http://localhost:9201",  // es02
        "http://localhost:9202",  // es03
    },
}
```

## 节点信息查询

```go
// CAT API — 人类可读的节点列表（JSON 格式方便程序解析）
es.Cat.Nodes(
    es.Cat.Nodes.WithH("name", "ip", "heap.percent", "ram.percent", "cpu", "master", "node.role"),
    es.Cat.Nodes.WithFormat("json"),
)
```

## 集群健康状态字段

```go
health["number_of_nodes"]        // 节点总数
health["number_of_data_nodes"]   // 数据节点数
health["active_primary_shards"]  // 主分片数
health["active_shards"]          // 总分片数（主 + 副本）
health["unassigned_shards"]      // 未分配分片（yellow/red 时不为 0）
health["relocating_shards"]      // 正在迁移的分片
```

## Docker 三节点集群搭建（docker-compose.yml）

```yaml
services:
  es01:
    image: elasticsearch:8.x
    ports: ["9200:9200"]
    environment:
      - cluster.name=es-cluster
      - node.name=es01
      - discovery.seed_hosts=es02,es03
      - cluster.initial_master_nodes=es01
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| Master 节点 | 负责集群元数据管理（索引创建、分片分配），`master=*` 标识 |
| Data 节点 | 存储数据、执行搜索，资源开销大 |
| CAT API | 人类友好的集群监控接口，支持 JSON 输出 |
| 健康状态 | green/yellow/red 直观反映集群可用性 |
