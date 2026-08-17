package main

import (
	"testing"
	"time"
)

func TestDefaultStartupIsBounded(t *testing.T) {
	result := make(chan error, 1)
	go func() {
		result <- runArgs(nil, "127.0.0.1:0")
	}()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runArgs() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("default startup did not terminate")
	}
}
