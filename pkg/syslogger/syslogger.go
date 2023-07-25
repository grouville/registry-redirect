package syslogger

import (
	"fmt"
	"log/syslog"
	"sync"
)

// interface created for mocking `mock_syslogger.go`
// used in logger/logger_test.go
type SyslogWriterInterface interface {
	Write(p []byte) (n int, err error)
	Close() error
}

var _ SyslogWriterInterface = &SyslogWriter{}

type SyslogWriter struct {
	writer *syslog.Writer
	mutex  sync.Mutex
}

func NewSyslogWriter(priority syslog.Priority, tag string) (*SyslogWriter, error) {
	w, err := syslog.Dial("udp", "0.0.0.0:514", priority, tag)
	if err != nil {
		return nil, err
	}
	return &SyslogWriter{writer: w}, nil
}

// Send p to syslog and return the number of bytes written and any error encountered
func (sw *SyslogWriter) Write(p []byte) (n int, err error) {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	if sw.writer == nil {
		return 0, fmt.Errorf("syslog writer is closed")
	}

	len, err := sw.writer.Write(p)
	if err != nil {
		return 0, err
	}
	return len, nil
}

// idempotent close
// if the writer is already closed, this is a no-op
func (sw *SyslogWriter) Close() error {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	if sw.writer != nil {
		err := sw.writer.Close()
		sw.writer = nil
		return err
	}
	return nil
}
