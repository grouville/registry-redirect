// Code generated by MockGen. DO NOT EDIT.
// Source: syslogger/syslogger.go

// Package syslogger is a generated GoMock package.
package syslogger

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockSyslogWriterInterface is a mock of SyslogWriterInterface interface.
type MockSyslogWriterInterface struct {
	ctrl     *gomock.Controller
	recorder *MockSyslogWriterInterfaceMockRecorder
}

// MockSyslogWriterInterfaceMockRecorder is the mock recorder for MockSyslogWriterInterface.
type MockSyslogWriterInterfaceMockRecorder struct {
	mock *MockSyslogWriterInterface
}

// NewMockSyslogWriterInterface creates a new mock instance.
func NewMockSyslogWriterInterface(ctrl *gomock.Controller) *MockSyslogWriterInterface {
	mock := &MockSyslogWriterInterface{ctrl: ctrl}
	mock.recorder = &MockSyslogWriterInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSyslogWriterInterface) EXPECT() *MockSyslogWriterInterfaceMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockSyslogWriterInterface) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockSyslogWriterInterfaceMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockSyslogWriterInterface)(nil).Close))
}

// Write mocks base method.
func (m *MockSyslogWriterInterface) Write(p []byte) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Write", p)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Write indicates an expected call of Write.
func (mr *MockSyslogWriterInterfaceMockRecorder) Write(p interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Write", reflect.TypeOf((*MockSyslogWriterInterface)(nil).Write), p)
}
