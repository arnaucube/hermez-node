package log

import (
	"fmt"
	"strings"
	"time"

	"github.com/hermeznetwork/tracerr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger

func init() {
	// default level: debug
	Init("debug", "")
}

// Init the logger with defined level. errorsPath defines the file where to store the errors, if set to "" will not store errors.
func Init(levelStr, logPath string) {
	var level zap.AtomicLevel
	err := level.UnmarshalText([]byte(levelStr))
	if err != nil {
		panic(fmt.Errorf("Error on setting log level: %s", err))
	}
	outputPaths := []string{"stdout"}
	if logPath != "" {
		log.Infof("log file: %s", logPath)
		outputPaths = append(outputPaths, logPath)
	}

	cfg := zap.Config{
		Level:            level,
		Encoding:         "console",
		OutputPaths:      outputPaths,
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey: "message",

			LevelKey:    "level",
			EncodeLevel: zapcore.CapitalColorLevelEncoder,

			TimeKey: "timestamp",
			EncodeTime: func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
				encoder.AppendString(ts.Local().Format(time.RFC3339))
			},
			EncodeDuration: zapcore.SecondsDurationEncoder,

			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,

			// StacktraceKey: "stacktrace",
			StacktraceKey: "",
			LineEnding:    zapcore.DefaultLineEnding,
		},
	}

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	//nolint:errcheck
	defer logger.Sync()
	withOptions := logger.WithOptions(zap.AddCallerSkip(1))
	log = withOptions.Sugar()

	log.Infof("log level: %s", level)
}

func sprintStackTrace(st []tracerr.Frame) string {
	builder := strings.Builder{}
	for _, f := range st {
		builder.WriteString(fmt.Sprintf("\n%s:%d %s()", f.Path, f.Line, f.Func))
	}
	builder.WriteString("\n")
	return builder.String()
}

// appendStackTraceMaybeArgs will append the stacktrace to the args if one of them
// is a tracerr.Error
func appendStackTraceMaybeArgs(args []interface{}) []interface{} {
	for i := range args {
		if err, ok := args[i].(tracerr.Error); ok {
			st := err.StackTrace()
			return append(args, sprintStackTrace(st))
		}
	}
	return args
}

// Debug calls log.Debug
func Debug(args ...interface{}) {
	log.Debug(args...)
}

// Info calls log.Info
func Info(args ...interface{}) {
	log.Info(args...)
}

// Warn calls log.Warn
func Warn(args ...interface{}) {
	args = appendStackTraceMaybeArgs(args)
	log.Warn(args...)
}

// Error calls log.Error
func Error(args ...interface{}) {
	args = appendStackTraceMaybeArgs(args)
	log.Error(args...)
}

// Fatal calls log.Fatal
func Fatal(args ...interface{}) {
	args = appendStackTraceMaybeArgs(args)
	log.Fatal(args...)
}

// Debugf calls log.Debugf
func Debugf(template string, args ...interface{}) {
	log.Debugf(template, args...)
}

// Infof calls log.Infof
func Infof(template string, args ...interface{}) {
	log.Infof(template, args...)
}

// Warnf calls log.Warnf
func Warnf(template string, args ...interface{}) {
	log.Warnf(template, args...)
}

// Fatalf calls log.Warnf
func Fatalf(template string, args ...interface{}) {
	log.Fatalf(template, args...)
}

// Errorf calls log.Errorf and stores the error message into the ErrorFile
func Errorf(template string, args ...interface{}) {
	log.Errorf(template, args...)
}

// appendStackTraceMaybeKV will append the stacktrace to the KV if one of them
// is a tracerr.Error
func appendStackTraceMaybeKV(msg string, kv []interface{}) string {
	for i := range kv {
		if i%2 == 0 {
			continue
		}
		if err, ok := kv[i].(tracerr.Error); ok {
			st := err.StackTrace()
			return fmt.Sprintf("%v: %v%v\n", msg, err, sprintStackTrace(st))
		}
	}
	return msg
}

// Debugw calls log.Debugw
func Debugw(template string, kv ...interface{}) {
	log.Debugw(template, kv...)
}

// Infow calls log.Infow
func Infow(template string, kv ...interface{}) {
	log.Infow(template, kv...)
}

// Warnw calls log.Warnw
func Warnw(template string, kv ...interface{}) {
	template = appendStackTraceMaybeKV(template, kv)
	log.Warnw(template, kv...)
}

// Errorw calls log.Errorw
func Errorw(template string, kv ...interface{}) {
	template = appendStackTraceMaybeKV(template, kv)
	log.Errorw(template, kv...)
}

// Fatalw calls log.Fatalw
func Fatalw(template string, kv ...interface{}) {
	template = appendStackTraceMaybeKV(template, kv)
	log.Fatalw(template, kv...)
}
