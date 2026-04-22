// 案例02：Redis Hash —— 用户信息存储
// 知识点：HSET / HGET / HMGET / HGETALL / HDEL / HEXISTS / HINCRBY / HKEYS / HLEN
//
// Hash 就像 Go 里的 map[string]string，适合存储对象
// 对比把对象 JSON 序列化成 String 的优势：
//
//	✅ 可单独读/改某个字段，不用读整个对象
//	✅ HINCRBY 原子更新数值字段（积分、余额）
//	✅ 字段少时用 ziplist 编码，内存更节省
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type User struct {
	ID       string
	Name     string
	Email    string
	Age      int
	Score    int
	CreateAt string
}

func userKey(id string) string {
	return fmt.Sprintf("user:%s", id)
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: HSET / HGET / HGETALL ===")
	case1_basic(ctx, rdb)

	fmt.Println("\n=== case2: HMGET 批量读指定字段 ===")
	case2_hmget(ctx, rdb)

	fmt.Println("\n=== case3: HEXISTS / HDEL ===")
	case3_hexistsDel(ctx, rdb)

	fmt.Println("\n=== case4: HINCRBY 原子更新数值（积分系统）===")
	case4_hincrby(ctx, rdb)

	fmt.Println("\n=== case5: HKEYS / HVALS / HLEN ===")
	case5_hkeys(ctx, rdb)

	fmt.Println("\n=== case6: Hash 存 Session（带 TTL）===")
	case6_session(ctx, rdb)

	fmt.Println("\n=== case7: 结构体 <-> Hash 双向映射 ===")
	case7_structMapping(ctx, rdb)
}

func case1_basic(ctx context.Context, rdb *redis.Client) {
	key := userKey("1001")
	rdb.Del(ctx, key)

	// HSET key field value [field value ...]
	rdb.HSet(ctx, key,
		"name", "Alice",
		"email", "alice@example.com",
		"age", "28",
		"score", "100",
		"create_at", time.Now().Format("2006-01-02"),
	)

	// HGET 读单个字段
	name, _ := rdb.HGet(ctx, key, "name").Result()
	fmt.Println("HGET name →", name)

	// 读不存在的字段 → redis.Nil
	_, err := rdb.HGet(ctx, key, "not_exist").Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println("HGET not_exist → 字段不存在 (redis.Nil)")
	}

	// HGETALL 读所有字段，返回 map[string]string
	fields, _ := rdb.HGetAll(ctx, key).Result()
	fmt.Println("HGETALL →")
	for k, v := range fields {
		fmt.Printf("  %-12s: %s\n", k, v)
	}

	rdb.Del(ctx, key)
}

func case2_hmget(ctx context.Context, rdb *redis.Client) {
	key := userKey("1002")
	rdb.Del(ctx, key)
	rdb.HSet(ctx, key, "name", "Bob", "email", "bob@example.com", "age", "32", "score", "250")

	// HMGET 只读需要的字段，不存在的返回 nil
	vals, _ := rdb.HMGet(ctx, key, "name", "score", "not_exist").Result()
	fieldNames := []string{"name", "score", "not_exist"}
	for i, f := range fieldNames {
		fmt.Printf("  HMGET %-12s → %v\n", f, vals[i])
	}

	rdb.Del(ctx, key)
}

func case3_hexistsDel(ctx context.Context, rdb *redis.Client) {
	key := userKey("1003")
	rdb.Del(ctx, key)
	rdb.HSet(ctx, key, "name", "Charlie", "email", "charlie@example.com", "avatar", "http://cdn/avatar.jpg")

	exists, _ := rdb.HExists(ctx, key, "avatar").Result()
	fmt.Println("HEXISTS avatar →", exists) // true

	// HDEL 只删除指定字段，不影响其他字段
	rdb.HDel(ctx, key, "avatar")

	exists, _ = rdb.HExists(ctx, key, "avatar").Result()
	fmt.Println("HEXISTS avatar (after HDel) →", exists) // false

	name, _ := rdb.HGet(ctx, key, "name").Result()
	fmt.Println("其他字段仍在，name →", name)

	rdb.Del(ctx, key)
}

