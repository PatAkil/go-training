package datastore

import (
	"sync"
)

// START OMIT
type Datastorer interface {
	Put(key string, value interface{}) error
	Get(key string) (interface{}, bool, error)
	Remove(key string) error
}

// END OMIT

type SimplisticDatastore struct {
	sync.Mutex
	data map[string]interface{}
}

func NewSimplisticDatastore() Datastorer {
	return &SimplisticDatastore{
		data: map[string]interface{}{},
	}
}

func (ds *SimplisticDatastore) Put(key string, value interface{}) error {
	ds.Lock()
	defer ds.Unlock()

	ds.data[key] = value

	return nil
}

func (ds *SimplisticDatastore) Get(key string) (interface{}, bool, error) {
	ds.Lock()
	defer ds.Unlock()

	value, found := ds.data[key]

	return value, found, nil
}

func (ds *SimplisticDatastore) Remove(key string) error {
	ds.Lock()
	defer ds.Unlock()

	delete(ds.data, key)

	return nil
}
