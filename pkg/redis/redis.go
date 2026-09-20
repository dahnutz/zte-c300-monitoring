package redis

import (
	"net"
	"time"

	"github.com/redis/go-redis/v9"
	"zte-c300-monitoring/config"
)

// NewRedisClient uses the resolved configuration in every environment. Reading
// raw environment variables again here would discard LoadConfig's defaults.
func NewRedisClient(cfg *config.Config) *redis.Client {
	c := cfg.RedisCfg
	return redis.NewClient(&redis.Options{
		Addr: net.JoinHostPort(c.Host, c.Port), Password: c.Password, DB: c.DB,
		MinIdleConns: c.MinIdleConnections, PoolSize: c.PoolSize,
		PoolTimeout: time.Duration(c.PoolTimeout) * time.Second,
	})
}
