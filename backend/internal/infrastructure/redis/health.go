package redis

import (
	"context"

	redisclient "github.com/redis/go-redis/v9"
)

type HealthChecker struct{ Client *redisclient.Client }

func (c HealthChecker) Check(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}
