package handler

import (
	"context"
	"fmt"
	"sync/atomic"
	user "user-service/kitex_gen/user"
)

type UserServiceImpl struct{}

// 故障开关：0=正常，1=故障
var failMode int32

func SetFailMode(fail bool) {
	if fail {
		atomic.StoreInt32(&failMode, 1)
		fmt.Println("[UserService] ⚠️  故障模式开启")
	} else {
		atomic.StoreInt32(&failMode, 0)
		fmt.Println("[UserService] ✓  恢复正常模式")
	}
}

func (s *UserServiceImpl) GetUser(ctx context.Context, req *user.GetUserRequest) (*user.GetUserResponse, error) {
	if atomic.LoadInt32(&failMode) == 1 {
		fmt.Printf("[UserService] 故障，拒绝请求 user_id=%d\n", req.UserId)
		return nil, fmt.Errorf("service unavailable")
	}

	fmt.Printf("[UserService] 收到请求: user_id=%d\n", req.UserId)
	users := map[int64]*user.GetUserResponse{
		1: {UserId: 1, Name: "张三", Balance: 1000},
		2: {UserId: 2, Name: "李四", Balance: 500},
		3: {UserId: 3, Name: "王五", Balance: 2000},
	}

	u, ok := users[req.UserId]
	if !ok {
		return nil, fmt.Errorf("用户不存在: %d", req.UserId)
	}

	fmt.Printf("[UserService] 返回: %+v\n", u)
	return u, nil
}
