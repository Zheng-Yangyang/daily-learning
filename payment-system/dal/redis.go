package dal

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(addr string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("Redis 连接失败: %v", err)
	}
	fmt.Println("[Redis] 连接成功 ✓")
	return &Redis{client: client}, nil
}

// ============================================================
// 分布式锁
// ============================================================

type Lock struct {
	redis *Redis
	key   string
	value string
	ttl   time.Duration
}

func (r *Redis) NewLock(key string, ttl time.Duration) *Lock {
	return &Lock{
		redis: r,
		key:   key,
		value: fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63()),
		ttl:   ttl,
	}
}

func (l *Lock) TryLock(ctx context.Context) (bool, error) {
	return l.redis.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
}

func (l *Lock) Unlock(ctx context.Context) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	result, err := l.redis.client.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil {
		return err
	}
	if result.(int64) == 0 {
		return fmt.Errorf("锁已失效或不属于当前持有者")
	}
	return nil
}

// ============================================================
// 缓存操作
// ============================================================

func (r *Redis) SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) GetCache(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *Redis) DelCache(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}
