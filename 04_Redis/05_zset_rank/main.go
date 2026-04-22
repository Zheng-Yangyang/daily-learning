// 案例05：Redis ZSet（有序集合）—— 排行榜
// 知识点：ZADD / ZSCORE / ZRANK / ZRANGE / ZREVRANGE
//
//	ZINCRBY / ZREM / ZCOUNT / ZRANGEBYSCORE
//
// ZSet 是有序、不重复的集合，每个元素关联一个 score（浮点数）
// 底层是 ziplist + skiplist，查询和更新都是 O(logN)
// 典型场景：
//
//	① 游戏排行榜（按分数排名）
//	② 热搜榜（按热度排序）
//	③ 延迟队列（score 存时间戳）
//	④ 范围查询（按分数区间筛选）
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

	fmt.Println("=== case1: ZADD / ZSCORE / ZCARD ===")
	case1_basic(ctx, rdb)

	fmt.Println("\n=== case2: ZRANK / ZREVRANK 排名查询 ===")
	case2_rank(ctx, rdb)

	fmt.Println("\n=== case3: ZRANGE / ZREVRANGE 范围查询 ===")
	case3_range(ctx, rdb)

	fmt.Println("\n=== case4: ZINCRBY 分数更新（实时排行榜）===")
	case4_zincrby(ctx, rdb)

	fmt.Println("\n=== case5: ZRANGEBYSCORE 按分数区间查询 ===")
	case5_rangeByScore(ctx, rdb)

	fmt.Println("\n=== case6: ZREM / ZREMRANGEBYRANK 删除 ===")
	case6_remove(ctx, rdb)

	fmt.Println("\n=== case7: 综合案例 —— 游戏周榜 ===")
	case7_weeklyRank(ctx, rdb)
}

func case1_basic(ctx context.Context, rdb *redis.Client) {
	key := "demo:zset"
	rdb.Del(ctx, key)

	// ZADD key score member [score member ...]
	rdb.ZAdd(ctx, key,
		redis.Z{Score: 100, Member: "Alice"},
		redis.Z{Score: 200, Member: "Bob"},
		redis.Z{Score: 150, Member: "Charlie"},
		redis.Z{Score: 200, Member: "Dave"}, // 和 Bob 同分
		redis.Z{Score: 50, Member: "Eve"},
	)

	// ZSCORE 获取某个成员的分数
	score, _ := rdb.ZScore(ctx, key, "Alice").Result()
	fmt.Printf("ZSCORE Alice → %.0f\n", score)

	// 不存在的成员返回 redis.Nil
	_, err := rdb.ZScore(ctx, key, "nobody").Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println("ZSCORE nobody → 不存在 (redis.Nil)")
	}

	// ZCARD 集合元素总数
	count, _ := rdb.ZCard(ctx, key).Result()
	fmt.Println("ZCARD →", count)

	rdb.Del(ctx, key)
}

func case2_rank(ctx context.Context, rdb *redis.Client) {
	key := "rank:score"
	rdb.Del(ctx, key)

	rdb.ZAdd(ctx, key,
		redis.Z{Score: 300, Member: "Alice"},
		redis.Z{Score: 500, Member: "Bob"},
		redis.Z{Score: 400, Member: "Charlie"},
		redis.Z{Score: 100, Member: "Dave"},
	)

	// ZRANK：从低到高排名，0-based（分数最低 rank=0）
	rank, _ := rdb.ZRank(ctx, key, "Alice").Result()
	fmt.Printf("ZRANK  Alice（从低到高）→ 第%d名\n", rank+1)

	// ZREVRANK：从高到低排名，0-based（分数最高 rank=0）
	revRank, _ := rdb.ZRevRank(ctx, key, "Alice").Result()
	fmt.Printf("ZREVRANK Alice（从高到低）→ 第%d名\n", revRank+1)

	// 打印所有人排名（从高到低）
	fmt.Println("排行榜（从高到低）:")
	members, _ := rdb.ZRevRangeWithScores(ctx, key, 0, -1).Result()
	for i, m := range members {
		fmt.Printf("  第%d名: %-10s %.0f分\n", i+1, m.Member, m.Score)
	}

	rdb.Del(ctx, key)
}

func case3_range(ctx context.Context, rdb *redis.Client) {
	key := "rank:range"
	rdb.Del(ctx, key)

	rdb.ZAdd(ctx, key,
		redis.Z{Score: 1, Member: "E"},
		redis.Z{Score: 2, Member: "D"},
		redis.Z{Score: 3, Member: "C"},
		redis.Z{Score: 4, Member: "B"},
		redis.Z{Score: 5, Member: "A"},
	)

	// ZRANGE：从低到高，返回成员
	asc, _ := rdb.ZRange(ctx, key, 0, -1).Result()
	fmt.Println("ZRANGE（低→高）:", asc)

	// ZREVRANGE：从高到低
	desc, _ := rdb.ZRevRange(ctx, key, 0, -1).Result()
	fmt.Println("ZREVRANGE（高→低）:", desc)

	// 带分数一起返回
	withScores, _ := rdb.ZRangeWithScores(ctx, key, 0, 2).Result()
	fmt.Println("ZRANGE 前3名（带分数）:")
	for _, z := range withScores {
		fmt.Printf("  %s → %.0f\n", z.Member, z.Score)
	}

	rdb.Del(ctx, key)
}

