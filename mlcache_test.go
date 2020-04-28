// Copyright (C) Liang Hong (lianghong@tencent.com)

package mlcache

import (
	// "fmt"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis"
	"github.com/gomodule/redigo/redis"
	"github.com/patrickmn/go-cache"
)

type TestStruct struct {
	Num      int
	children []*TestStruct
}

var addr, addr2 string
var s *miniredis.Miniredis

func init() {
	// this is used for L2 cache
	var err error
	s, err = miniredis.Run()
	if err != nil {
		panic(err)
	}
	// defer s.Close()
	// Optionally set some keys your code expects:
	s.Set("foo", "bar")
	addr = s.Addr()

	// this is used for L3 cache
	s2, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	s2.Set("foobar", "foo/bar")
	s2.Set("foo", "bar")
	addr2 = s2.Addr()
}

// test l1: no l2, l3 cache handler
func TestMlCache(t *testing.T) {
	mlc := New(3, cache.DefaultExpiration, 0, nil, nil)
	val, cs, err := mlc.Get("foo", Opt{Ttl: 5 * time.Second, L2: nil, L3: nil}, nil)
	if err != nil {
		t.Error("err != nil")
	}
	if cs.Found {
		t.Error("foo was found")
	}
	if val != nil {
		t.Error("foo val was not nil")
	}
}

// test l1: no l2, l3 cache handler
// get staled value
// func TestMlCache2(t *testing.T) {
// 	mlc := New(3, cache.DefaultExpiration, 0, nil, nil)
// 	mlc.(*mLCache).L1.Set("foo", "bar", 1*time.Second)
// 	val, cs, err := mlc.Get("foo", Opt{Ttl: 5 * time.Second, L2: nil, L3: nil})
// 	if err != nil {
// 		t.Error("err != nil")
// 	}
// 	if !cs.found {
// 		t.Error("foo was not found")
// 	}
// 	if val == nil {
// 		t.Error("foo's val was nil")
// 	}
// 	if val != "bar" {
// 		t.Error("foo's val not equal bar")
// 	}
//
// 	time.Sleep(1 * time.Second)
//
// 	val, cs, err = mlc.Get("foo", Opt{Ttl: 5 * time.Second})
// 	if err != nil {
// 		t.Error("err != nil")
// 	}
// 	if cs.found {
// 		t.Error("foo was found")
// 	}
// 	if cs.stale {
// 		t.Error("foo was found staled")
// 	}
// }

// L2 GetCacheHandler
func gh(key string, ctx interface{}) (interface{}, bool, error) {
	c, err := redis.Dial("tcp", addr)
	if err != nil {
		return nil, false, err
	}
	val, err := c.Do("GET", key)
	found := true
	// nil or interface{}{}
	if val == nil {
		found = false
	}
	return val, found, err
}

// L2 SetCacheHandler
func sh(key string, val interface{}, ttl time.Duration, ctx interface{}) error {
	c, err := redis.Dial("tcp", addr)
	if err != nil {
		return err
	}
	_, err = c.Do("SET", key, val)
	return err
}

// L3 GetCacheHandler
func l3gh(key string, ctx interface{}) (interface{}, bool, error) {
	c, _ := redis.Dial("tcp", addr2)
	val, err := c.Do("GET", key)
	found := true
	// nil or interface{}{}
	if val == nil {
		found = false
	}
	return val, found, err
}

// test l2: if l1 is missing then found from l2
// use mlc's L2
// should set to L1
func TestMlCache3(t *testing.T) {
	lc := &LC{
		GetCacheHandler: gh,
		SetCacheHandler: sh,
	}
	mlc := New(3, cache.DefaultExpiration, 0, lc, nil)
	val, cs, err := mlc.Get("foo", Opt{Ttl: 5 * time.Second}, nil)
	if err != nil {
		t.Error("err != nil")
	}
	if !cs.Found {
		t.Error("foo's val was not found")
	}
	if string(val.([]byte)) != "bar" {
		t.Error("foo's val was not equal bar")
	}
	if cs.CacheFlag != "L2" {
		t.Error("foo's val was not found in L2 cache")
	}

	var t_ time.Time
	var found bool
	_, t_, found = mlc.(*mLCache).L1.GetWithExpiration("foo")
	if !found {
		t.Error("foo's val was not found in L1 cache")
	}
	ttl := t_.Unix() - time.Now().Unix()
	if ttl != 5 {
		t.Errorf("foo's val ttl is not equal 5, ttl: %d", ttl)
	}
}

// test l2: if L1 is missing then found from l2
// use opt's L2
func TestMlCache4(t *testing.T) {
	lc := &LC{
		GetCacheHandler: gh,
		SetCacheHandler: sh,
	}
	mlc := New(3, cache.DefaultExpiration, 0, nil, nil)
	val, cs, err := mlc.Get("foo", Opt{Ttl: 5 * time.Second, L2: lc}, nil)
	if err != nil {
		t.Error("err != nil")
	}
	if !cs.Found {
		t.Error("foo's val was not found")
	}
	if string(val.([]byte)) != "bar" {
		t.Error("foo's val was not equal bar")
	}
	if cs.CacheFlag != "L2" {
		t.Error("foo's val was not found in L2 cache")
	}
}

