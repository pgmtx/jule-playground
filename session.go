package main

import (
	"encoding/hex"
	"time"
	"sync"
	"crypto/rand"
	"os"
)

type session struct {
	dir string
	created time.Time
}

const sessionTTL = 2 * time.Minute

var sessionMutex sync.Mutex
var sessions = map[string]session{}

func NewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func AddSession(dir string) string {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	
	token := NewToken()
	sessions[token] = session{dir: dir, created: time.Now()}
	return token
}

func GetSession(token string) (string, bool) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	
	s, ok := sessions[token]
	return s.dir, ok
}

func TakeSession(token string) (string, bool) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	s, ok := sessions[token]
	// delete works fine even when key is absent
	delete(sessions, token)
	return s.dir, ok
}

func CleanupLoop() {
	for range time.Tick(30 * time.Second) {
		var expired []string
		
		sessionMutex.Lock()
		for token, s := range sessions {
			if time.Since(s.created) > sessionTTL {
				expired = append(expired, s.dir)
				// Deleting from a map during iteration is safe in Go
				delete(sessions, token)
			}
		}
		sessionMutex.Unlock()

		for _, dir := range expired {
			_ = os.RemoveAll(dir)
		}
	}
}
