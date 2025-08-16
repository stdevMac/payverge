package logic

import (
	"sync"
	"time"
)

type Challenge struct {
	Value     string
	ExpiresAt time.Time
}

type ChallengeStore struct {
	challenges map[string]Challenge
	mu         sync.RWMutex
}

func NewChallengeStore() *ChallengeStore {
	return &ChallengeStore{
		challenges: make(map[string]Challenge),
	}
}

func (cs *ChallengeStore) Set(address string, challenge Challenge) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.challenges[address] = challenge
}

func (cs *ChallengeStore) Get(address string) (Challenge, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	challenge, ok := cs.challenges[address]
	return challenge, ok
}

func (cs *ChallengeStore) Delete(address string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.challenges, address)
}

func (cs *ChallengeStore) Cleanup() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for address, challenge := range cs.challenges {
		if time.Now().After(challenge.ExpiresAt) {
			delete(cs.challenges, address)
		}
	}
}
