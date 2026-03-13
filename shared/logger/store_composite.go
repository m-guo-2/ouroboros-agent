package logger

// CompositeReader tries the primary reader (SQLite) first.
// When the primary returns empty results, it falls back to the secondary reader
// (JSONL files) so that historical data remains accessible.
type CompositeReader struct {
	primary   LogReader
	secondary LogReader
}

func NewCompositeReader(primary, secondary LogReader) *CompositeReader {
	return &CompositeReader{primary: primary, secondary: secondary}
}

func (c *CompositeReader) ListTraces(filter TraceFilter) ([]TraceSummary, error) {
	result, err := c.primary.ListTraces(filter)
	if err == nil && len(result) > 0 {
		return result, nil
	}
	return c.secondary.ListTraces(filter)
}

func (c *CompositeReader) ReadTraceEvents(traceID string) ([]TraceEvent, error) {
	events, err := c.primary.ReadTraceEvents(traceID)
	if err == nil && len(events) > 0 {
		return events, nil
	}
	return c.secondary.ReadTraceEvents(traceID)
}

func (c *CompositeReader) ReadLLMIO(ref string) ([]byte, error) {
	data, err := c.primary.ReadLLMIO(ref)
	if err == nil {
		return data, nil
	}
	return c.secondary.ReadLLMIO(ref)
}

func (c *CompositeReader) ListLLMIORefs(traceID string) ([]string, error) {
	refs, err := c.primary.ListLLMIORefs(traceID)
	if err == nil && len(refs) > 0 {
		return refs, nil
	}
	return c.secondary.ListLLMIORefs(traceID)
}
