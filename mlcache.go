package mlcache

import (
	// "errors"
	// "fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

type CacheStatus struct {
	// the ttl of key life cycle
	// ttl time.Duration

	// if found the key
	Found bool

	// if the val staled
	Stale bool

	// CacheFlag: L1/L2/L3
	CacheFlag string
}

type Opt struct {
	// the ttl of key life cycle
	Ttl time.Duration

	Timeout time.Duration

	// L2 cache handler
	L2 *LC

	// L3 cache handler
	L3 *LC
}

type MLCache interface {
	// Set(string, interface{}, Opt)

	Get(string, Opt, interface{}) (interface{}, CacheStatus, error)
}

type GetCacheHandler func(string, interface{}) (interface{}, bool, error)

type SetCacheHandler func(string, interface{}, time.Duration, interface{}) error

type LC struct {
	GetCacheHandler GetCacheHandler

	SetCacheHandler SetCacheHandler
}

type mLCache struct {
	// L1 cache ---> go cache
	L1 *cache.Cache

	// L2 cache ---> redis cache
	L2 *LC

	// L3 cache ---> mysql cache
	L3 *LC

	// retry times when get/set handler failed
	Retry int

	// lock cache key
	Lock *KeyLock

	// global mutex
	Mu *sync.Mutex
}

func New(
	retry int,
	defaultExpiration,
	cleanupInterval time.Duration,
	l2, l3 *LC,
) MLCache {
	return &mLCache{
		L1:    cache.New(defaultExpiration, cleanupInterval),
		Retry: retry,
		Lock:  newKeyLock(0),
		L2:    l2,
		L3:    l3,
		Mu:    &sync.Mutex{},
	}
}

// TODO
// 1. set l3 cache
// 2. set l2 cache
// 3. set l1 cache
// func (mlc *mLCache) Set(key string, val interface{}, opt Opt) { }

// 1. set from l1 cache
// 2. set from l2 cache
// 3. set from l3 cache
func (mlc *mLCache) Get(key string, opt Opt, ctx interface{}) (val interface{}, cs CacheStatus, err error) {
	// err can not be nil
	val, cs, err = mlc.GetFromL1Cache(key, ctx)
	if err != nil {
		return
	}

	// hit l1 cache
	if cs.Found && !cs.Stale {
		cs.CacheFlag = "L1"
		return
	}

	// no L2 cache, should not let val/cs/err be covered
	gh, _ := cacheHandler(opt.L2, mlc.L2)
	if gh == nil {
		return
	}

	// missing L1 cache
	timeout := opt.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	if !mlc.Lock.TimeoutLock(key, timeout) {
		return
	}

	// get lock
	defer mlc.Lock.Unlock(key)

	// first: fetch from L1 cache
	val, cs, err = mlc.GetFromL1Cache(key, ctx)
	if err != nil {
		return
	}

	// hit l1 cache
	if cs.Found && !cs.Stale {
		cs.CacheFlag = "L1"
		return
	}

	// second: fetch from L2 cache and set to L1 cache
	val, cs, err = mlc.GetFromL2AndSetL1Cache(key, opt, ctx)
	if err != nil {
		return
	}

	return
}

func (mlc *mLCache) GetFromL1Cache(key string, ctx interface{}) (val interface{}, cs CacheStatus, err error) {
	var found bool

	l1 := mlc.L1
	val, found = l1.Get(key)
	// cs.ttl = 0
	cs.Found = found
	cs.Stale = false

	return
}

func (mlc *mLCache) GetFromL2Cache(key string, opt Opt, ctx interface{}) (val interface{}, cs CacheStatus, err error) {
	gh, _ := cacheHandler(opt.L2, mlc.L2)
	if gh == nil {
		cs.Found = false
		cs.Stale = false
		return
	}

	var found bool
	retry := mlc.Retry

	for {
		val, found, err = gh(key, ctx)
		if err == nil {
			break
		}

		retry--

		if retry == 0 {
			break
		}
	}

	cs.Found = found
	cs.Stale = false

	return
}

func (mlc *mLCache) GetFromL3Cache(key string, opt Opt, ctx interface{}) (val interface{}, cs CacheStatus, err error) {
	gh, _ := cacheHandler(opt.L3, mlc.L3)
	if gh == nil {
		cs.Found = false
		cs.Stale = false
		return
	}

	var found bool
	retry := mlc.Retry

	for {
		val, found, err = gh(key, ctx)
		if err == nil {
			break
		}
		retry--
		if retry == 0 {
			break
		}
	}

	cs.Found = found
	cs.Stale = false

	return
}

func (mlc *mLCache) SetL1Cache(key string, val interface{}, opt Opt) (err error) {
	mlc.L1.Set(key, val, opt.Ttl)
	return
}

func cacheHandler(lc *LC, lc2 *LC) (gh GetCacheHandler, sh SetCacheHandler) {
	match := true
	if lc == nil && (lc2 == nil || lc2.GetCacheHandler == nil) {
		match = false
		gh = nil
	}

	if lc != nil && lc.GetCacheHandler == nil && (lc2 == nil || lc2.GetCacheHandler == nil) {
		match = false
		gh = nil
	}

	if match {
		if lc != nil && lc.GetCacheHandler != nil {
			gh = lc.GetCacheHandler
		} else {
			gh = lc2.GetCacheHandler
		}
	}

	match = true
	if lc == nil && (lc2 == nil || lc2.SetCacheHandler == nil) {
		match = false
		sh = nil
	}

	if lc != nil && lc.SetCacheHandler == nil && (lc2 == nil || lc2.SetCacheHandler == nil) {
		match = false
		sh = nil
	}

	if match {
		if lc != nil && lc.SetCacheHandler != nil {
			sh = lc.SetCacheHandler
		} else {
			sh = lc2.SetCacheHandler
		}
	}

	return
}

func (mlc *mLCache) SetL2Cache(key string, val interface{}, opt Opt, ctx interface{}) (err error) {
	_, sh := cacheHandler(opt.L2, mlc.L2)
	if sh == nil {
		return
	}

	retry := mlc.Retry

	for {
		err = sh(key, val, opt.Ttl, ctx)
		if err == nil {
			break
		}
		retry--
		if retry == 0 {
			break
		}
	}

	return
}

func (mlc *mLCache) GetFromL2AndSetL1Cache(key string, opt Opt, ctx interface{}) (val interface{}, cs CacheStatus, err error) {
	val, cs, err = mlc.GetFromL2Cache(key, opt, ctx)
	if err != nil {
		// need log something
	}

	// missing L2 cache
	// should lookup from L3 cache and set L1, L2 cache
	if !cs.Found || err != nil {
		// no L3 cache, should not let val/cs/err be covered
		gh, _ := cacheHandler(opt.L3, mlc.L3)
		if gh == nil {
			return
		}

		val, cs, err = mlc.GetFromL3AndSetL1L2Cache(key, opt, ctx)
		return
	}

	cs.CacheFlag = "L2"

	// hit L2 cache
	// should set L1 cache
	mlc.SetL1Cache(key, val, opt)

	return
}

func (mlc *mLCache) GetFromL3AndSetL1L2Cache(key string, opt Opt, ctx interface{}) (val interface{}, cs CacheStatus, err error) {
	val, cs, err = mlc.GetFromL3Cache(key, opt, ctx)
	if !cs.Found || err != nil {
		return
	}

	cs.CacheFlag = "L3"

	// hit L3 cache
	// should set L1 and L2 cache
	mlc.SetL2Cache(key, val, opt, ctx)
	mlc.SetL1Cache(key, val, opt)

	return
}

func (mlc *mLCache) SetL2SetCacheHandler(f SetCacheHandler) {
	mlc.L2.SetCacheHandler = f
}

func (mlc *mLCache) GetL2GetCacheHandler(f GetCacheHandler) {
	mlc.L2.GetCacheHandler = f
}

func (mlc *mLCache) SetL3SetCacheHandler(f SetCacheHandler) {
	mlc.L3.SetCacheHandler = f
}

func (mlc *mLCache) GetL3GetCacheHandler(f GetCacheHandler) {
	mlc.L3.GetCacheHandler = f
}
