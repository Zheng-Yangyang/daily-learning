// 案例01：Redis 基础连接 & String 操作
// 知识点：SET / GET / DEL / EXPIRE / TTL / INCR / APPEND / MSET / MGET
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
		PoolSize: 10,
	})
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	pong, err := rdb.Ping(ctx).Result()
	checkErr(err)
	fmt.Println("PING →", pong)

	fmt.Println("\n=== case1: SET / GET / DEL ===")
	case1_setGetDel(ctx, rdb)

	fmt.Println("\n=== case2: EXPIRE / TTL / PERSIST ===")
	case2_expire(ctx, rdb)

	fmt.Println("\n=== case3: SETNX（不存在才写入）===")
	case3_setnx(ctx, rdb)

	fmt.Println("\n=== case4: INCR / INCRBY / DECR（计数器）===")
	case4_incr(ctx, rdb)

	fmt.Println("\n=== case5: APPEND / STRLEN ===")
	case5_append(ctx, rdb)

	fmt.Println("\n=== case6: GETDEL（原子读取并删除）===")
	case6_getdel(ctx, rdb)

	fmt.Println("\n=== case7: MSET / MGET（批量操作）===")
	case7_mset(ctx, rdb)
}

func case1_setGetDel(ctx context.Context, rdb *redis.Client) {
	err := rdb.Set(ctx, "name", "golang-redis", 0).Err()
	checkErr(err)

	val, err := rdb.Get(ctx, "name").Result()
	checkErr(err)
	fmt.Println("GET name →", val)

	_, err = rdb.Get(ctx, "not_exist").Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println("GET not_exist → key 不存在 (redis.Nil)")
	}

	rdb.Del(ctx, "name")
	val, _ = rdb.Get(ctx, "name").Result()
	fmt.Println("GET name after DEL →", `"`+val+`"`)
}

func case2_expire(ctx context.Context, rdb *redis.Client) {
	rdb.Set(ctx, "session:001", "user_data", 5*time.Second)
	ttl, _ := rdb.TTL(ctx, "session:001").Result()
	fmt.Printf("TTL session:001 → %v\n", ttl)

	rdb.Set(ctx, "token", "abc123", 0)
	rdb.Expire(ctx, "token", 10*time.Second)
	ttl, _ = rdb.TTL(ctx, "token").Result()
	fmt.Printf("TTL token (after Expire) → %v\n", ttl)

	rdb.Persist(ctx, "token")
	ttl, _ = rdb.TTL(ctx, "token").Result()
	fmt.Printf("TTL token (after Persist) → %v\n", ttl)

	fmt.Print("等待 session:001 过期中...")
	time.Sleep(6 * time.Second)
	_, err := rdb.Get(ctx, "session:001").Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println(" 已过期 ✅")
	}

	rdb.Del(ctx, "token")
}

func case3_setnx(ctx context.Context, rdb *redis.Client) {
	ok, _ := rdb.SetNX(ctx, "lock:order", "worker-1", 30*time.Second).Result()
	fmt.Println("第一次 SetNX →", ok)

	ok, _ = rdb.SetNX(ctx, "lock:order", "worker-2", 30*time.Second).Result()
	fmt.Println("第二次 SetNX →", ok)

	holder, _ := rdb.Get(ctx, "lock:order").Result()
	fmt.Println("锁持有者 →", holder)

	rdb.Del(ctx, "lock:order")
}

func case4_incr(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "pv:home")

	for i := 0; i < 5; i++ {
		count, _ := rdb.Incr(ctx, "pv:home").Result()
		fmt.Printf("  第%d次 INCR → %d\n", i+1, count)
	}

	rdb.IncrBy(ctx, "pv:home", 100)
	count, _ := rdb.Get(ctx, "pv:home").Result()
	fmt.Println("  IncrBy 100 →", count)

	rdb.DecrBy(ctx, "pv:home", 5)
	count, _ = rdb.Get(ctx, "pv:home").Result()
	fmt.Println("  DecrBy 5 →", count)

	rdb.Del(ctx, "pv:home")
}

func case5_append(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "log:20240101")

	rdb.Append(ctx, "log:20240101", "09:00 user_login\n")
	rdb.Append(ctx, "log:20240101", "09:05 view_page\n")
	rdb.Append(ctx, "log:20240101", "09:10 user_logout\n")

	length, _ := rdb.StrLen(ctx, "log:20240101").Result()
	fmt.Println("日志长度(bytes):", length)

	content, _ := rdb.Get(ctx, "log:20240101").Result()
	fmt.Print("日志内容:\n", content)

	rdb.Del(ctx, "log:20240101")
}

func case6_getdel(ctx context.Context, rdb *redis.Client) {
	rdb.Set(ctx, "one_time_token", "secret_abc", 10*time.Minute)

	token, _ := rdb.GetDel(ctx, "one_time_token").Result()
	fmt.Println("第一次消费 token →", token)

	_, err := rdb.GetDel(ctx, "one_time_token").Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println("第二次消费 → token 已不存在，无法重复消费 ✅")
	}
}

func case7_mset(ctx context.Context, rdb *redis.Client) {
	rdb.MSet(ctx,
		"config:timeout", "30",
		"config:retry", "3",
		"config:debug", "false",
	)

	vals, _ := rdb.MGet(ctx, "config:timeout", "config:retry", "config:debug", "config:missing").Result()
	keys := []string{"timeout", "retry", "debug", "missing"}
	for i, k := range keys {
		fmt.Printf("  config:%-10s → %v\n", k, vals[i])
	}

	rdb.Del(ctx, "config:timeout", "config:retry", "config:debug")
}

func checkErr(err error) {
	if err != nil && !errors.Is(err, redis.Nil) {
		panic(err)
	}
}
