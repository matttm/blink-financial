package store

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(addr string) *RedisQueue {
	return &RedisQueue{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

func (q *RedisQueue) Close() error {
	return q.client.Close()
}

func (q *RedisQueue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

func (q *RedisQueue) Enqueue(ctx context.Context, listKey, value string) error {
	return q.client.RPush(ctx, listKey, value).Err()
}
