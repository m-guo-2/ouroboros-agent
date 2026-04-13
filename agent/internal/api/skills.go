package api

import (
	"net/http"
	"strings"

	"agent/internal/github"
	"agent/internal/storage"
)

// GET/POST /api/skills
func handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills, err := storage.GetAllSkills()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if skills == nil {
			skills = []storage.SkillRecord{}
		}
		ok(w, skills)
	case http.MethodPost:
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		name, _ := body["name"].(string)
		if name == "" {
			apiErr(w, http.StatusBadRequest, "name is required")
			return
		}
		rec := storage.SkillRecord{
			Name:    name,
			Enabled: true,
		}
		if v, ok := body["id"].(string); ok {
			rec.ID = v
		}
		if v, ok := body["description"].(string); ok {
			rec.Description = v
		}
		if v, ok := body["enabled"].(bool); ok {
			rec.Enabled = v
		}
		if v, ok := body["readme"].(string); ok {
			rec.Readme = v
		}
		created, err := storage.CreateSkill(rec)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": created})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// POST /api/skills/refresh — force re-sync from GitHub
func handleSkillsRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := storage.RefreshSkills(); err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	skills, _ := storage.GetAllSkills()
	ok(w, map[string]interface{}{"refreshed": len(skills)})
}

// GET/PUT/DELETE /api/skills/{id}[/context]
func handleSkillsWithID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	if id == "" {
		apiErr(w, http.StatusBadRequest, "missing skill id")
		return
	}
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	// GET /api/skills/{agentId}/context — compile skill context for an agent
	if sub == "context" && r.Method == http.MethodGet {
		var skillIDs []string
		agentCfg, _ := storage.GetAgentConfig(id)
		if agentCfg != nil {
			skillIDs = agentCfg.Skills
		}
		ctx, err := storage.GetSkillsContext(skillIDs)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, ctx)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s, err := storage.GetSkillByID(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s == nil {
			apiErr(w, http.StatusNotFound, "skill not found")
			return
		}
		ok(w, s)
	case http.MethodPut:
		existing, err := storage.GetSkillByID(id)
		if err != nil || existing == nil {
			apiErr(w, http.StatusNotFound, "skill not found")
			return
		}
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		updated, err := storage.UpdateSkill(id, body)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, updated)
	case http.MethodDelete:
		deleted, err := storage.DeleteSkill(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			apiErr(w, http.StatusNotFound, "skill not found")
			return
		}
		ok(w, map[string]bool{"deleted": true})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// POST /api/skills/import/browse — browse skills from an external public GitHub repo
func handleSkillsImportBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeBody(r, &body); err != nil || body.URL == "" {
		apiErr(w, http.StatusBadRequest, "url is required")
		return
	}

	src, err := github.ParseGitHubURL(body.URL)
	if err != nil {
		apiErr(w, http.StatusBadRequest, err.Error())
		return
	}

	client := github.NewPublicClient(src.Owner, src.Repo, src.Branch)
	skills, err := github.BrowseSkills(client, src.Path)
	if err != nil {
		if github.IsNotFound(err) {
			apiErr(w, http.StatusNotFound, "path not found in repository")
			return
		}
		apiErr(w, http.StatusBadGateway, err.Error())
		return
	}

	localSkills, _ := storage.GetAllSkills()
	localIDs := make(map[string]bool, len(localSkills))
	for _, s := range localSkills {
		localIDs[s.ID] = true
	}
	for i := range skills {
		if localIDs[skills[i].ID] {
			skills[i].Exists = true
		}
	}

	ok(w, map[string]interface{}{
		"repo":   src.Owner + "/" + src.Repo,
		"branch": src.Branch,
		"path":   src.Path,
		"skills": skills,
	})
}

// POST /api/skills/import — import selected skills from an external public GitHub repo
func handleSkillsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Repo      string   `json:"repo"`
		Branch    string   `json:"branch"`
		Path      string   `json:"path"`
		Skills    []string `json:"skills"`
		Overwrite bool     `json:"overwrite"`
	}
	if err := decodeBody(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Repo == "" || len(body.Skills) == 0 {
		apiErr(w, http.StatusBadRequest, "repo and skills are required")
		return
	}
	if body.Branch == "" {
		body.Branch = "main"
	}

	parts := strings.SplitN(body.Repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		apiErr(w, http.StatusBadRequest, "repo must be owner/repo format")
		return
	}

	store := github.DefaultStore
	if store == nil {
		apiErr(w, http.StatusInternalServerError, "skill store not initialized")
		return
	}

	srcClient := github.NewPublicClient(parts[0], parts[1], body.Branch)
	results := github.ImportSkills(srcClient, body.Path, store, body.Skills, body.Overwrite)

	if err := store.Refresh(); err != nil {
		apiErr(w, http.StatusInternalServerError, "import succeeded but refresh failed: "+err.Error())
		return
	}

	imported := 0
	for _, r := range results {
		if r.OK {
			imported++
		}
	}

	ok(w, map[string]interface{}{
		"imported": imported,
		"total":    len(body.Skills),
		"results":  results,
	})
}
