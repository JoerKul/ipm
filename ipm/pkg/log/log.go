package log

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

var logger *logrus.Logger

func Init(level, logFile string) error {
	logger = logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Standardausgabe
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	// Logfile, falls angegeben
	if logFile != "" {
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		writers = append(writers, file)
	}

	logger.SetOutput(io.MultiWriter(writers...))

	// Wenn nur --log-file gesetzt ist, aber kein --log-level, nutze Info-Level
	if logFile != "" && level == "" {
		logger.SetLevel(logrus.InfoLevel)
		return nil
	}

	// Ohne Logfile und Level keine Logs
	if level == "" {
		logger.SetLevel(logrus.PanicLevel)
		return nil
	}

	// Expliziter Log-Level
	switch level {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel) // Fallback auf info
	}
	return nil
}

func Info(msg string, fields ...map[string]interface{}) {
	entry := logger.WithFields(mergeFields(fields...))
	entry.Info(msg)
}

func Debug(msg string, fields ...map[string]interface{}) {
	entry := logger.WithFields(mergeFields(fields...))
	entry.Debug(msg)
}

func Warn(msg string, fields ...map[string]interface{}) {
	entry := logger.WithFields(mergeFields(fields...))
	entry.Warn(msg)
}

func Error(msg string, err error, fields ...map[string]interface{}) {
	entry := logger.WithFields(mergeFields(fields...))
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Error(msg)
}

func mergeFields(fields ...map[string]interface{}) logrus.Fields {
	result := make(logrus.Fields)
	for _, f := range fields {
		for k, v := range f {
			result[k] = v
		}
	}
	return result
}