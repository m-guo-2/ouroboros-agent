package subagent

import (
	"strings"
	"testing"
)

func TestNormalizeProfileSupportsWebResearch(t *testing.T) {
	got, err := normalizeProfile("web_research")
	if err != nil {
		t.Fatalf("normalizeProfile returned error: %v", err)
	}
	if got != "web_research" {
		t.Fatalf("expected web_research, got %s", got)
	}
}

func TestAllowedToolsForWebResearch(t *testing.T) {
	allowed := allowedToolsForProfile("web_research")
	if !allowed["tavily_search"] {
		t.Fatalf("expected tavily_search to be allowed")
	}
	if !allowed["recall_context"] {
		t.Fatalf("expected recall_context to be allowed")
	}
	if allowed["shell"] {
		t.Fatalf("did not expect shell to be allowed")
	}
	if allowed["write_file"] {
		t.Fatalf("did not expect write_file to be allowed")
	}
}

func TestDefaultPromptByProfileWebResearch(t *testing.T) {
	prompt := defaultPromptByProfile("web_research")
	for _, want := range []string{
		"tavily_search",
		"先缩窄或改写 query 再继续检索",
		"不尝试本地文件写入、Shell 执行",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q", want)
		}
	}
}

func TestNormalizeProfileSupportsDataReport(t *testing.T) {
	got, err := normalizeProfile("data_report")
	if err != nil {
		t.Fatalf("normalizeProfile returned error: %v", err)
	}
	if got != "data_report" {
		t.Fatalf("expected data_report, got %s", got)
	}
}

func TestAllowedToolsForDataReport(t *testing.T) {
	allowed := allowedToolsForProfile("data_report")
	if !allowed["render_card"] {
		t.Fatal("expected render_card to be allowed")
	}
	if !allowed["read_file"] {
		t.Fatal("expected read_file to be allowed")
	}
	if !allowed["list_dir"] {
		t.Fatal("expected list_dir to be allowed")
	}
	if allowed["shell"] {
		t.Fatal("did not expect shell to be allowed")
	}
	if allowed["write_file"] {
		t.Fatal("did not expect write_file to be allowed")
	}
}

func TestProfileDisplayNameDataReport(t *testing.T) {
	name := profileDisplayName("data_report")
	if name != "data-report-subagent" {
		t.Fatalf("expected data-report-subagent, got %s", name)
	}
}

func TestDefaultPromptByProfileDataReport(t *testing.T) {
	prompt := defaultPromptByProfile("data_report")
	for _, want := range []string{
		"data_report",
		"render_card",
		"kpi",
		"fallback",
		"imageUrl",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q", want)
		}
	}
}
