package storage

import (
	"encoding/json"
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

	// Index buckets for graph lookups
	ConceptsBySessionBucket = "ConceptsBySession"
	EdgesBySourceBucket     = "EdgesBySource"
	EdgesByTargetBucket     = "EdgesByTarget"
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
		buckets := []string{
			ChatBucket, ConceptBucket, EdgeBucket,
			ConceptsBySessionBucket, EdgesBySourceBucket, EdgesByTargetBucket,
		}
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

// PutJSON marshals the value to JSON and stores it in the specified bucket.
func (s *BoltStorage) PutJSON(bucket, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return s.Put(bucket, key, data)
}

// GetJSON retrieves a value by key and unmarshals it from JSON into dest.
func (s *BoltStorage) GetJSON(bucket, key string, dest any) error {
	data, err := s.Get(bucket, key)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("key %s not found in bucket %s", key, bucket)
	}
	return json.Unmarshal(data, dest)
}

// List returns all key-value pairs in a bucket.
func (s *BoltStorage) List(bucket string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		return b.ForEach(func(k, v []byte) error {
			val := make([]byte, len(v))
			copy(val, v)
			result[string(k)] = val
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListByPrefix returns all key-value pairs in a bucket where the key
// starts with the given prefix. Uses BoltDB cursor seek for efficiency.
func (s *BoltStorage) ListByPrefix(bucket, prefix string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	pfx := []byte(prefix)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		c := b.Cursor()
		for k, v := c.Seek(pfx); k != nil && len(k) >= len(pfx) && string(k[:len(pfx)]) == prefix; k, v = c.Next() {
			val := make([]byte, len(v))
			copy(val, v)
			result[string(k)] = val
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AddToIndex appends a value to a JSON array stored at the given key in the
// specified bucket. This is used for maintaining index lists (e.g., mapping
// a session ID to a list of concept IDs).
func (s *BoltStorage) AddToIndex(bucket, key, value string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}

		var items []string
		existing := b.Get([]byte(key))
		if existing != nil {
			if err := json.Unmarshal(existing, &items); err != nil {
				return fmt.Errorf("unmarshal index: %w", err)
			}
		}

		// Deduplicate
		for _, item := range items {
			if item == value {
				return nil
			}
		}

		items = append(items, value)
		data, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("marshal index: %w", err)
		}
		return b.Put([]byte(key), data)
	})
}

// GetIndex retrieves the string list stored at the given key in the
// specified bucket.
func (s *BoltStorage) GetIndex(bucket, key string) ([]string, error) {
	var items []string
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		v := b.Get([]byte(key))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &items)
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}
