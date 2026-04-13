package github

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// GitHubSource holds the parsed components of a GitHub URL.
type GitHubSource struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

// ParseGitHubURL extracts owner, repo, branch, and path from various GitHub URL formats:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo/tree/branch/path/to/dir
//   - https://github.com/owner/repo/blob/branch/path/to/file
//   - owner/repo (shorthand)
//   - owner/repo/path/to/dir (shorthand with path, branch defaults to main)
func ParseGitHubURL(raw string) (GitHubSource, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return GitHubSource{}, fmt.Errorf("empty URL")
	}

	// Full URL: starts with http:// or https://
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return parseFullURL(raw)
	}

	// Shorthand: owner/repo or owner/repo/path
	return parseShorthand(raw)
}

func parseFullURL(raw string) (GitHubSource, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return GitHubSource{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host != "github.com" && u.Host != "www.github.com" {
		return GitHubSource{}, fmt.Errorf("not a github.com URL: %s", u.Host)
	}

	// Path: /owner/repo[/tree|blob/branch/path...]
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return GitHubSource{}, fmt.Errorf("URL must contain owner/repo: %s", raw)
	}

	src := GitHubSource{
		Owner:  segments[0],
		Repo:   strings.TrimSuffix(segments[1], ".git"),
		Branch: "main",
	}

	// /owner/repo only
	if len(segments) == 2 {
		return src, nil
	}

	// /owner/repo/tree/branch/... or /owner/repo/blob/branch/...
	verb := segments[2]
	if verb != "tree" && verb != "blob" {
		return GitHubSource{}, fmt.Errorf("unexpected URL segment %q (expected tree or blob): %s", verb, raw)
	}
	if len(segments) < 4 {
		return GitHubSource{}, fmt.Errorf("URL missing branch after /%s/: %s", verb, raw)
	}

	src.Branch = segments[3]
	subPath := strings.Join(segments[4:], "/")

	// For blob URLs pointing to a file, use the parent directory
	if verb == "blob" && subPath != "" {
		if idx := strings.LastIndex(subPath, "/"); idx >= 0 {
			subPath = subPath[:idx]
		} else {
			subPath = ""
		}
	}

	src.Path = subPath
	return src, nil
}

func parseShorthand(raw string) (GitHubSource, error) {
	parts := strings.SplitN(raw, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return GitHubSource{}, fmt.Errorf("shorthand must be owner/repo[/path]: %s", raw)
	}

	src := GitHubSource{
		Owner:  parts[0],
		Repo:   parts[1],
		Branch: "main",
	}
	if len(parts) == 3 && parts[2] != "" {
		src.Path = parts[2]
	}
	return src, nil
}

// NewPublicClient creates a Client for reading public GitHub repositories
// without authentication.
func NewPublicClient(owner, repo, branch string) *Client {
	return &Client{
		owner:  owner,
		repo:   repo,
		branch: branch,
		http:   defaultHTTPClient(),
	}
}

// BrowseSkillEntry represents a skill discovered in an external repository.
type BrowseSkillEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Exists      bool   `json:"exists"`
}

const maxConcurrency = 5

// BrowseSkills lists all skill directories under basePath in the given client's repo.
// A directory is considered a skill if it contains a SKILL.md file.
func BrowseSkills(client *Client, basePath string) ([]BrowseSkillEntry, error) {
	if basePath == "" {
		basePath = "."
	}
	entries, err := client.ListDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("list directory %q: %w", basePath, err)
	}

	var dirs []FileEntry
	for _, e := range entries {
		if e.Type == "dir" {
			dirs = append(dirs, e)
		}
	}

	type indexedEntry struct {
		idx   int
		entry BrowseSkillEntry
		ok    bool
	}

	results := make(chan indexedEntry, len(dirs))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, e := range dirs {
		wg.Add(1)
		go func(idx int, dirName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			skillMdPath := basePath + "/" + dirName + "/SKILL.md"
			if basePath == "." {
				skillMdPath = dirName + "/SKILL.md"
			}
			content, _, err := client.GetFileContent(skillMdPath)
			if err != nil {
				return
			}

			fm, _ := splitFrontmatter(content)
			name := dirName
			description := ""
			if fm != "" {
				var meta map[string]interface{}
				if err := json.Unmarshal([]byte(frontmatterToJSON(fm)), &meta); err == nil {
					if v, ok := meta["name"].(string); ok && v != "" {
						name = v
					}
					if v, ok := meta["description"].(string); ok {
						description = v
					}
				}
			}

			results <- indexedEntry{
				idx: idx,
				entry: BrowseSkillEntry{
					ID:          dirName,
					Name:        name,
					Description: description,
				},
				ok: true,
			}
		}(i, e.Name)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and sort by original directory order.
	collected := make([]indexedEntry, 0, len(dirs))
	for r := range results {
		if r.ok {
			collected = append(collected, r)
		}
	}
	skills := make([]BrowseSkillEntry, 0, len(collected))
	// Use a simple insertion approach since we need original order.
	ordered := make(map[int]BrowseSkillEntry, len(collected))
	for _, c := range collected {
		ordered[c.idx] = c.entry
	}
	for i := range dirs {
		if entry, ok := ordered[i]; ok {
			skills = append(skills, entry)
		}
	}

	return skills, nil
}

