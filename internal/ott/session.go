package ott

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type LatestSession struct {
	Items      []ReleaseItem
	Expiration time.Time
}

var (
	latestSessions = make(map[string]LatestSession)
	latestMutex    sync.Mutex
)

func generateSessionKey() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateLatestSession stores a slice of release items and returns a unique key.
func CreateLatestSession(items []ReleaseItem) string {
	latestMutex.Lock()
	defer latestMutex.Unlock()

	// Prune expired sessions
	now := time.Now()
	for k, v := range latestSessions {
		if now.After(v.Expiration) {
			delete(latestSessions, k)
		}
	}

	key := generateSessionKey()
	latestSessions[key] = LatestSession{
		Items:      items,
		Expiration: now.Add(15 * time.Minute),
	}
	return key
}

// GetLatestSession retrieves a session by key if it hasn't expired.
func GetLatestSession(key string) ([]ReleaseItem, bool) {
	latestMutex.Lock()
	defer latestMutex.Unlock()

	sess, ok := latestSessions[key]
	if !ok || time.Now().After(sess.Expiration) {
		return nil, false
	}
	return sess.Items, true
}
