// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Package cache provides general-purpose in-memory caches.
// Different caches provide different eviction policies suitable for
// specific use cases.
package cache

import (
	"time"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"istio.io/istio/pkg/log"
)

var XdsCache = log.RegisterScope("xds-cache", "Xds entire cache.")

type XdsResourceCache interface {
	// Initialize try to load cache without returning error
	Initialize()

	// Load xds resource base on the discovery request.
	Load(req *discovery.DiscoveryRequest) (*discovery.DiscoveryResponse, error)

	// Add xds resource into memory but not store, we should wait an ack to
	// make sure these resources are valid and accepted by envoy.
	Add(resp *discovery.DiscoveryResponse) error

	// Store xds resource into store base on the ack discovery request.
	// Caller should make sure this discovery request is ack request.
	Store(req *discovery.DiscoveryRequest) error
}

// Stats returns usage statistics about an individual cache, useful to assess the
// efficiency of a cache.
//
// The values returned in this struct are approximations of the current state of the cache.
// For the sake of efficiency, certain edge cases in the implementation can lead to
// inaccuracies.
type Stats struct {
	// Writes captures the number of times state in the cache was added or updated.
	Writes uint64

	// Hits captures the number of times a Get operation succeeded to find an entry in the cache.
	Hits uint64

	// Misses captures the number of times a Get operation failed to find an entry in the cache.
	Misses uint64

	// Evictions captures the number of entries that have been evicted from the cache
	Evictions uint64

	// Removals captures the number of entries that have been explicitly removed from the
	// cache
	Removals uint64
}

// Cache defines the standard behavior of in-memory thread-safe caches.
//
// Different caches can have different eviction policies which determine
// when and how entries are automatically removed from the cache.
//
// Using a cache is very simple:
//
//	  c := NewLRU(5*time.Second,     // default per-entry ttl
//	              5*time.Second,     // eviction interval
//	              500)               // max # of entries tracked
//	  c.Set("foo", "bar")			// add an entry
//	  value, ok := c.Get("foo")		// try to retrieve the entry
//	  if ok {
//			fmt.Printf("Got value %v\n", value)
//	  } else {
//	     fmt.Printf("Value was not found, must have been evicted")
//	  }
type Cache interface {
	// Ideas for the future:
	//   - Return the number of entries in the cache in stats.
	//   - Provide an eviction callback to know when entries are evicted.
	//   - Have Set and Remove return the previous value for the key, if any.
	//   - Have Get return the expiration time for entries.

	// Set inserts an entry in the cache. This will replace any entry with
	// the same key that is already in the cache. The entry may be automatically
	// expunged from the cache at some point, depending on the eviction policies
	// of the cache and the options specified when the cache was created.
	Set(key any, value any)

	// Get retrieves the value associated with the supplied key if the key
	// is present in the cache.
	Get(key any) (value any, ok bool)

	// Remove synchronously deletes the given key from the cache. This has no effect if the key is not
	// currently in the cache.
	Remove(key any)

	// RemoveAll synchronously deletes all entries from the cache.
	RemoveAll()

	// Stats returns information about the efficiency of the cache.
	Stats() Stats
}

// ExpiringCache is a cache with entries that are evicted over time
type ExpiringCache interface {
	Cache

	// SetWithExpiration inserts an entry in the cache with a requested expiration time.
	// This will replace any entry with the same key that is already in the cache.
	// The entry will be automatically expunged from the cache at or slightly after the
	// requested expiration time.
	SetWithExpiration(key any, value any, expiration time.Duration)

	// EvictExpired() synchronously evicts all expired entries from the cache
	EvictExpired()
}
