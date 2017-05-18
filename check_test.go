package main

import "testing"
import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"
)

func TestCheck_ConcurrencyReadWrite(t *testing.T) {
	const tries = 1000
	testChecks := Checks{&Check{}, &Check{}}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	writeFn := func() {
		failed := rnd.Intn(1) == 0
		for _, check := range testChecks {
			if failed {
				check.MarkFailed("")
			} else {
				check.MarkHealthy()
			}
		}
	}

	readFn := func() {
		for _, check := range testChecks {
			check.m.RLock()
			fmt.Fprint(ioutil.Discard, check.Failed)
			check.m.RUnlock()
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(tries)
	go func() {
		for i := 0; i < tries; i++ {
			writeFn()
		}
	}()

	go func() {
		for i := 0; i < tries; i++ {
			readFn()
			wg.Done()
		}
	}()

	wg.Wait()
}
