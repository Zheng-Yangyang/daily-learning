// 案例12：Redis Session 管理
// 知识点：Hash 存储 Session + EXPIRE 滑动过期 + 并发安全
//
// 为什么用 Redis 存 Session：
//
//	① 多实例部署时，本地 Session 无法共享
//	② Redis 天然支持过期，不用手动清理
//	③ 读写速度远快于数据库
//
// 典型场景：
//
//	① 用户登录态管理
//	② 多端登录控制（踢掉其他设备）
//	③ 权限缓存
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ─────────────────────────────────────────────
// Session 结构
// ─────────────────────────────────────────────
type Session struct {
	SessionID string
	UserID    string
	Username  string
	Role      string
	LoginIP   string
	LoginTime string
	Device    string
}

func sessionKey(sid string) string {
	return "session:" + sid
}

func userSessionKey(uid string) string {
	return "user:sessions:" + uid
}

// ─────────────────────────────────────────────
// SessionManager
// ─────────────────────────────────────────────
type SessionManager struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSessionManager(rdb *redis.Client, ttl time.Duration) *SessionManager {
	return &SessionManager{rdb: rdb, ttl: ttl}
}

// 生成随机 SessionID
func generateSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Create 创建 Session
func (m *SessionManager) Create(ctx context.Context, s Session) (string, error) {
	sid := generateSessionID()
	s.SessionID = sid
	key := sessionKey(sid)

	_, err := m.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, key,
			"session_id", sid,
			"user_id", s.UserID,
			"username", s.Username,
			"role", s.Role,
			"login_ip", s.LoginIP,
			"login_time", s.LoginTime,
			"device", s.Device,
		)
		pipe.Expire(ctx, key, m.ttl)
		// 记录该用户的所有 session（用于多端登录控制）
		pipe.SAdd(ctx, userSessionKey(s.UserID), sid)
		pipe.Expire(ctx, userSessionKey(s.UserID), m.ttl)
		return nil
	})
	return sid, err
}

// Get 获取 Session（并刷新 TTL）
func (m *SessionManager) Get(ctx context.Context, sid string) (*Session, error) {
	key := sessionKey(sid)
	fields, err := m.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, errors.New("session 不存在或已过期")
	}
	// 滑动过期：每次访问刷新 TTL
	m.rdb.Expire(ctx, key, m.ttl)

	return &Session{
		SessionID: fields["session_id"],
		UserID:    fields["user_id"],
		Username:  fields["username"],
		Role:      fields["role"],
		LoginIP:   fields["login_ip"],
		LoginTime: fields["login_time"],
		Device:    fields["device"],
	}, nil
}

// Delete 删除 Session（登出）
func (m *SessionManager) Delete(ctx context.Context, sid string) error {
	// 先获取 userID
	uid, err := m.rdb.HGet(ctx, sessionKey(sid), "user_id").Result()
	if err != nil {
		return err
	}
	_, err = m.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, sessionKey(sid))
		pipe.SRem(ctx, userSessionKey(uid), sid)
		return nil
	})
	return err
}

// DeleteAll 删除用户所有 Session（强制下线所有设备）
func (m *SessionManager) DeleteAll(ctx context.Context, uid string) error {
	sids, err := m.rdb.SMembers(ctx, userSessionKey(uid)).Result()
	if err != nil {
		return err
	}
	if len(sids) == 0 {
		return nil
	}
	_, err = m.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, sid := range sids {
			pipe.Del(ctx, sessionKey(sid))
		}
		pipe.Del(ctx, userSessionKey(uid))
		return nil
	})
	return err
}