func case4_zincrby(ctx context.Context, rdb *redis.Client) {
	key := "rank:game"
	rdb.Del(ctx, key)

	// 初始化玩家分数
	rdb.ZAdd(ctx, key,
		redis.Z{Score: 0, Member: "Alice"},
		redis.Z{Score: 0, Member: "Bob"},
		redis.Z{Score: 0, Member: "Charlie"},
	)

	// 模拟游戏对局得分
	events := []struct {
		player string
		delta  float64
		desc   string
	}{
		{"Alice", 100, "击杀"},
		{"Bob", 150, "击杀"},
		{"Alice", 50, "助攻"},
		{"Charlie", 200, "MVP"},
		{"Bob", 80, "击杀"},
		{"Alice", 120, "击杀"},
	}

	for _, e := range events {
		newScore, _ := rdb.ZIncrBy(ctx, key, e.delta, e.player).Result()
		fmt.Printf("  %s [%s] +%.0f → 总分: %.0f\n", e.player, e.desc, e.delta, newScore)
	}

	fmt.Println("最终排行:")
	members, _ := rdb.ZRevRangeWithScores(ctx, key, 0, -1).Result()
	for i, m := range members {
		fmt.Printf("  第%d名: %-10s %.0f分\n", i+1, m.Member, m.Score)
	}

	rdb.Del(ctx, key)
}

func case5_rangeByScore(ctx context.Context, rdb *redis.Client) {
	key := "rank:score_range"
	rdb.Del(ctx, key)

	rdb.ZAdd(ctx, key,
		redis.Z{Score: 60, Member: "F"},
		redis.Z{Score: 70, Member: "E"},
		redis.Z{Score: 80, Member: "D"},
		redis.Z{Score: 90, Member: "C"},
		redis.Z{Score: 95, Member: "B"},
		redis.Z{Score: 100, Member: "A"},
	)

	// ZRANGEBYSCORE：按分数区间查询（从低到高）
	// "+inf" "-inf" 表示正负无穷
	opt := &redis.ZRangeBy{Min: "80", Max: "95"}
	members, _ := rdb.ZRangeByScoreWithScores(ctx, key, opt).Result()
	fmt.Println("分数在 [80, 95] 的成员:")
	for _, m := range members {
		fmt.Printf("  %s → %.0f\n", m.Member, m.Score)
	}

	// ZCOUNT：统计分数区间内的元素数量
	count, _ := rdb.ZCount(ctx, key, "80", "+inf").Result()
	fmt.Println("分数 >= 80 的人数:", count)

	// 查询前3名（分数最高）
	top3, _ := rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: "-inf", Max: "+inf", Offset: 0, Count: 3,
	}).Result()
	fmt.Println("Top3:")
	for i, m := range top3 {
		fmt.Printf("  第%d名: %s %.0f分\n", i+1, m.Member, m.Score)
	}

	rdb.Del(ctx, key)
}

func case6_remove(ctx context.Context, rdb *redis.Client) {
	key := "rank:remove"
	rdb.Del(ctx, key)

	rdb.ZAdd(ctx, key,
		redis.Z{Score: 100, Member: "A"},
		redis.Z{Score: 200, Member: "B"},
		redis.Z{Score: 300, Member: "C"},
		redis.Z{Score: 400, Member: "D"},
		redis.Z{Score: 500, Member: "E"},
	)

	// ZREM 删除指定成员
	rdb.ZRem(ctx, key, "A")
	members, _ := rdb.ZRange(ctx, key, 0, -1).Result()
	fmt.Println("ZREM A 后:", members)

	// ZREMRANGEBYRANK 按排名区间删除（从低到高，删除排名最低的2个）
	rdb.ZRemRangeByRank(ctx, key, 0, 1)
	members, _ = rdb.ZRange(ctx, key, 0, -1).Result()
	fmt.Println("ZREMRANGEBYRANK 0 1 后:", members)

	// ZREMRANGEBYSCORE 按分数区间删除
	rdb.ZRemRangeByScore(ctx, key, "400", "500")
	members, _ = rdb.ZRange(ctx, key, 0, -1).Result()
	fmt.Println("ZREMRANGEBYSCORE 400 500 后:", members)

	rdb.Del(ctx, key)
}

func case7_weeklyRank(ctx context.Context, rdb *redis.Client) {
	// 综合案例：游戏周榜
	// 设计：每周一个 key，自动过期，支持实时更新和查询
	key := "game:rank:2024-w01"
	rdb.Del(ctx, key)

	// 模拟一周内玩家得分上报
	type ScoreEvent struct {
		player string
		score  float64
	}
	events := []ScoreEvent{
		{"Alice", 500}, {"Bob", 300}, {"Charlie", 800},
		{"Alice", 200}, {"Dave", 600}, {"Bob", 400},
		{"Eve", 750}, {"Charlie", 150}, {"Alice", 300},
	}

	for _, e := range events {
		rdb.ZIncrBy(ctx, key, e.score, e.player)
	}

	// 查看完整榜单
	fmt.Println("本周排行榜 TOP5:")
	top, _ := rdb.ZRevRangeWithScores(ctx, key, 0, 4).Result()
	for i, m := range top {
		rank := int64(i + 1)
		fmt.Printf("  🏆 第%d名: %-10s %.0f分\n", rank, m.Member, m.Score)
	}

	// 查询某玩家排名和分数
	player := "Alice"
	score, _ := rdb.ZScore(ctx, key, player).Result()
	revRank, _ := rdb.ZRevRank(ctx, key, player).Result()
	fmt.Printf("\n%s 的成绩: %.0f分，排名第%d\n", player, score, revRank+1)

	// 查询总参与人数
	total, _ := rdb.ZCard(ctx, key).Result()
	fmt.Println("本周参与人数:", total)

	rdb.Del(ctx, key)
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}

func checkErr(err error) {
	if err != nil && !errors.Is(err, redis.Nil) {
		panic(err)
	}
}
