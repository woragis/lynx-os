package redis

import (
	"context"
	"strconv"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	redisclient "github.com/redis/go-redis/v9"
)

type LoginRateLimiter struct{ client *redisclient.Client }

func NewLoginRateLimiter(client *redisclient.Client) *LoginRateLimiter {
	return &LoginRateLimiter{client: client}
}

func (l *LoginRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	value, err := l.client.Get(ctx, key).Result()
	if err == redisclient.Nil {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return false, err
	}
	return count < 5, nil
}

func (l *LoginRateLimiter) RecordFailure(ctx context.Context, key string) error {
	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		return l.client.Expire(ctx, key, 15*time.Minute).Err()
	}
	return nil
}

func (l *LoginRateLimiter) Reset(ctx context.Context, key string) error {
	return l.client.Del(ctx, key).Err()
}

var _ domain.LoginRateLimiter = (*LoginRateLimiter)(nil)
