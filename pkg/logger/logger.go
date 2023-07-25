package logger

import (
	"context"
	"encoding/json"
	"log/syslog"

	"github.com/chainguard-dev/registry-redirect/pkg/syslogger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"knative.dev/pkg/logging"
)

type MyLoggingConfig struct {
	Level string
}

// Convert log level string to syslog priority
func (m *MyLoggingConfig) levelToSyslogPriority(level string) syslog.Priority {
	switch level {
	case "debug":
		return syslog.LOG_DEBUG
	case "info":
		return syslog.LOG_INFO
	case "warn":
		return syslog.LOG_WARNING
	case "error":
		return syslog.LOG_ERR
	case "dpanic", "panic", "fatal":
		return syslog.LOG_CRIT
	default: // Default to info level if no level has been configured
		return syslog.LOG_INFO
	}
}

// Convert custom logging config to Knative logging config
func (m *MyLoggingConfig) customConfigToKnativeConfig(logCfg *MyLoggingConfig) *logging.Config {
	var knativeCfg logging.Config
	bytes, _ := json.Marshal(logCfg)
	_ = json.Unmarshal(bytes, &knativeCfg)
	return &knativeCfg
}

func SetupLogging(ctx context.Context, logCfg *MyLoggingConfig, component string) (context.Context, *syslogger.SyslogWriter, error) {
	syslogPriority := logCfg.levelToSyslogPriority(logCfg.Level) | syslog.LOG_LOCAL0

	// Setup syslog
	syslogWriter, err := syslogger.NewSyslogWriter(syslogPriority, component)
	if err != nil {
		return nil, nil, err
	}

	// Convert custom logging config to Knative logging config
	knativeCfg := logCfg.customConfigToKnativeConfig(logCfg)

	// Setup zap logger
	logger, atomicLevel := logging.NewLoggerFromConfig(knativeCfg, component)

	// Combine syslog and zap logger
	baseLogger := logger.Desugar()
	baseLogger = baseLogger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(syslogWriter),
			atomicLevel,
		))
	}))

	// Update context with logger
	ctx = logging.WithLogger(ctx, baseLogger.Sugar())

	return ctx, syslogWriter, nil
}
