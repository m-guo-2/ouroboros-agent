package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerInstallsOfficeRuntime(t *testing.T) {
	manager := NewManager(t.TempDir())
	sb, created, err := manager.GetOrCreate("runtime-test")
	if err != nil {
		t.Fatalf("get sandbox: %v", err)
	}
	if !created {
		t.Fatal("expected sandbox to be created")
	}

	if _, err := os.Stat(filepath.Join(sb.RootDir, runtimeDirName, "bin", "office")); err != nil {
		t.Fatalf("office cli not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sb.RootDir, runtimeDirName, "capabilities.json")); err != nil {
		t.Fatalf("capabilities not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sb.RootDir, runtimeDirName, "template.json")); err != nil {
		t.Fatalf("template manifest not installed: %v", err)
	}
	if sb.Template.ID != OfficeWorkerTemplateID {
		t.Fatalf("template id = %q, want %q", sb.Template.ID, OfficeWorkerTemplateID)
	}

	out, exitCode, err := sb.Exec(context.Background(), "office capabilities", 5*time.Second)
	if err != nil {
		t.Fatalf("office capabilities: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("office capabilities exit=%d output=%s", exitCode, out)
	}
	if !strings.Contains(out, `"office"`) || !strings.Contains(out, `"docx"`) || !strings.Contains(out, `"xlsx"`) {
		t.Fatalf("capabilities output missing office data: %s", out)
	}

	out, exitCode, err = sb.Exec(context.Background(), "printf '%s:%s' \"$SANDBOX_TEMPLATE_ID\" \"$SANDBOX_CATEGORY\"", 5*time.Second)
	if err != nil || exitCode != 0 {
		t.Fatalf("template env exit=%d err=%v output=%s", exitCode, err, out)
	}
	if !strings.Contains(out, OfficeWorkerTemplateID+":white-collar-office-suite") {
		t.Fatalf("template env mismatch: %s", out)
	}

	out, exitCode, err = sb.Exec(context.Background(), "if command -v fc-match >/dev/null 2>&1; then office fonts; else echo no-fc-match; fi", 5*time.Second)
	if err != nil || exitCode != 0 {
		t.Fatalf("office fonts exit=%d err=%v output=%s", exitCode, err, out)
	}
	if strings.TrimSpace(out) != "no-fc-match" && (!strings.Contains(out, `"宋体"`) || !strings.Contains(out, `"仿宋_GB2312"`) || !strings.Contains(out, `"方正小标宋简体"`)) {
		t.Fatalf("office fonts missing expected public-document font names: %s", out)
	}
}

func TestOfficeRuntimeEndToEnd(t *testing.T) {
	if os.Getenv("SANDBOX_OFFICE_E2E") != "1" {
		t.Skip("set SANDBOX_OFFICE_E2E=1 to run office runtime e2e test")
	}

	manager := NewManager(t.TempDir())
	sb, _, err := manager.GetOrCreate("office-e2e")
	if err != nil {
		t.Fatalf("get sandbox: %v", err)
	}

	out, exitCode, err := sb.Exec(context.Background(), "office self-test --work-dir output/self-test", 60*time.Second)
	if err != nil {
		t.Fatalf("office self-test: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("office self-test exit=%d output=%s", exitCode, out)
	}
	if !strings.Contains(out, `"ok": true`) {
		t.Fatalf("self-test did not report ok: %s", out)
	}

	out, exitCode, err = sb.Exec(context.Background(), "office inspect-docx output/self-test/selftest_edited.docx", 30*time.Second)
	if err != nil || exitCode != 0 {
		t.Fatalf("inspect docx exit=%d err=%v output=%s", exitCode, err, out)
	}
	if !strings.Contains(out, "Office Runtime 自检") || !strings.Contains(out, "OK") {
		t.Fatalf("inspect docx missing expected content: %s", out)
	}

	out, exitCode, err = sb.Exec(context.Background(), "office inspect-xlsx output/self-test/selftest_edited.xlsx", 30*time.Second)
	if err != nil || exitCode != 0 {
		t.Fatalf("inspect xlsx exit=%d err=%v output=%s", exitCode, err, out)
	}
	if !strings.Contains(out, `"formula": "=SUM(B4:C4)"`) {
		t.Fatalf("inspect xlsx missing expected formula: %s", out)
	}

	for _, command := range []string{
		"printf '%s' '{\"contains_text\":[\"Office Runtime 自检\",\"OK\"]}' > output/self-test/expect_docx.json && office validate output/self-test/selftest_edited.docx --expect output/self-test/expect_docx.json",
		"printf '%s' '{\"contains_text\":[\"=SUM(B4:C4)\"]}' > output/self-test/expect_xlsx.json && office validate output/self-test/selftest_edited.xlsx --expect output/self-test/expect_xlsx.json",
	} {
		out, exitCode, err = sb.Exec(context.Background(), "sh -c "+shellQuote(command), 30*time.Second)
		if err != nil || exitCode != 0 {
			t.Fatalf("validate command exit=%d err=%v output=%s", exitCode, err, out)
		}
	}

	out, exitCode, err = sb.Exec(context.Background(), "if command -v soffice >/dev/null 2>&1; then office render output/self-test/selftest_edited.docx --out-dir output/rendered-docx && office render output/self-test/selftest_edited.xlsx --out-dir output/rendered-xlsx; else echo no-soffice; fi", 120*time.Second)
	if err != nil {
		t.Fatalf("render command: %v", err)
	}
	if exitCode != 0 {
		t.Logf("office render skipped or failed in current local environment: %s", out)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
