# go-mlcache

[![GoDoc](https://godoc.org/github.com/hongliang5316/go-mlcache?status.svg)](https://godoc.org/github.com/hongliang5316/go-mlcache)
[![Build Status](https://travis-ci.org/hongliang5316/go-mlcache.svg?branch=master)](https://travis-ci.org/hongliang5316/go-mlcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/hongliang5316/go-mlcache)](https://goreportcard.com/report/github.com/hongliang5316/go-mlcache)

go-mlcache is a Multiple layer cache library.

## Features:

* Caching and negative caching with TTLs.
* Prevent dog pile effect.

```
      ┌────────────────────────────┐
 L1   │           memory           │
      └────────────────────────────┘
                    │ mutex & callback
                    ▼
      ┌────────────────────────────┐
 L2   │      redis/memcached       │
      └────────────────────────────┘
                    │ callback
                    ▼
      ┌────────────────────────────┐
 L3   │      mysql/postgresql      │
      └────────────────────────────┘
```

## Installation

```bash

go get github.com/hongliang5316/go-mlcache
```

## Example Usage

```go
package main

import (
        "fmt"
        "log"
        "time"

        "github.com/gomodule/redigo/redis"
        mlcache "github.com/hongliang5316/go-mlcache"
        cache "github.com/patrickmn/go-cache"
)

func gh(key string, ctx interface{}) (interface{}, bool, error) {
        c, err := redis.Dial("tcp", "127.0.0.1:6379")
        if err != nil {
                return nil, false, err
        }

        val, err := c.Do("GET", key)
        return val, true, err
}

func main() {
        mlc := mlcache.New(3, cache.DefaultExpiration, 10*time.Minute, nil, nil)
        lc := &mlcache.LC{GetCacheHandler: gh}
        val, cs, err := mlc.Get("foo", mlcache.Opt{Ttl: 3 * time.Second, L2: lc}, nil)
        if err != nil {
                log.Fatalf("Call mlc.Get failed, err:%+v", err)
                return
        }

        // first get val from L2 cache
        fmt.Println("get key: foo from cache layer:", cs.CacheFlag)

        if val == nil {
                fmt.Println("key: foo, not found")
        } else {
                fmt.Println("key: foo, val:", string(val.([]byte)))
        }

        val, cs, err = mlc.Get("foo", mlcache.Opt{Ttl: 3 * time.Second, L2: lc}, nil)
        if err != nil {
                log.Fatalf("Call mlc.Get failed, err:%+v", err)
                return
        }

        // second get val from L1 cache
        fmt.Println("get key: foo from cache layer:", cs.CacheFlag)

        if val == nil {
                fmt.Println("key: foo, not found")
        } else {
                fmt.Println("key: foo, val:", string(val.([]byte)))
        }
}
```
