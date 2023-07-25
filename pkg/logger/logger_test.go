package logger

import (
	"context"
	"fmt"
	"testing"

	"github.com/chainguard-dev/registry-redirect/pkg/syslogger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"knative.dev/pkg/logging"
)

func TestSetupLogging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSyslogWriter := syslogger.NewMockSyslogWriterInterface(ctrl)

	logCfg := &MyLoggingConfig{
		Level: "debug",
	}

	// Mock the expected calls below
	mockSyslogWriter.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes()
	mockSyslogWriter.EXPECT().Close().Return(nil)
	mockSyslogWriter.EXPECT().Close().Return(fmt.Errorf("syslog writer is closed")).AnyTimes()

	ctx := context.Background()
	newCtx, _, err := SetupLogging(ctx, logCfg, "testComponent")

	require.NoError(t, err)

	logger := logging.FromContext(newCtx)
	require.NotNil(t, logger)

	logger.Debug("Test message")

	// Close the syslog writer and assert no error
	err = mockSyslogWriter.Close()
	require.NoError(t, err)

	// Subsequent Close calls should return an error
	err = mockSyslogWriter.Close()
	require.Error(t, err)
	require.Equal(t, "syslog writer is closed", err.Error())
}
