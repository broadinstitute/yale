package logs

import (
	"io"
	"log"
	"os"
)

var (
	Debug = log.New(chooseDebugOutput(), "[DEBUG] ", log.Ldate|log.Ltime)
	// Info Poor man's info level logger
	Info = log.New(os.Stdout, "[INFO] ", log.Ldate|log.Ltime)
	// Error Poor man's error level logger
	Error = log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime)
	// Warn Poor man's warn level logger
	Warn = log.New(os.Stdout, "[WARN] ", log.Ldate|log.Ltime)
)

func chooseDebugOutput() io.Writer {
	if os.Getenv("YALE_DEBUG_ENABLED") == "true" {
		return os.Stdout
	} else {
		return io.Discard
	}
}
