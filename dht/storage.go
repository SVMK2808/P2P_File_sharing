package dht

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	badger "github.com/dgraph-io/badger/v3"
)

// Storage interface defines the methods for persistent key-value storage
type Storage interface {
	Get(key string) (storedValue, bool)
	Put(key string, value storedValue) error
	Iterate(func(key string, value storedValue) bool)
	Close() error
	Path() string
}

// BadgerStorage implements Storage using BadgerDB
type BadgerStorage struct {
	db   *badger.DB
	path string
}

// NewBadgerStorage creates a new BadgerDB instance
func NewBadgerStorage(nodeID string) (*BadgerStorage, error) {
	// Create data directory if it doesn't exist
	dataDir := filepath.Join("data", nodeID)
	log.Printf("Initializing BadgerDB for node %s at path %s", nodeID, dataDir)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	opts := badger.DefaultOptions(dataDir)
	opts.Logger = nil // Disable default logger to reduce noise

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %v", err)
	}

	return &BadgerStorage{
		db:   db,
		path: dataDir,
	}, nil
}

func (s *BadgerStorage) Get(key string) (storedValue, bool) {
	var val storedValue
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(valBytes []byte) error {
			return json.Unmarshal(valBytes, &val)
		})
	})

	if err != nil {
		if err != badger.ErrKeyNotFound {
			log.Printf("Error getting key %s: %v", key, err)
		}
		return storedValue{}, false
	}
	return val, true
}

func (s *BadgerStorage) Put(key string, value storedValue) error {
	valBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %v", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), valBytes)
	})
}

func (s *BadgerStorage) Iterate(fn func(key string, value storedValue) bool) {
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := string(item.Key())
			
			var v storedValue
			err := item.Value(func(valBytes []byte) error {
				return json.Unmarshal(valBytes, &v)
			})
			
			if err == nil {
				if !fn(k, v) {
					break
				}
			}
		}
		return nil
	})
}

func (s *BadgerStorage) Close() error {
	return s.db.Close()
}

func (s *BadgerStorage) Path() string {
	return s.path
}
