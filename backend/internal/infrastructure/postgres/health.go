package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthChecker struct{ Pool *pgxpool.Pool }

func (c HealthChecker) Check(ctx context.Context) error {
	return c.Pool.Ping(ctx)
}
