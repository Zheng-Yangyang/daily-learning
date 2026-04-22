// 案例04：Redis Set —— 去重 & 集合运算
// 知识点：SADD / SREM / SMEMBERS / SISMEMBER / SCARD
//
//	SINTER / SUNION / SDIFF（交集 / 并集 / 差集）
//	SRANDMEMBER / SPOP（随机抽取）
//
// Set 是无序、不重复的集合
// 典型场景：
//
//	① UV 统计（用户去重）
//	② 标签系统（用户兴趣标签）
//	③ 共同好友（SINTER）
//	④ 抽奖（SRANDMEMBER / SPOP）
//	⑤ 黑名单 / 白名单
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: SADD / SMEMBERS / SCARD / SISMEMBER ===")
	case1_basic(ctx, rdb)

	fmt.Println("\n=== case2: UV 去重统计 ===")
	case2_uv(ctx, rdb)

	fmt.Println("\n=== case3: SINTER 共同好友 ===")
	case3_commonFriends(ctx, rdb)

	fmt.Println("\n=== case4: SUNION 并集 / SDIFF 差集 ===")
	case4_unionDiff(ctx, rdb)

	fmt.Println("\n=== case5: SRANDMEMBER 抽奖（不删除）===")
	case5_srandmember(ctx, rdb)

	fmt.Println("\n=== case6: SPOP 抽奖（删除已中奖）===")
	case6_spop(ctx, rdb)

	fmt.Println("\n=== case7: 标签系统 ===")
	case7_tags(ctx, rdb)
}

func case1_basic(ctx context.Context, rdb *redis.Client) {
	key := "demo:set"
	rdb.Del(ctx, key)

	// SADD 添加元素，重复的自动忽略，返回实际新增数量
	n, _ := rdb.SAdd(ctx, key, "a", "b", "c", "a", "b").Result()
	fmt.Println("SADD (5个元素含2个重复) 实际新增:", n) // 3

	// SMEMBERS 返回所有元素（无序）
	members, _ := rdb.SMembers(ctx, key).Result()
	fmt.Println("SMEMBERS →", members)

	// SCARD 集合元素数量
	count, _ := rdb.SCard(ctx, key).Result()
	fmt.Println("SCARD →", count)

	// SISMEMBER 判断元素是否存在
	exists, _ := rdb.SIsMember(ctx, key, "a").Result()
	fmt.Println("SISMEMBER a →", exists) // true

	exists, _ = rdb.SIsMember(ctx, key, "z").Result()
	fmt.Println("SISMEMBER z →", exists) // false

	// SREM 删除元素
	rdb.SRem(ctx, key, "a")
	members, _ = rdb.SMembers(ctx, key).Result()
	fmt.Println("SMEMBERS after SREM a →", members)

	rdb.Del(ctx, key)
}

func case2_uv(ctx context.Context, rdb *redis.Client) {
	// UV（独立访客数）：同一用户当天访问多次只算一次
	// 用 Set 存每天访问的 userID，SCARD 就是 UV
	key := "uv:2024-01-01"
	rdb.Del(ctx, key)

	// 模拟一天内的访问日志（有重复用户）
	visits := []string{"user1", "user2", "user1", "user3", "user2", "user4", "user1"}
	for _, uid := range visits {
		rdb.SAdd(ctx, key, uid)
	}

	uv, _ := rdb.SCard(ctx, key).Result()
	fmt.Printf("总访问次数(PV): %d，独立访客数(UV): %d\n", len(visits), uv)

	rdb.Del(ctx, key)
}

func case3_commonFriends(ctx context.Context, rdb *redis.Client) {
	// 每个用户的关注列表用 Set 存储
	alice := "friends:alice"
	bob := "friends:bob"
	rdb.Del(ctx, alice, bob)

	rdb.SAdd(ctx, alice, "user1", "user2", "user3", "user4")
	rdb.SAdd(ctx, bob, "user2", "user3", "user5", "user6")

	// SINTER 交集 = 共同好友
	common, _ := rdb.SInter(ctx, alice, bob).Result()
	fmt.Println("Alice 的好友:", mustSMembers(ctx, rdb, alice))
	fmt.Println("Bob   的好友:", mustSMembers(ctx, rdb, bob))
	fmt.Println("共同好友(SINTER):", common)

	// SMISMEMBER 批量判断（go-redis v9）
	results, _ := rdb.SMIsMember(ctx, alice, "user2", "user5", "user9").Result()
	fmt.Printf("Alice 是否认识 user2=%v user5=%v user9=%v\n", results[0], results[1], results[2])

	rdb.Del(ctx, alice, bob)
}

