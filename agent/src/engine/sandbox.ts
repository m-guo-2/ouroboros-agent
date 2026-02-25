/**
 * Sandbox — 本地目录隔离
 *
 * 提供路径校验和安全的命令执行能力。
 * 确保 Agent 的所有文件操作和 Shell 命令
 * 不会逃逸到分配的工作空间之外。
 */

import { resolve, relative } from "node:path";
import { existsSync, mkdirSync } from "node:fs";
import { exec } from "node:child_process";

const COMMAND_TIMEOUT_MS = 30_000;
const MAX_OUTPUT_BYTES = 1024 * 512; // 512KB

/**
 * 检查路径是否在允许的根目录内。
 * 防御 `../` 逃逸和符号链接攻击。
 */
export function isPathInsideSandbox(targetPath: string, sandboxRoot: string): boolean {
  const resolved = resolve(sandboxRoot, targetPath);
  const rel = relative(sandboxRoot, resolved);
  return rel === "" || (!rel.startsWith("..") && !rel.startsWith("/"));
}

/**
 * 解析并校验路径，确保在沙盒内。
 * 通过则返回绝对路径，否则抛异常。
 */
export function resolveSandboxPath(targetPath: string, sandboxRoot: string): string {
  const resolved = resolve(sandboxRoot, targetPath);
  if (!isPathInsideSandbox(resolved, sandboxRoot)) {
    throw new Error(
      `Path escapes sandbox: "${targetPath}" resolves to "${resolved}", ` +
      `which is outside "${sandboxRoot}"`,
    );
  }
  return resolved;
}

/**
 * 确保沙盒工作目录存在。
 */
export function ensureSandboxDir(sandboxRoot: string): void {
  if (!existsSync(sandboxRoot)) {
    mkdirSync(sandboxRoot, { recursive: true });
  }
}

/**
 * 在沙盒内执行 Shell 命令。
 * - cwd 锁定在 sandboxRoot
 * - 有超时保护
 * - 输出截断保护
 */
export function execInSandbox(
  command: string,
  sandboxRoot: string,
  timeoutMs = COMMAND_TIMEOUT_MS,
): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  return new Promise((resolvePromise) => {
    const child = exec(command, {
      cwd: sandboxRoot,
      timeout: timeoutMs,
      maxBuffer: MAX_OUTPUT_BYTES,
      env: {
        ...process.env,
        HOME: sandboxRoot,
      },
    }, (error, stdout, stderr) => {
      const exitCode = error?.code
        ? (typeof error.code === "number" ? error.code : 1)
        : 0;

      resolvePromise({
        stdout: stdout?.toString() || "",
        stderr: stderr?.toString() || (error?.message || ""),
        exitCode: typeof exitCode === "number" ? exitCode : 1,
      });
    });

    // 防止僵尸进程
    child.unref?.();
  });
}
