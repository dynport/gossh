package gossh

import (
	"bytes"
	"strings"
)

type LogWriter struct {
	LogTo  func(n ...interface{})
	Buffer bytes.Buffer
}

func (w *LogWriter) String() (s string) {
	return w.Buffer.String()
}

func (w *LogWriter) Write(b []byte) (i int, e error) {
	if w.LogTo != nil {
		for _, s := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(s)
			if len(trimmed) > 0 {
				w.LogTo(trimmed)
			}
		}
	}
	w.Buffer.Write(b)
	return len(b), nil
}