// ImportSkillResult reports the outcome of importing a single skill.
type ImportSkillResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ImportSkills copies selected skills from a source repo into the destination Store's
// GitHub repository, committing each file via PutFile. Skills are imported concurrently.
func ImportSkills(src *Client, srcBasePath string, dst *Store, skillIDs []string, overwrite bool) []ImportSkillResult {
	results := make([]ImportSkillResult, len(skillIDs))

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, id := range skillIDs {
		results[i].ID = id

		if !overwrite && dst.GetByID(id) != nil {
			results[i].Error = "skill already exists"
			continue
		}

		wg.Add(1)
		go func(idx int, skillID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := importOneSkill(src, srcBasePath, dst, skillID); err != nil {
				results[idx].Error = err.Error()
			} else {
				results[idx].OK = true
			}
		}(i, id)
	}

	wg.Wait()
	return results
}

func importOneSkill(src *Client, srcBasePath string, dst *Store, id string) error {
	srcDir := id
	if srcBasePath != "" && srcBasePath != "." {
		srcDir = srcBasePath + "/" + id
	}

	// List the skill directory once to discover what exists,
	// avoiding blind requests that produce 404s for missing scripts/references.
	entries, err := src.ListDir(srcDir)
	if err != nil {
		return fmt.Errorf("list skill dir: %w", err)
	}

	hasSkillMd := false
	hasScripts := false
	hasRefs := false
	for _, e := range entries {
		switch {
		case e.Type == "file" && e.Name == "SKILL.md":
			hasSkillMd = true
		case e.Type == "dir" && e.Name == "scripts":
			hasScripts = true
		case e.Type == "dir" && e.Name == "references":
			hasRefs = true
		}
	}
	if !hasSkillMd {
		return fmt.Errorf("SKILL.md not found in %s", srcDir)
	}

	content, _, err := src.GetFileContent(srcDir + "/SKILL.md")
	if err != nil {
		return fmt.Errorf("read SKILL.md: %w", err)
	}

	dstPath := dst.skillMdPath(id)

	var sha string
	if existing := dst.GetByID(id); existing != nil {
		dst.mu.RLock()
		if e, ok := dst.cache[id]; ok {
			sha = e.skillMdSHA
		}
		dst.mu.RUnlock()
	}

	msg := fmt.Sprintf("import skill: %s", id)
	if err := dst.client.PutFile(dstPath, msg, content, sha); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}

	if hasScripts {
		importDir(src, srcDir+"/scripts", dst, dst.scriptsDir(id), id)
	}
	if hasRefs {
		importDir(src, srcDir+"/references", dst, dst.refsDir(id), id)
	}

	return nil
}

func importDir(src *Client, srcDir string, dst *Store, dstDir, skillID string) {
	entries, err := src.ListDir(srcDir)
	if err != nil {
		return
	}

	var files []FileEntry
	for _, e := range entries {
		if e.Type == "file" {
			files = append(files, e)
		}
	}
	if len(files) == 0 {
		return
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, e := range files {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			content, _, err := src.GetFileContent(srcDir + "/" + name)
			if err != nil {
				return
			}
			dstPath := dstDir + "/" + name
			msg := fmt.Sprintf("import skill %s: %s", skillID, name)
			_ = dst.client.PutFile(dstPath, msg, content, "")
		}(e.Name)
	}

	wg.Wait()
}
