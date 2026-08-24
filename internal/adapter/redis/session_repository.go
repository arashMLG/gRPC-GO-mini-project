// Package redis contains the Redis-backed adapters. The third-party client
// is imported as goredis so that this package can keep the short, meaningful
// name "redis" for itself.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"myGuy/internal/domain"

	goredis "github.com/redis/go-redis/v9"
)

// SessionRepository implements domain.SessionRepository on top of plain
// Redis string keys with a TTL, which is what gives sessions automatic
// expiry without any cleanup job.
type SessionRepository struct {
	client *goredis.Client
}

func NewSessionRepository(client *goredis.Client) *SessionRepository {
	return &SessionRepository{client: client}
}

var _ domain.SessionRepository = (*SessionRepository)(nil)

// sessionKey namespaces session keys so they cannot collide with the
// leaderboard key or anything else sharing this Redis database.
func sessionKey(token string) string {
	return "session:" + token
}

func (r *SessionRepository) Save(ctx context.Context, token, username string, ttl time.Duration) error {
	if err := r.client.Set(ctx, sessionKey(token), username, ttl).Err(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (r *SessionRepository) Username(ctx context.Context, token string) (string, error) {
	username, err := r.client.Get(ctx, sessionKey(token)).Result()
	if errors.Is(err, goredis.Nil) {
		// A missing key means the token is unknown or has expired. Translate
		// the Redis-specific sentinel into the domain's vocabulary.
		return "", domain.ErrNotLoggedIn
	}
	if err != nil {
		return "", fmt.Errorf("read session: %w", err)
	}
	return username, nil
}

func (r *SessionRepository) Delete(ctx context.Context, token string) error {
	if err := r.client.Del(ctx, sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
