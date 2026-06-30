package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

type monitorHandler struct {
	traces *tracesHandler
}

func (h *monitorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/monitor"), "/")
	switch {
	case path == "overview":
		h.overview(w, r)
	case path == "executions":
		h.executions(w, r)
	case strings.HasPrefix(path, "executions/"):
		h.executionDetail(w, strings.TrimPrefix(path, "executions/"))
	default:
		apiErr(w, http.StatusNotFound, "not found")
	}
}

func monitorFilter(r *http.Request) storage.ExecutionFilter {
	q := r.URL.Query()
	return storage.ExecutionFilter{
		From:    parseInt64(q.Get("from"), 0),
		To:      parseInt64(q.Get("to"), 0),
		Before:  parseInt64(q.Get("before"), 0),
		AgentID: q.Get("agentId"),
		Channel: q.Get("channel"),
		Status:  q.Get("status"),
		Outcome: q.Get("outcome"),
		Search:  strings.TrimSpace(q.Get("search")),
		Limit:   parseLimit(q.Get("limit"), 50, 200),
	}
}

func (h *monitorHandler) overview(w http.ResponseWriter, r *http.Request) {
	filter := monitorFilter(r)
	overview, err := storage.GetExecutionOverview(filter)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to load monitor overview")
		return
	}
	bucketMs := int64(60 * 60 * 1000)
	if filter.To-filter.From > 48*60*60*1000 {
		bucketMs = 24 * 60 * 60 * 1000
	}
	trend, err := storage.ListExecutionTrend(filter, bucketMs)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to load monitor trend")
		return
	}
	failures, err := storage.ListFailureBreakdown(filter)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to load failure breakdown")
		return
	}
	ok(w, map[string]any{
		"summary":          overview,
		"trend":            trend,
		"failureBreakdown": failures,
	})
}

func (h *monitorHandler) executions(w http.ResponseWriter, r *http.Request) {
	items, err := storage.ListExecutions(monitorFilter(r))
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to list executions")
		return
	}
	ok(w, items)
}

func (h *monitorHandler) executionDetail(w http.ResponseWriter, id string) {
	if id == "" || strings.Contains(id, "/") {
		apiErr(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	execution, err := storage.GetExecution(id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to load execution")
		return
	}
	if execution == nil {
		apiErr(w, http.StatusNotFound, "execution not found")
		return
	}
	requests, err := storage.ListExecutionRequests(id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to load execution requests")
		return
	}
	events, err := storage.ListLifecycleEvents(execution.SessionID)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to load execution lifecycle")
		return
	}
	filteredEvents := make([]storage.MessageLifecycleEvent, 0)
	for _, event := range events {
		if event.TraceID == id {
			filteredEvents = append(filteredEvents, event)
		}
	}

	var trace *executionTrace
	if h.traces != nil {
		trace = h.traces.buildTrace(id)
	}
	ok(w, map[string]any{
		"execution": execution,
		"requests":  requests,
		"lifecycle": filteredEvents,
		"trace":     trace,
	})
}