func case4_hincrby(ctx context.Context, rdb *redis.Client) {
	key := userKey("1004")
	rdb.Del(ctx, key)
	rdb.HSet(ctx, key, "name", "Dave", "score", "0", "balance", "99.50")

	tasks := []struct {
		desc  string
		delta int64
	}{
		{"完成签到 +10", 10},
		{"完成任务 +50", 50},
		{"购买道具 -30", -30},
		{"分享获奖 +20", 20},
	}

	for _, t := range tasks {
		newScore, _ := rdb.HIncrBy(ctx, key, "score", t.delta).Result()
		fmt.Printf("  %s → 当前积分: %d\n", t.desc, newScore)
	}

	// HIncrByFloat 支持浮点（余额场景）
	newBal, _ := rdb.HIncrByFloat(ctx, key, "balance", -10.25).Result()
	fmt.Printf("  余额 -10.25 → %.2f\n", newBal)

	rdb.Del(ctx, key)
}

func case5_hkeys(ctx context.Context, rdb *redis.Client) {
	key := userKey("1005")
	rdb.Del(ctx, key)
	rdb.HSet(ctx, key, "name", "Eve", "email", "eve@example.com", "age", "25", "city", "Beijing")

	keys, _ := rdb.HKeys(ctx, key).Result()
	fmt.Println("HKEYS →", keys)

	vals, _ := rdb.HVals(ctx, key).Result()
	fmt.Println("HVALS →", vals)

	length, _ := rdb.HLen(ctx, key).Result()
	fmt.Println("HLEN  →", length)

	rdb.Del(ctx, key)
}

func case6_session(ctx context.Context, rdb *redis.Client) {
	sid := "sess:abc123"
	rdb.Del(ctx, sid)

	rdb.HSet(ctx, sid,
		"user_id", "1001",
		"username", "Alice",
		"role", "admin",
		"login_time", time.Now().Format(time.RFC3339),
		"ip", "192.168.1.100",
	)
	// Hash 没有字段级别 TTL，只能对整个 key 设过期
	rdb.Expire(ctx, sid, 30*time.Minute)

	ttl, _ := rdb.TTL(ctx, sid).Result()
	fmt.Printf("Session TTL → %v\n", ttl)

	// 验权时只读需要的字段
	vals, _ := rdb.HMGet(ctx, sid, "user_id", "role").Result()
	fmt.Printf("Session check: user_id=%v, role=%v\n", vals[0], vals[1])

	rdb.Del(ctx, sid)
}

func case7_structMapping(ctx context.Context, rdb *redis.Client) {
	u := User{
		ID:       "9999",
		Name:     "Frank",
		Email:    "frank@example.com",
		Age:      35,
		Score:    500,
		CreateAt: time.Now().Format("2006-01-02"),
	}

	saveUser(ctx, rdb, u)
	fmt.Printf("已保存: %+v\n", u)

	loaded, err := loadUser(ctx, rdb, "9999")
	if err != nil {
		fmt.Println("加载失败:", err)
		return
	}
	fmt.Printf("已加载: %+v\n", loaded)

	rdb.Del(ctx, userKey("9999"))
}

func saveUser(ctx context.Context, rdb *redis.Client, u User) {
	rdb.HSet(ctx, userKey(u.ID),
		"id", u.ID,
		"name", u.Name,
		"email", u.Email,
		"age", fmt.Sprintf("%d", u.Age),
		"score", fmt.Sprintf("%d", u.Score),
		"create_at", u.CreateAt,
	)
}

func loadUser(ctx context.Context, rdb *redis.Client, id string) (User, error) {
	m, err := rdb.HGetAll(ctx, userKey(id)).Result()
	if err != nil {
		return User{}, err
	}
	if len(m) == 0 {
		return User{}, fmt.Errorf("用户 %s 不存在", id)
	}
	var age, score int
	fmt.Sscan(m["age"], &age)
	fmt.Sscan(m["score"], &score)
	return User{
		ID:       m["id"],
		Name:     m["name"],
		Email:    m["email"],
		Age:      age,
		Score:    score,
		CreateAt: m["create_at"],
	}, nil
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}

func checkErr(err error) {
	if err != nil && !errors.Is(err, redis.Nil) {
		panic(err)
	}
}