// ListUserSessions 列出用户所有在线 Session
func (m *SessionManager) ListUserSessions(ctx context.Context, uid string) ([]*Session, error) {
	sids, err := m.rdb.SMembers(ctx, userSessionKey(uid)).Result()
	if err != nil {
		return nil, err
	}
	var sessions []*Session
	for _, sid := range sids {
		s, err := m.Get(ctx, sid)
		if err != nil {
			// session 已过期，清理索引
			m.rdb.SRem(ctx, userSessionKey(uid), sid)
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 创建和获取 Session ===")
	case1_createGet(ctx, rdb)

	fmt.Println("\n=== case2: 滑动过期 ===")
	case2_slidingExpire(ctx, rdb)

	fmt.Println("\n=== case3: 登出（删除 Session）===")
	case3_logout(ctx, rdb)

	fmt.Println("\n=== case4: 多端登录控制 ===")
	case4_multiDevice(ctx, rdb)

	fmt.Println("\n=== case5: 强制下线所有设备 ===")
	case5_forceLogout(ctx, rdb)
}

func case1_createGet(ctx context.Context, rdb *redis.Client) {
	mgr := NewSessionManager(rdb, 30*time.Minute)

	// 创建 Session
	sid, err := mgr.Create(ctx, Session{
		UserID:    "1001",
		Username:  "Alice",
		Role:      "admin",
		LoginIP:   "192.168.1.100",
		LoginTime: time.Now().Format(time.RFC3339),
		Device:    "Chrome/Mac",
	})
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}
	fmt.Println("创建 Session:", sid)

	// 获取 Session
	s, err := mgr.Get(ctx, sid)
	if err != nil {
		fmt.Println("获取失败:", err)
		return
	}
	fmt.Printf("获取 Session: user=%s role=%s device=%s\n", s.Username, s.Role, s.Device)

	// 查看 TTL
	ttl, _ := rdb.TTL(ctx, sessionKey(sid)).Result()
	fmt.Printf("Session TTL: %v\n", ttl.Round(time.Second))

	// 获取不存在的 Session
	_, err = mgr.Get(ctx, "not_exist_sid")
	if err != nil {
		fmt.Println("获取不存在的 Session:", err)
	}

	mgr.DeleteAll(ctx, "1001")
}

func case2_slidingExpire(ctx context.Context, rdb *redis.Client) {
	// TTL 设为 3 秒，演示滑动过期
	mgr := NewSessionManager(rdb, 3*time.Second)

	sid, _ := mgr.Create(ctx, Session{
		UserID:    "1002",
		Username:  "Bob",
		Role:      "user",
		LoginIP:   "10.0.0.1",
		LoginTime: time.Now().Format(time.RFC3339),
		Device:    "Safari/iPhone",
	})

	fmt.Println("Session 创建，TTL=3s")

	// 每隔 2 秒访问一次，TTL 会被刷新，Session 不会过期
	for i := 1; i <= 3; i++ {
		time.Sleep(2 * time.Second)
		s, err := mgr.Get(ctx, sid)
		if err != nil {
			fmt.Printf("  第%d次访问（%ds后）: Session 已过期\n", i, i*2)
		} else {
			ttl, _ := rdb.TTL(ctx, sessionKey(sid)).Result()
			fmt.Printf("  第%d次访问（%ds后）: user=%s TTL刷新为%v ✅\n",
				i, i*2, s.Username, ttl.Round(time.Second))
		}
	}

	mgr.DeleteAll(ctx, "1002")
}

func case3_logout(ctx context.Context, rdb *redis.Client) {
	mgr := NewSessionManager(rdb, 30*time.Minute)

	sid, _ := mgr.Create(ctx, Session{
		UserID:    "1003",
		Username:  "Charlie",
		Role:      "user",
		LoginIP:   "172.16.0.1",
		LoginTime: time.Now().Format(time.RFC3339),
		Device:    "Firefox/Windows",
	})
	fmt.Println("Session 已创建:", sid[:8]+"...")

	// 登出
	err := mgr.Delete(ctx, sid)
	if err != nil {
		fmt.Println("登出失败:", err)
		return
	}
	fmt.Println("登出成功")

	// 再次获取，应该不存在
	_, err = mgr.Get(ctx, sid)
	if err != nil {
		fmt.Println("登出后获取 Session:", err, "✅")
	}
}

func case4_multiDevice(ctx context.Context, rdb *redis.Client) {
	mgr := NewSessionManager(rdb, 30*time.Minute)
	uid := "1004"
	rdb.Del(ctx, userSessionKey(uid))

	// 同一用户从3个设备登录
	devices := []string{"Chrome/Mac", "Safari/iPhone", "App/Android"}
	for _, device := range devices {
		sid, _ := mgr.Create(ctx, Session{
			UserID:    uid,
			Username:  "Dave",
			Role:      "user",
			LoginIP:   "10.0.0.1",
			LoginTime: time.Now().Format(time.RFC3339),
			Device:    device,
		})
		fmt.Printf("  设备 [%s] 登录，SessionID: %s...\n", device, sid[:8])
	}

	// 查看所有在线 Session
	sessions, _ := mgr.ListUserSessions(ctx, uid)
	fmt.Printf("  用户 Dave 当前在线设备数: %d\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("    - %s\n", s.Device)
	}

	mgr.DeleteAll(ctx, uid)
}

func case5_forceLogout(ctx context.Context, rdb *redis.Client) {
	mgr := NewSessionManager(rdb, 30*time.Minute)
	uid := "1005"

	// 用户从多个设备登录
	for _, device := range []string{"Web", "iOS", "Android"} {
		mgr.Create(ctx, Session{
			UserID:    uid,
			Username:  "Eve",
			Role:      "user",
			LoginIP:   "10.0.0.1",
			LoginTime: time.Now().Format(time.RFC3339),
			Device:    device,
		})
	}

	sessions, _ := mgr.ListUserSessions(ctx, uid)
	fmt.Printf("  强制下线前，在线设备数: %d\n", len(sessions))

	// 强制下线所有设备（密码修改、账号封禁等场景）
	err := mgr.DeleteAll(ctx, uid)
	if err != nil {
		fmt.Println("强制下线失败:", err)
		return
	}
	fmt.Println("  已强制下线所有设备 ✅")

	// 验证所有 Session 已删除
	sessions, _ = mgr.ListUserSessions(ctx, uid)
	fmt.Printf("  强制下线后，在线设备数: %d ✅\n", len(sessions))
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
