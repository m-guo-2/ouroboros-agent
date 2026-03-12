package logger

import (
	"fmt"
	"os"
	"sync"
)

const asyncBufferSize = 4096

type writeMsg struct {
	kind      byte // 'a' = append, 'l' = llm-io
	level     Level
	entry     map[string]any
	ref       string
	traceID   string
	iteration int
	data      []byte
}

// asyncWriter wraps a LogWriter and dispatches writes through a buffered channel
// so the calling goroutine never blocks on I/O.
type asyncWriter struct {
	ch   chan writeMsg
	dest LogWriter
	wg   sync.WaitGroup
}

func newAsyncWriter(dest LogWriter) *asyncWriter {
	aw := &asyncWriter{
		ch:   make(chan writeMsg, asyncBufferSize),
		dest: dest,
	}
	aw.wg.Add(1)
	go aw.loop()
	return aw
}

func (aw *asyncWriter) loop() {
	defer aw.wg.Done()
	for msg := range aw.ch {
		switch msg.kind {
		case 'a':
			if err := aw.dest.Append(msg.level, msg.entry); err != nil {
				fmt.Fprintf(os.Stderr, "logger: async append error: %v\n", err)
			}
		case 'l':
			if err := aw.dest.WriteLLMIO(msg.ref, msg.traceID, msg.iteration, msg.data); err != nil {
				fmt.Fprintf(os.Stderr, "logger: async write-llmio error: %v\n", err)
			}
		}
	}
}

func (aw *asyncWriter) Append(level Level, entry map[string]any) {
	select {
	case aw.ch <- writeMsg{kind: 'a', level: level, entry: entry}:
	default:
		fmt.Fprintf(os.Stderr, "logger: async buffer full, dropping log entry\n")
	}
}

func (aw *asyncWriter) WriteLLMIO(ref, traceID string, iteration int, data []byte) {
	select {
	case aw.ch <- writeMsg{kind: 'l', ref: ref, traceID: traceID, iteration: iteration, data: data}:
	default:
		fmt.Fprintf(os.Stderr, "logger: async buffer full, dropping llm-io entry\n")
	}
}

func (aw *asyncWriter) Flush() {
	close(aw.ch)
	aw.wg.Wait()
	aw.dest.Close()
}

// multiWriter fans out writes to multiple LogWriter backends.
type multiWriter struct {
	backends []LogWriter
}

func newMultiWriter(backends ...LogWriter) *multiWriter {
	return &multiWriter{backends: backends}
}

func (mw *multiWriter) Append(level Level, entry map[string]any) error {
	var firstErr error
	for _, b := range mw.backends {
		if err := b.Append(level, entry); err != nil && firstErr == nil {
			firstErr = err
			fmt.Fprintf(os.Stderr, "logger: multi-writer append error: %v\n", err)
		}
	}
	return firstErr
}

func (mw *multiWriter) WriteLLMIO(ref, traceID string, iteration int, data []byte) error {
	var firstErr error
	for _, b := range mw.backends {
		if err := b.WriteLLMIO(ref, traceID, iteration, data); err != nil && firstErr == nil {
			firstErr = err
			fmt.Fprintf(os.Stderr, "logger: multi-writer llm-io error: %v\n", err)
		}
	}
	return firstErr
}

func (mw *multiWriter) Close() error {
	var firstErr error
	for _, b := range mw.backends {
		if err := b.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
