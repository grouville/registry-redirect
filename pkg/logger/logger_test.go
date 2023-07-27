package logger

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"testing"
	"time"

	"sync"

	"knative.dev/pkg/logging"
)

func startMockSyslogServer(t *testing.T, messages chan string, done chan bool, wg *sync.WaitGroup, serverReady chan struct{}) {
	listener, err := net.Listen("tcp", "localhost:1514")
	if err != nil {
		t.Errorf("Failed to set up mock syslog server: %s", err)
	}

	// Signal that server is ready for connections
	close(serverReady)
	wg.Done()

	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				t.Errorf("Error accepting connection: %s", err)
				done <- true
				return
			}

			buffer := make([]byte, 1024)
			for {
				// Set a deadline for the Read operation
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				n, err := conn.Read(buffer)
				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						// If the deadline is reached, close the connection and break the loop
						conn.Close()
						break
					}
					if err == io.EOF {
						done <- true
						return
					}
					t.Errorf("Error reading from connection: %s", err)
					return
				}
				// Send received log message to messages channel
				messages <- string(buffer[:n])
				// Reset the deadline here
				conn.SetDeadline(time.Time{})
			}
		}
	}()
}

// func serverTest(ctx context.Context, logger *zap.SugaredLogger, wg *sync.WaitGroup) (*http.Server, error) {
// 	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
// 		logger.Info("request received") // using your syslog writer
// 	})

// 	port := "8080"
// 	logger.Info("http server starting...")
// 	srv := &http.Server{
// 		Addr:        fmt.Sprintf(":%s", port),
// 		Handler:     nil,
// 		BaseContext: func(_ net.Listener) context.Context { return ctx },
// 	}

// 	go func() {
// 		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
// 			logger.Errorf("listen:%+s\n", err)
// 		}
// 		wg.Done()
// 	}()

// 	logger.Infof("http server listening on port: %s", port)

// 	return srv, nil
// }

func TestServer(t *testing.T) {
	// Initialize mock syslog server variables
	messages := make(chan string, 100) // Buffer of 100 messages
	done := make(chan bool)
	serverReady := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1) // We'll wait for one event (server readiness)

	// Start the mock syslog server
	go startMockSyslogServer(t, messages, done, &wg, serverReady)

	// Wait for mock server to be ready
	select {
	case <-serverReady:
		t.Log("Mock server is ready")
	case <-time.After(time.Second * 5):
		t.Fatal("timeout waiting for mock server to be ready")
	}

	// Set up syslog client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure all paths cancel the context to prevent context leak

	ctx, sw, err := NewLogger(ctx, &Config{
		Level:     "info",
		Component: "dagger-registry-2023-01-23",
		Protocol:  "tcp",
		Address:   "localhost:1514",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer sw.Close()

	log := logging.FromContext(ctx)

	// Send multiple log entries
	numLogEntries := 5
	for i := 1; i <= numLogEntries; i++ {
		log.Infof("This is log entry %d", i)
	}

	// Wait for all the messages to arrive at the mock server
	for i := 1; i <= numLogEntries; i++ {
		select {
		case received := <-messages:
			expectedPattern := fmt.Sprintf(`This is log entry %d`, i)
			matches, err := regexp.MatchString(expectedPattern, received)
			if err != nil {
				t.Errorf("Error matching log message: %s", err)
			}
			if !matches {
				t.Errorf("Unexpected log message: %s", received)
			}

		case <-time.After(time.Second * 5):
			t.Errorf("Timeout waiting for log message %d", i)
		}
	}

	// Close the logger, triggering a graceful shutdown
	sw.Close()

	// Wait for mock syslog server to finish its work
	<-done
}
