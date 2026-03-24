package api

import (
	"net/http"
	"strings"

	"agent/internal/subagent"
)

func handleSubagentJobs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/subagent-jobs/")
	jobID := strings.TrimRight(path, "/")
	if jobID == "" {
		apiErr(w, http.StatusBadRequest, "missing job id")
		return
	}
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	mgr := subagent.DefaultManager()
	job, found := mgr.Get(jobID)
	if !found {
		apiErr(w, http.StatusNotFound, "subagent job not found")
		return
	}

	events, _ := mgr.ReadEvents(jobID)

	type jobDetail struct {
		*subagent.Job
		Events []map[string]interface{} `json:"events"`
	}
	ok(w, jobDetail{Job: job, Events: events})
}

func getSessionSubagentJobs(w http.ResponseWriter, _ *http.Request, sessionID string) {
	mgr := subagent.DefaultManager()
	jobs := mgr.ListBySession(sessionID)

	type jobSummary struct {
		ID            string            `json:"id"`
		Name          string            `json:"name"`
		Profile       string            `json:"profile"`
		Status        subagent.JobStatus `json:"status"`
		SubTraceID    string            `json:"subTraceId"`
		ParentTraceID string            `json:"parentTraceId"`
		CreatedAt     int64             `json:"createdAt"`
		UpdatedAt     int64             `json:"updatedAt"`
		ImpactCount   int               `json:"impactCount"`
		Task          string            `json:"task"`
	}

	result := make([]jobSummary, 0, len(jobs))
	for _, j := range jobs {
		task := j.Task
		if len(task) > 200 {
			task = task[:200] + "..."
		}
		result = append(result, jobSummary{
			ID:            j.ID,
			Name:          j.Name,
			Profile:       j.Profile,
			Status:        j.Status,
			SubTraceID:    j.SubTraceID,
			ParentTraceID: j.ParentTraceID,
			CreatedAt:     j.CreatedAt,
			UpdatedAt:     j.UpdatedAt,
			ImpactCount:   len(j.Impacts),
			Task:          task,
		})
	}
	ok(w, result)
}
