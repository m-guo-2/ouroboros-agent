package sandbox

import (
	"context"
	"testing"
	"time"
)

func newTestSandbox(t *testing.T) *Sandbox {
	t.Helper()

	return &Sandbox{
		SessionID: "test-session",
		RootDir:   t.TempDir(),
		env:       []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"},
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}
}

func TestExecWithEnvDoesNotPersistOverrides(t *testing.T) {
	sb := newTestSandbox(t)

	out, exitCode, err := sb.ExecWithEnv(context.Background(), "printf %s \"$SANDBOX_TMP_VALUE\"", time.Second, map[string]string{
		"SANDBOX_TMP_VALUE": "one-shot",
	})
	if err != nil {
		t.Fatalf("exec with env: %v", err)
	}
	if exitCode != 0 || out != "one-shot" {
		t.Fatalf("expected one-shot env output, got exit=%d output=%q", exitCode, out)
	}

	out, exitCode, err = sb.Exec(context.Background(), "printf %s \"${SANDBOX_TMP_VALUE:-missing}\"", time.Second)
	if err != nil {
		t.Fatalf("exec after one-shot env: %v", err)
	}
	if exitCode != 0 || out != "missing" {
		t.Fatalf("expected one-shot env not to persist, got exit=%d output=%q", exitCode, out)
	}
}

func TestSetEnvPersistsForCommandsAndEnviron(t *testing.T) {
	sb := newTestSandbox(t)

	if err := sb.SetEnv(map[string]string{"SANDBOX_PERSISTED_VALUE": "persisted"}); err != nil {
		t.Fatalf("set env: %v", err)
	}

	out, exitCode, err := sb.Exec(context.Background(), "printf %s \"$SANDBOX_PERSISTED_VALUE\"", time.Second)
	if err != nil {
		t.Fatalf("exec with persisted env: %v", err)
	}
	if exitCode != 0 || out != "persisted" {
		t.Fatalf("expected persisted env output, got exit=%d output=%q", exitCode, out)
	}
	if got := sb.Environ()["SANDBOX_PERSISTED_VALUE"]; got != "persisted" {
		t.Fatalf("expected Environ to include persisted env, got %q", got)
	}
}

func TestUnsetEnvRemovesPersistedValue(t *testing.T) {
	sb := newTestSandbox(t)

	if err := sb.SetEnv(map[string]string{"SANDBOX_REMOVED_VALUE": "remove-me"}); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if err := sb.UnsetEnv([]string{"SANDBOX_REMOVED_VALUE"}); err != nil {
		t.Fatalf("unset env: %v", err)
	}

	out, exitCode, err := sb.Exec(context.Background(), "printf %s \"${SANDBOX_REMOVED_VALUE:-missing}\"", time.Second)
	if err != nil {
		t.Fatalf("exec after unset env: %v", err)
	}
	if exitCode != 0 || out != "missing" {
		t.Fatalf("expected env to be removed, got exit=%d output=%q", exitCode, out)
	}
}

func TestInvalidEnvNameRejected(t *testing.T) {
	sb := newTestSandbox(t)

	if err := sb.SetEnv(map[string]string{"BAD-NAME": "value"}); err == nil {
		t.Fatal("expected invalid env name to be rejected")
	}
	if _, _, err := sb.ExecWithEnv(context.Background(), "true", time.Second, map[string]string{"BAD-NAME": "value"}); err == nil {
		t.Fatal("expected invalid one-shot env name to be rejected")
	}
}
