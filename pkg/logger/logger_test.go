package logger

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"knative.dev/pkg/logging"
)

// Utility functions copied from main.go
// These is the graceful shutdown logic we want to test
type CustomHandler struct {
	wg      *sync.WaitGroup
	handler http.Handler
}

func NewCustomHandler(wg *sync.WaitGroup, handler http.Handler) *CustomHandler {
	return &CustomHandler{wg: wg, handler: handler}
}

func (h *CustomHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.wg.Add(1)
	defer h.wg.Done()
	h.handler.ServeHTTP(w, r)
}

// simple handler that logs the request
// and sleeps for 5 seconds
// to simulate a long running request
func makeLoggingHandler(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(ctx)
		select {
		case <-time.After(5 * time.Second):
			// simulate a long running request, in case ctx.Done() (srv.ShutdownServer)
			// is not called fast enough
		case <-ctx.Done():
			break
			// the context will be canceled when the server is shutting down
			// this gives an exact timing of workload simulation
		}
		logger.Infof("Received request:%s %s", r.Method, r.URL) // use our logger to test concurrency
	}
}

func syslogServer(port string, messages chan<- string, ready chan<- struct{}, shutdownSyslog <-chan struct{}) {
	// start listening on the port
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Error starting the server: %v", err)
	}

	// Notify that server is ready
	ready <- struct{}{}

	var wg sync.WaitGroup
	var connections []net.Conn
	var connectionsLock sync.Mutex

	// Shutdown logic
	go func() {
		<-shutdownSyslog
		ln.Close()
		// closing connections to force the scanner to stop
		connectionsLock.Lock()
		defer connectionsLock.Unlock()
		for _, conn := range connections {
			conn.Close()
		}
	}()

	// start accepting connections
	for {
		conn, err := ln.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "accept" {
				// Listener closed, stopping accepting connections
				break
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		// new connection, add it to the list
		connectionsLock.Lock()
		connections = append(connections, conn)
		connectionsLock.Unlock()

		// read from the connection
		// and send the messages to the message channel
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer conn.Close()

			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				text := scanner.Text()
				messages <- text
			}
		}(conn)
	}

	// no more incoming connections, wait for all connections to be closed
	wg.Wait()
	close(messages)
}

func startHTTPServer(
	ctx context.Context,
	port string,
	handler http.Handler,
	ready chan<- struct{},
	shutdownHTTP <-chan struct{},
	shutdownSyslog chan<- struct{},
) {
	var wg sync.WaitGroup
	customHandler := NewCustomHandler(&wg, handler)
	server := &http.Server{
		Addr:    ":" + port,
		Handler: customHandler,
	}

	// Start the server
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting the server: %v", err)
		}
	}()

	// Wait until the server is ready
	timeout := time.After(5 * time.Second)           // Wait up to 5 seconds for server to start
	ticker := time.NewTicker(100 * time.Millisecond) // Check every 100ms if server is ready
	defer ticker.Stop()
loop:
	for {
		select {
		case <-timeout:
			log.Fatal("Server start timeout")
			return
		case <-ticker.C:
			conn, err := net.Dial("tcp", ":"+port)
			if err == nil {
				_ = conn.Close()
				ready <- struct{}{}
				break loop
			}
		}
	}

	// Listen for shutdown signal
	go func(ctx context.Context) {
		<-shutdownHTTP

		// Start shutdown process of HTTP server
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("Server Shutdown: %v", err)
		}

		// Wait for all HTTP requests to be processed
		// Feature that we test on the prod server
		wg.Wait()

		// Notify that the HTTP server is shutdown
		close(shutdownSyslog)
	}(ctx)
}

func TestToto(t *testing.T) {
	amountOfMessages := 100 // amount of messages to send to the logger

	syslogReady := make(chan struct{})              // channel to notify when the syslog server is ready
	shutdownSyslog := make(chan struct{})           // channel to notify when to shutdown the syslog server
	messages := make(chan string, amountOfMessages) // channel to receive syslog messages
	httpReady := make(chan struct{})                // channel to notify when the http server is ready
	shutdownHTTP := make(chan struct{})             // channel to notify when to shutdown the http server

	go syslogServer("16901", messages, syslogReady, shutdownSyslog)

	<-syslogReady // Wait until the syslog server is ready

	// start the logger and http server
	logCfg := Config{
		Level:     "info",
		Component: "dagger-registry-2023-07-28",
		Protocol:  "tcp",
		Address:   "127.0.0.1:16901",
	}

	ctx, syslogger, err := NewLogger(context.Background(), &logCfg)
	if err != nil {
		panic(err)
	}

	// init the server mux
	cancelContext, cancel := context.WithCancel(ctx)

	mux := http.NewServeMux()
	handler := makeLoggingHandler(cancelContext)
	mux.HandleFunc("/", handler)

	go startHTTPServer(ctx, "8080", mux, httpReady, shutdownHTTP, shutdownSyslog)

	<-httpReady // Wait until the http server is ready

	// Send 100 requests to the http server
	// almost at the same time
	var wg sync.WaitGroup
	for i := 0; i < amountOfMessages; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := http.Get("http://localhost:8080/")
			if err != nil {
				panic(err)
			}
		}()
	}

	cancel()  // Cancel the context to stop the http server
	wg.Wait() // Wait for all 100 requests to be sent

	// Simulate shutdown signal while requests are still being processed
	// As they have a 5s sleep, they will be processed at the same time as the shutdown signal
	// This is the feature that we test on the prod server: do we lose messages?
	close(shutdownHTTP)

	// Count received messages
	count := 0
	for range messages {
		count++
	}

	require.EqualValues(t, count, amountOfMessages,
		fmt.Sprintf("Expected %d messages, got %d", amountOfMessages, count),
	)
	syslogger.Close()
}