// new gh
func gh2(key string, ctx interface{}) (interface{}, bool, error) {
	c, _ := redis.Dial("tcp", addr)
	c.Do("SET", key, "foo-bar")
	val, err := c.Do("GET", key)
	found := true
	// nil or interface{}{}
	if val == nil {
		found = false
	}
	return val, found, err
}

// test l2: if L1 is missing then found from l2
// both mlc and opt have L2 should use opt's L2
func TestMlCache5(t *testing.T) {
	lc := &LC{
		GetCacheHandler: gh,
		SetCacheHandler: sh,
	}
	mlc := New(3, cache.DefaultExpiration, 0, lc, nil)
	opt := Opt{
		Ttl: 3 * time.Second,
		L2: &LC{
			GetCacheHandler: gh2,
			SetCacheHandler: sh,
		},
	}
	val, cs, err := mlc.Get("foo", opt, nil)
	if err != nil {
		t.Error("err != nil")
	}
	if !cs.Found {
		t.Error("foo's val was not found")
	}
	if string(val.([]byte)) != "foo-bar" {
		t.Error("foo's val was not equal bar")
	}
	if cs.CacheFlag != "L2" {
		t.Error("foo's val was not found in L2 cache")
	}
}

// test l3: if L1, L2 are both missing
// should use L3 cache and set val to L1 and L2
// L2, L3 both use redis
func TestMlCache6(t *testing.T) {
	l2lc := &LC{
		GetCacheHandler: gh,
		SetCacheHandler: sh,
	}

	l3lc := &LC{
		GetCacheHandler: l3gh,
		SetCacheHandler: nil,
	}

	mlc := New(3, cache.DefaultExpiration, 0, nil, nil)
	val, cs, err := mlc.Get("foobar", Opt{Ttl: 5 * time.Second, L2: l2lc, L3: l3lc}, nil)
	if err != nil {
		t.Error("err != nil")
	}
	if !cs.Found {
		t.Error("foobar's val was not found")
	}
	if string(val.([]byte)) != "foo/bar" {
		t.Error("foobar's val was not equal foo/bar")
	}
	if cs.CacheFlag != "L3" {
		t.Error("foo's val was not found in L3 cache")
	}

	// check if set to L2
	c, _ := redis.Dial("tcp", addr)
	val, err = c.Do("GET", "foobar")
	if err != nil {
		t.Error("err != nil")
	}
	if string(val.([]byte)) != "foo/bar" {
		t.Error("foobar's val in L2 cache was not equal foo/bar")
	}

	// check if set to L1
	value, found := mlc.(*mLCache).L1.Get("foobar")
	if !found {
		t.Error("foobar's val was not in L1 cache")
	}
	if string(value.([]byte)) != "foo/bar" {
		t.Error("foobar's val in L2 cache was not equal foo/bar")
	}
}

// if L2 cache is down, use L3 cache
func TestMlCache7(t *testing.T) {
	s.Close()
	l2lc := &LC{
		GetCacheHandler: gh,
		SetCacheHandler: sh,
	}

	l3lc := &LC{
		GetCacheHandler: l3gh,
		SetCacheHandler: nil,
	}
	mlc := New(3, cache.DefaultExpiration, 0, nil, nil)
	val, cs, err := mlc.Get("foo", Opt{Ttl: 5 * time.Second, L2: l2lc, L3: l3lc}, nil)
	if err != nil {
		t.Error("err != nil")
	}
	if !cs.Found {
		t.Error("foo's val was not found")
	}
	if val == nil {
		t.Error("foo' val was nil")
	}
	if cs.CacheFlag != "L3" {
		t.Error("foo's val was not found in L3 cache")
	}
}

// L2 GetCacheHandler
func test8Gh(key string, ctx interface{}) (interface{}, bool, error) {
	if *(ctx.(*string)) != "testCtx" { // check ctx
		return nil, false, errors.New("ctx's val not equal testCtx")
	}
	// do something to get key from L2 cache
	// change ctx
	*(ctx.(*string)) = "changeTestCtx"
	return nil, false, nil
}

// L3 GetCacheHandler
func test8L3Gh(key string, ctx interface{}) (interface{}, bool, error) {
	if *(ctx.(*string)) != "changeTestCtx" { // check ctx
		return nil, false, errors.New("ctx's val not equal testCtx")
	}
	// do something to get key from L3 cache
	return "ok", true, nil
}

// test ctx
func TestMlCache8(t *testing.T) {
	var ctx string = "testCtx"
	l2lc := &LC{GetCacheHandler: test8Gh}
	l3lc := &LC{GetCacheHandler: test8L3Gh}
	mlc := New(3, cache.DefaultExpiration, 0, nil, nil)
	val, cs, err := mlc.Get("foo", Opt{Ttl: 5 * time.Second, L2: l2lc, L3: l3lc}, &ctx)
	if err != nil {
		t.Error("err != nil")
	}
	if !cs.Found {
		t.Error("foo's val was not found")
	}
	if val.(string) != "ok" {
		t.Error("val's val was not found")
	}
	if cs.CacheFlag != "L3" {
		t.Error("foo's val was not found in L3 cache")
	}
	if ctx != "changeTestCtx" {
		t.Error("ctx's val not equal changeTestCtx")
	}
}
