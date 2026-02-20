package storage

import (
	"fmt"

	"go.etcd.io/bbolt"
)

// BoltStorage provides an interface for the embedded BoltDB datastore.
type BoltStorage struct {
	db *bbolt.DB
}

const (
	ChatBucket    = "ChatLogs"
	ConceptBucket = "Concepts"
	EdgeBucket    = "Edges"
)

// NewBoltStorage opens the database at the given path and sets up initial buckets.
func NewBoltStorage(path string) (*BoltStorage, error) {
	// Open the db with default options
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open boltdb: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		buckets := []string{ChatBucket, ConceptBucket, EdgeBucket}
		for _, b := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(b))
			if err != nil {
				return fmt.Errorf("create bucket %s: %w", b, err)
			}
		}
		return nil
	})

	if err != nil {
		db.Close()
		return nil, err
	}

	return &BoltStorage{db: db}, nil
}

// Close gracefully closes the database connection.
func (s *BoltStorage) Close() error {
	return s.db.Close()
}

// Put saves a key-value pair in a specified bucket.
func (s *BoltStorage) Put(bucket, key string, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		return b.Put([]byte(key), value)
	})
}

// Get retrieves a value by key from the specified bucket.
func (s *BoltStorage) Get(bucket, key string) ([]byte, error) {
	var val []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}

		v := b.Get([]byte(key))
		if v != nil {
			// BoltDB returns bytes that are only valid during the transaction.
			// To use them after the View function returns, they must be copied.
			val = make([]byte, len(v))
			copy(val, v)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return val, nil
}
