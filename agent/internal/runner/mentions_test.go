package runner

import "testing"

func TestNormalizeToolMentionsFiltersUnsafeTargets(t *testing.T) {
	got := normalizeToolMentions([]interface{}{
		map[string]interface{}{"userId": " user-1 ", "name": " 张三 "},
		map[string]interface{}{"userId": "user-1", "name": "重复"},
		map[string]interface{}{"userId": "@all", "name": "所有人"},
		map[string]interface{}{"id": "user-2"},
		"",
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 mentions, got %+v", got)
	}
	if got[0]["userId"] != "user-1" || got[0]["name"] != "张三" {
		t.Fatalf("unexpected first mention: %+v", got[0])
	}
	if got[1]["userId"] != "user-2" {
		t.Fatalf("unexpected second mention: %+v", got[1])
	}
}
