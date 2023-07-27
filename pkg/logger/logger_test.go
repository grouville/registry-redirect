package logger

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"testing"
	"time"

	"sync"

	"knative.dev/pkg/logging"
)

var doneLogging chan struct{}

func init() {
	doneLogging = make(chan struct{})
}

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
			select {
			case <-doneLogging:
				// Signal to stop logging and exit the function
				return
			default:
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
		}
	}()
}

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

	// Create a context with a cancel function
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up syslog client
	ctx, sw, err := NewLogger(ctx, &Config{
		Level:     "info",
		Component: "dagger-registry-2023-01-23",
		Protocol:  "tcp",
		Address:   "localhost:1514",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	// Create a channel to signal that the HTTP server is ready to receive requests.
	serverReady = make(chan struct{})

	// Start the HTTP server in a separate goroutine
	go func() {
		// Initialize an HTTP server
		srv := &http.Server{
			Addr: fmt.Sprintf(":%d", 8080),
		}

		// Handle the HTTP request and log a message
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			log := logging.FromContext(ctx)
			log.Info("request received") // using your syslog writer
		})

		// Start the HTTP server
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				t.Errorf("HTTP server error: %+s\n", err)
			}
		}()

		// Signal that the server is ready to receive requests
		close(serverReady)

		// Wait for the server to finish its work (graceful shutdown)
		<-ctx.Done()

		// Shutdown the HTTP server gracefully
		ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelShutdown()
		if err := srv.Shutdown(ctxShutdown); err != nil {
			t.Errorf("HTTP server shutdown error: %+s\n", err)
		}
	}()

	// Send multiple log entries to the HTTP server concurrently
	numLogEntries := 5
	for i := 1; i <= numLogEntries; i++ {
		go func(i int) {
			// Simulate some processing time for each log entry
			time.Sleep(100 * time.Millisecond)
			resp, err := http.Get("http://localhost:8080")
			if err != nil {
				t.Errorf("HTTP request error: %s", err)
				return
			}
			defer resp.Body.Close()
		}(i)
	}

	// // Allow some time for logs to be processed by the syslog server
	time.Sleep(1 * time.Second)

	// Wait for the HTTP server to finish its work (graceful shutdown) and process any remaining log entries
	wg.Wait()

	// Introduce a small delay to allow the HTTP server to finish processing remaining logs
	// time.Sleep(100 * time.Millisecond)

	// _ = sw
	sw.Close()

	// Wait for mock syslog server to finish its work
	<-done

	// sw.Close()
	// Ensure that all the messages have been received by the syslog server
	for i := 1; i <= numLogEntries; i++ {
		select {
		case received := <-messages:
			expectedPattern := fmt.Sprintf(`request received`)
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
}
