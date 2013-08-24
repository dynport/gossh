package gossh

import (
	"fmt"
	"time"
)

type Result struct {
	StdoutBuffer, StderrBuffer *LogWriter
	Runtime                    time.Duration
	Error                      error
	ExitStatus                 int
}

func (r *Result) Stdout() string {
	return r.StdoutBuffer.String()
}

func (r *Result) Stderr() string {
	return r.StderrBuffer.String()
}

func (r *Result) String() (out string) {
	m := map[string]string{
		"stdout":  fmt.Sprintf("%d bytes", len(r.StdoutBuffer.String())),
		"stderr":  fmt.Sprintf("%d bytes", len(r.StderrBuffer.String())),
		"runtime": fmt.Sprintf("%0.6f", r.Runtime.Seconds()),
		"status":  fmt.Sprintf("%d", r.ExitStatus),
	}
	return fmt.Sprintf("%+v", m)
}

func (self *Result) Success() bool {
	return self.ExitStatus == 0
}
