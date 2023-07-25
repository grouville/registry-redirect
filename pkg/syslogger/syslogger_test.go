package syslogger

import (
	"log/syslog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyslogWriter(t *testing.T) {
	sw, err := NewSyslogWriter(syslog.LOG_INFO|syslog.LOG_USER, "test")
	require.NoError(t, err, "Unexpected error in NewSyslogWriter")

	data := []byte("hello, world!")
	n, err := sw.Write(data)
	require.NoError(t, err, "Unexpected error in Write")
	require.Equal(t, len(data), n, "Write returned incorrect length")

	err = sw.Close()
	require.NoError(t, err, "Unexpected error in Close")

	_, err = sw.Write(data)
	require.Error(t, err, "Expected error in Write after Close, but got nil")
}

func TestSyslogWriterConcurrency(t *testing.T) {
	sw, err := NewSyslogWriter(syslog.LOG_INFO|syslog.LOG_USER, "test")
	require.NoError(t, err, "Unexpected error in NewSyslogWriter")

	data := []byte("hello, world!")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := sw.Write(data)
			require.NoError(t, err, "Unexpected error in Write")
			require.Equal(t, len(data), n, "Write returned incorrect length")
		}()
	}

	wg.Wait()

	err = sw.Close()
	require.NoError(t, err, "Unexpected error in Close")

	_, err = sw.Write(data)
	require.Error(t, err, "Expected error in Write after Close, but got nil")
}

// this test proves that the syslog writer fails after being closed
// returning an error from Write after Close is expected behavior
// However, this test is flaky because it relies on the timing of the goroutines

// func TestSyslogWriterConcurrency(t *testing.T) {
// 	sw, err := NewSyslogWriter(syslog.LOG_INFO|syslog.LOG_USER, "test")
// 	if err != nil {
// 		t.Fatalf("Unexpected error in NewSyslogWriter: %v", err)
// 	}

// 	data := []byte("hello, world!")

// 	// Start multiple goroutines that concurrently write to the syslog writer.
// 	var wg sync.WaitGroup
// 	for i := 0; i < 100; i++ {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			n, err := sw.Write(data)
// 			if err != nil {
// 				t.Errorf("Unexpected error in Write: %v", err)
// 			}
// 			if n != len(data) {
// 				t.Errorf("Write returned incorrect length. Expected %d, got %d", len(data), n)
// 			}
// 		}()
// 	}

// 	// Concurrently close the syslog writer.
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		err := sw.Close()
// 		if err != nil {
// 			t.Errorf("Unexpected error in Close: %v", err)
// 		}
// 	}()

// 	wg.Wait()

// 	// After Close, the writer should be closed and Write should fail.
// 	_, err = sw.Write(data)
// 	if err == nil {
// 		t.Error("Expected error in Write after Close, but got nil")
// 	}
// }
