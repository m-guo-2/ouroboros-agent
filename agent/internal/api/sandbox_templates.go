package api

import (
	"net/http"

	"agent/internal/sandbox"
)

func handleSandboxTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ok(w, sandbox.SandboxTemplates())
}

func validateSandboxTemplateID(id string) bool {
	_, ok := sandbox.ResolveSandboxTemplate(id)
	return ok
}
