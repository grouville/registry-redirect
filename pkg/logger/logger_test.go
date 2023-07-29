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

func syslogServer(port string, messages chan<- string, ready chan<- struct{}, shutdownSyslog <-chan struct{}) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Error starting the server: %v", err)
	}
	defer ln.Close()

	// Notify that server is ready
	ready <- struct{}{}

	var wg sync.WaitGroup

	go func() {
		for {
			select {
			case <-shutdownSyslog:
				// Stop accepting new connections and return, leading to
				// the termination of the goroutine
				ln.Close()
				return
			default:
			}

			conn, err := ln.Accept()
			if err != nil {
				// If listener is closed, stop accepting connections
				if opErr, ok := err.(*net.OpError); ok && opErr.Op == "accept" {
					return
				}
				log.Printf("Error accepting connection: %v", err)
				continue
			}

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
	}()

	// Wait for all connections to be handled before returning
	wg.Wait()
}

func startHTTPServer(port string, handler http.Handler, ready chan<- struct{}, shutdownMessage <-chan struct{}) {
	var wg sync.WaitGroup
	customHandler := NewCustomHandler(&wg, handler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: customHandler,
	}

	// Notify that server is ready
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
	go func() {
		<-shutdownMessage
		log.Println("Shutting down server...")

		// Wait for all requests to be processed
		wg.Wait()

		if err := server.Shutdown(context.Background()); err != nil {
			log.Fatalf("Server Shutdown: %v", err)
		}
	}()
}

func TestToto(t *testing.T) {
	port := "8080"

	syslogReady := make(chan struct{})
	shutdownSyslog := make(chan struct{})
	messages := make(chan string, 5)

	go syslogServer(port, messages, syslogReady, shutdownSyslog)

	// Wait until the syslog server is ready
	<-syslogReady

	httpReady := make(chan struct{})
	shutdownHTTP := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// fmt.Fprint(w, "Hello, World!")
		// write to syslog, with our syslogger
	})

	go startHTTPServer(port, mux, httpReady, shutdownHTTP)

	// Wait until the http server is ready
	<-httpReady

	// Perform other actions
	// ...

	close(shutdownSyslog)
	close(shutdownHTTP)

	// Print received messages
	// test; blocking until we receive a message
	// don't know if expected behavior
	for msg := range messages {
		fmt.Println("Received:", msg)
	}
}
