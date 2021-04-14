package mlcache

import (
	"sync"
	"time"
)

type KeyLock struct {
	Mu   *sync.Mutex
	Lock map[string]chan struct{}
}

func newKeyLock(size int64) *KeyLock {
	return &KeyLock{
		Mu:   new(sync.Mutex),
		Lock: make(map[string]chan struct{}, size),
	}
}

func (kl *KeyLock) getVal(key string) chan struct{} {
	kl.Mu.Lock()
	defer kl.Mu.Unlock()

	var ch chan struct{}
	var ok bool

	ch, ok = kl.Lock[key]
	if !ok {
		ch = make(chan struct{}, 1)
		kl.Lock[key] = ch
	}

	return ch
}

func (kl *KeyLock) TimeoutLock(key string, timeout time.Duration) bool {
	ch := kl.getVal(key)

	t := time.NewTimer(timeout)

	defer t.Stop()

	select {
	case ch <- struct{}{}:
		return true
	case <-t.C:
		return false
	}
}

func (kl *KeyLock) Trylock(key string) bool {
	ch := kl.getVal(key)

	select {
	case ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (kl *KeyLock) Unlock(key string) {
	kl.Mu.Lock()
	defer kl.Mu.Unlock()
	<-kl.Lock[key]
}
