package slingflight

import "sync"

type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

type Group struct {
	wg sync.Mutex
	m  map[string]*call
}
