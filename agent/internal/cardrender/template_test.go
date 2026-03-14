package cardrender

import (
	"strings"
	"testing"
)

func TestListTemplates(t *testing.T) {
	names := ListTemplates()
	if len(names) == 0 {
		t.Fatal("expected at least one template, got none")
	}

	expected := []string{"kpi", "ranking", "status-board", "summary", "table", "timeline"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d templates, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("template[%d]: expected %q, got %q", i, name, names[i])
		}
	}
}

func TestRenderTemplate_KPI(t *testing.T) {
	data := map[string]interface{}{
		"title":      "月活跃用户",
		"value":      "128,000",
		"trend":      "+12.5%",
		"trendUp":    true,
		"comparison": "较上月",
	}
	html, err := RenderTemplate("kpi", data)
	if err != nil {
		t.Fatalf("RenderTemplate(kpi): %v", err)
	}
	if !strings.Contains(html, "128,000") {
		t.Error("expected value in HTML output")
	}
	if !strings.Contains(html, "+12.5%") {
		t.Error("expected trend in HTML output")
	}
	if !strings.Contains(html, "trend-up") {
		t.Error("expected trend-up class for positive trend")
	}
}

func TestRenderTemplate_Table(t *testing.T) {
	data := map[string]interface{}{
		"title":   "销售对比",
		"headers": []interface{}{"产品", "Q1", "Q2"},
		"rows": []interface{}{
			[]interface{}{"产品A", "120", "156"},
			[]interface{}{"产品B", "89", "72"},
		},
	}
	html, err := RenderTemplate("table", data)
	if err != nil {
		t.Fatalf("RenderTemplate(table): %v", err)
	}
	if !strings.Contains(html, "产品A") {
		t.Error("expected row data in HTML output")
	}
	if !strings.Contains(html, "<table>") {
		t.Error("expected table element in HTML output")
	}
}

func TestRenderTemplate_StatusBoard(t *testing.T) {
	data := map[string]interface{}{
		"title": "系统状态",
		"items": []interface{}{
			map[string]interface{}{"name": "API", "status": "ok"},
			map[string]interface{}{"name": "DB", "status": "error"},
		},
	}
	html, err := RenderTemplate("status-board", data)
	if err != nil {
		t.Fatalf("RenderTemplate(status-board): %v", err)
	}
	if !strings.Contains(html, "status-ok") {
		t.Error("expected status-ok class")
	}
	if !strings.Contains(html, "status-error") {
		t.Error("expected status-error class")
	}
}

func TestRenderTemplate_Ranking(t *testing.T) {
	data := map[string]interface{}{
		"title": "排行",
		"items": []interface{}{
			map[string]interface{}{"name": "Alice", "value": float64(100)},
			map[string]interface{}{"name": "Bob", "value": float64(80)},
		},
	}
	html, err := RenderTemplate("ranking", data)
	if err != nil {
		t.Fatalf("RenderTemplate(ranking): %v", err)
	}
	if !strings.Contains(html, "Alice") {
		t.Error("expected name in HTML output")
	}
	if !strings.Contains(html, "rank-1") {
		t.Error("expected rank-1 class for first item")
	}
}

func TestRenderTemplate_Timeline(t *testing.T) {
	data := map[string]interface{}{
		"title": "项目进展",
		"events": []interface{}{
			map[string]interface{}{"time": "03-01", "description": "启动"},
			map[string]interface{}{"time": "03-10", "description": "完成"},
		},
	}
	html, err := RenderTemplate("timeline", data)
	if err != nil {
		t.Fatalf("RenderTemplate(timeline): %v", err)
	}
	if !strings.Contains(html, "03-01") {
		t.Error("expected time in HTML output")
	}
	if !strings.Contains(html, "timeline") {
		t.Error("expected timeline class")
	}
}

func TestRenderTemplate_Summary(t *testing.T) {
	data := map[string]interface{}{
		"title": "概况",
		"items": []interface{}{
			map[string]interface{}{"label": "负责人", "value": "张三"},
			map[string]interface{}{"label": "状态", "value": "进行中"},
		},
	}
	html, err := RenderTemplate("summary", data)
	if err != nil {
		t.Fatalf("RenderTemplate(summary): %v", err)
	}
	if !strings.Contains(html, "张三") {
		t.Error("expected value in HTML output")
	}
}

func TestRenderTemplate_UnknownTemplate(t *testing.T) {
	_, err := RenderTemplate("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
	if !strings.Contains(err.Error(), "kpi") {
		t.Errorf("error should list available templates, got: %v", err)
	}
}
