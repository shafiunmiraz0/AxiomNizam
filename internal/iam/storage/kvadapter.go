// Package storage — kvadapter.go
//
// Adapter that allows IAM repositories to use the platform KVStore
// interface instead of a raw etcd client.  This enables IAM to work
// with the embedded Raft backend (STORAGE_BACKEND=raft).
package storage

import (
	"context"
	"time"

	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// iamBackend is the internal interface for IAM key-value operations.
// Both etcdStore and kvStoreBackend implement this.
type iamBackend interface {
	put(key string, val []byte, ttl time.Duration) error
	get(key string) ([]byte, error)
	del(key string) error
	list(prefix string) ([][]byte, error)
	delPrefix(prefix string) error
}

// kvStoreBackend wraps a platform KVStore to satisfy iamBackend.
type kvStoreBackend struct {
	kv platformstore.KVStore
}

func newKVStoreBackend(kv platformstore.KVStore) *kvStoreBackend {
	return &kvStoreBackend{kv: kv}
}

func (s *kvStoreBackend) put(key string, val []byte, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	if ttl > 0 {
		return s.kv.PutWithTTL(ctx, key, string(val), ttl)
	}
	return s.kv.Put(ctx, key, string(val))
}

func (s *kvStoreBackend) get(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	val, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if val == "" {
		return nil, nil
	}
	return []byte(val), nil
}

func (s *kvStoreBackend) del(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	return s.kv.Delete(ctx, key)
}

func (s *kvStoreBackend) list(prefix string) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	entries, err := s.kv.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	results := make([][]byte, 0, len(entries))
	for _, v := range entries {
		results = append(results, []byte(v))
	}
	return results, nil
}

func (s *kvStoreBackend) delPrefix(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	entries, err := s.kv.List(ctx, prefix)
	if err != nil {
		return err
	}
	for key := range entries {
		if err := s.kv.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// ── KVStore-based repository constructors ──

func NewKVClientRepository(kv platformstore.KVStore) *EtcdClientRepository {
	return &EtcdClientRepository{store: newKVStoreBackend(kv)}
}

func NewKVRoleRepository(kv platformstore.KVStore) *EtcdRoleRepository {
	return &EtcdRoleRepository{store: newKVStoreBackend(kv)}
}

func NewKVRoleBindingRepository(kv platformstore.KVStore) *EtcdRoleBindingRepository {
	return &EtcdRoleBindingRepository{store: newKVStoreBackend(kv)}
}

func NewKVSessionRepository(kv platformstore.KVStore) *EtcdSessionRepository {
	return &EtcdSessionRepository{store: newKVStoreBackend(kv)}
}

func NewKVRefreshTokenRepository(kv platformstore.KVStore) *EtcdRefreshTokenRepository {
	return &EtcdRefreshTokenRepository{store: newKVStoreBackend(kv)}
}

func NewKVCodeRepository(kv platformstore.KVStore) *EtcdCodeRepository {
	return &EtcdCodeRepository{store: newKVStoreBackend(kv)}
}

func NewKVRevokedTokenStore(kv platformstore.KVStore) *EtcdRevokedTokenStore {
	return &EtcdRevokedTokenStore{store: newKVStoreBackend(kv)}
}
