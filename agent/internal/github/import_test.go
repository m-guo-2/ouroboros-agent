package github

import "testing"

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    GitHubSource
		wantErr bool
	}{
		{
			name:  "full URL with tree/branch/path",
			input: "https://github.com/user/awesome-skills/tree/main/skills",
			want:  GitHubSource{Owner: "user", Repo: "awesome-skills", Branch: "main", Path: "skills"},
		},
		{
			name:  "full URL repo only",
			input: "https://github.com/user/awesome-skills",
			want:  GitHubSource{Owner: "user", Repo: "awesome-skills", Branch: "main"},
		},
		{
			name:  "full URL with trailing slash",
			input: "https://github.com/user/repo/",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main"},
		},
		{
			name:  "full URL non-main branch",
			input: "https://github.com/org/repo/tree/develop/data/skills",
			want:  GitHubSource{Owner: "org", Repo: "repo", Branch: "develop", Path: "data/skills"},
		},
		{
			name:  "blob URL extracts parent directory",
			input: "https://github.com/user/repo/blob/main/skills/my-skill/SKILL.md",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main", Path: "skills/my-skill"},
		},
		{
			name:  "blob URL file at root",
			input: "https://github.com/user/repo/blob/main/README.md",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main", Path: ""},
		},
		{
			name:  "repo with .git suffix",
			input: "https://github.com/user/repo.git",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main"},
		},
		{
			name:  "shorthand owner/repo",
			input: "user/awesome-skills",
			want:  GitHubSource{Owner: "user", Repo: "awesome-skills", Branch: "main"},
		},
		{
			name:  "shorthand with path",
			input: "user/repo/skills",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main", Path: "skills"},
		},
		{
			name:  "shorthand with deep path",
			input: "user/repo/data/skills/go",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main", Path: "data/skills/go"},
		},
		{
			name:  "whitespace trimmed",
			input: "  user/repo  ",
			want:  GitHubSource{Owner: "user", Repo: "repo", Branch: "main"},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only whitespace",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "single segment",
			input:   "just-a-name",
			wantErr: true,
		},
		{
			name:    "non-github host",
			input:   "https://gitlab.com/user/repo",
			wantErr: true,
		},
		{
			name:    "github URL missing repo",
			input:   "https://github.com/user",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGitHubURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