func case4_unionDiff(ctx context.Context, rdb *redis.Client) {
	setA := "set:A"
	setB := "set:B"
	rdb.Del(ctx, setA, setB)

	rdb.SAdd(ctx, setA, "1", "2", "3", "4")
	rdb.SAdd(ctx, setB, "3", "4", "5", "6")

	// SUNION 并集：A 和 B 所有元素合并去重
	union, _ := rdb.SUnion(ctx, setA, setB).Result()
	fmt.Println("A:", mustSMembers(ctx, rdb, setA))
	fmt.Println("B:", mustSMembers(ctx, rdb, setB))
	fmt.Println("SUNION (A∪B):", union)

	// SDIFF 差集：在 A 中但不在 B 中的元素
	diff, _ := rdb.SDiff(ctx, setA, setB).Result()
	fmt.Println("SDIFF  (A-B):", diff) // {1, 2}

	// 反过来：在 B 中但不在 A 中
	diff2, _ := rdb.SDiff(ctx, setB, setA).Result()
	fmt.Println("SDIFF  (B-A):", diff2) // {5, 6}

	rdb.Del(ctx, setA, setB)
}

func case5_srandmember(ctx context.Context, rdb *redis.Client) {
	// SRANDMEMBER：随机返回元素，不从集合中删除
	// 适合：展示随机推荐、不消耗名额的抽样
	key := "lottery:pool"
	rdb.Del(ctx, key)
	rdb.SAdd(ctx, key, "Alice", "Bob", "Charlie", "Dave", "Eve", "Frank")

	// count > 0：返回 count 个不重复的随机元素
	winners, _ := rdb.SRandMemberN(ctx, key, 3).Result()
	fmt.Println("随机抽取3人(不删除):", winners)

	// 集合元素数量不变
	remaining, _ := rdb.SCard(ctx, key).Result()
	fmt.Println("抽取后集合数量:", remaining) // 仍是 6

	rdb.Del(ctx, key)
}

func case6_spop(ctx context.Context, rdb *redis.Client) {
	// SPOP：随机弹出元素，同时从集合中删除
	// 适合：真实抽奖（中奖后从奖池移除，不会重复中奖）
	key := "lottery:realpool"
	rdb.Del(ctx, key)
	rdb.SAdd(ctx, key, "Alice", "Bob", "Charlie", "Dave", "Eve")

	fmt.Println("奖池:", mustSMembers(ctx, rdb, key))

	// 一等奖：抽1人
	first, _ := rdb.SPop(ctx, key).Result()
	fmt.Println("一等奖:", first)

	// 二等奖：抽2人
	second, _ := rdb.SPopN(ctx, key, 2).Result()
	fmt.Println("二等奖:", second)

	remaining, _ := rdb.SMembers(ctx, key).Result()
	fmt.Println("剩余未中奖:", remaining)

	rdb.Del(ctx, key)
}

func case7_tags(ctx context.Context, rdb *redis.Client) {
	// 标签系统：给文章打标签，查询有共同标签的文章
	rdb.Del(ctx, "tags:article:1", "tags:article:2", "tags:article:3")

	rdb.SAdd(ctx, "tags:article:1", "golang", "redis", "backend")
	rdb.SAdd(ctx, "tags:article:2", "golang", "grpc", "backend")
	rdb.SAdd(ctx, "tags:article:3", "redis", "mysql", "database")

	// 查找同时包含 golang 和 backend 标签的文章
	// 思路：反向索引，每个标签存哪些文章
	rdb.Del(ctx, "tag:golang", "tag:backend", "tag:redis")
	rdb.SAdd(ctx, "tag:golang", "article:1", "article:2")
	rdb.SAdd(ctx, "tag:backend", "article:1", "article:2")
	rdb.SAdd(ctx, "tag:redis", "article:1", "article:3")

	// 同时有 golang 和 backend 标签的文章
	result, _ := rdb.SInter(ctx, "tag:golang", "tag:backend").Result()
	fmt.Println("同时含 golang+backend 的文章:", result)

	// 含 golang 或 redis 标签的文章（并集）
	result2, _ := rdb.SUnion(ctx, "tag:golang", "tag:redis").Result()
	fmt.Println("含 golang 或 redis 的文章:", result2)

	rdb.Del(ctx, "tags:article:1", "tags:article:2", "tags:article:3",
		"tag:golang", "tag:backend", "tag:redis")
}

func mustSMembers(ctx context.Context, rdb *redis.Client, key string) []string {
	m, _ := rdb.SMembers(ctx, key).Result()
	return m
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}

func checkErr(err error) {
	if err != nil && !errors.Is(err, redis.Nil) {
		panic(err)
	}
}
