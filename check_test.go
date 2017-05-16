package main

import "testing"
import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"time"
)

func TestCheck_ReadWrite(t *testing.T) {
	const tries = 1000
	testChecks := []*Check{&Check{}, &Check{}}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	writeFn := func() {
		failed := rnd.Intn(1) == 0
		for _, check := range testChecks {
			check.m.Lock()
			check.Failed = failed
			check.m.Unlock()
		}
	}

	readFn := func() {
		for _, check := range testChecks {
			check.m.RLock()
			fmt.Fprint(ioutil.Discard, check.Failed)
			check.m.RUnlock()
		}
	}

	go func() {
		for i := 0; i < tries; i++ {
			writeFn()
		}
	}()

	go func() {
		for i := 0; i < tries; i++ {
			readFn()
		}
	}()
}
