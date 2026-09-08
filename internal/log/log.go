package log

import (
	"io"
	"log"
	"os"
)

var logger = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)

func Init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func SetOutput(w io.Writer) {
	logger.SetOutput(w)
}

func Printf(format string, args ...any) {
	logger.Printf(format, args...)
}

func Debugf(format string, args ...any) {
	if os.Getenv("HASHUP_DEBUG") != "" {
		logger.Printf(format, args...)
	}
}

func Debug(args ...any) {
	if os.Getenv("HASHUP_DEBUG") != "" {
		logger.Println(args...)
	}
}

func Fatal(args ...any) {
	logger.Fatal(args...)
}

func Fatalf(format string, args ...any) {
	logger.Fatalf(format, args...)
}

func Errorf(format string, args ...any) {
	logger.Printf(format, args...)
}
