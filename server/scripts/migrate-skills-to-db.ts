/**
 * 迁移脚本：将 server/data/skills/ 目录下的 skill 文件迁移到 SQLite 数据库
 *
 * 用法：bun run server/scripts/migrate-skills-to-db.ts
 */

import { resolve, join } from "path";
import { readdirSync, existsSync, readFileSync } from "fs";
import { skillDb } from "../src/services/database";

const SKILLS_DIR = resolve(import.meta.dir, "../data/skills");

function migrate() {
  console.log(`📂 扫描目录: ${SKILLS_DIR}`);

  if (!existsSync(SKILLS_DIR)) {
    console.log("⚠️  Skills 目录不存在，无需迁移");
    return;
  }

  const entries = readdirSync(SKILLS_DIR, { withFileTypes: true });
  const skillDirs = entries.filter(e => e.isDirectory());

  if (skillDirs.length === 0) {
    console.log("⚠️  无 skill 目录，无需迁移");
    return;
  }

  console.log(`📦 发现 ${skillDirs.length} 个 skill 目录\n`);

  let migrated = 0;
  let skipped = 0;
  let failed = 0;

  for (const entry of skillDirs) {
    const skillDir = join(SKILLS_DIR, entry.name);
    const manifestPath = join(skillDir, "skill.json");

    if (!existsSync(manifestPath)) {
      console.log(`  ⚠️  ${entry.name}: 缺少 skill.json，跳过`);
      skipped++;
      continue;
    }

    // Skip if already in DB
    const existing = skillDb.getById(entry.name);
    if (existing) {
      console.log(`  ⏭️  ${entry.name}: 已存在于数据库 (v${existing.version})，跳过`);
      skipped++;
      continue;
    }

    try {
      const manifest = JSON.parse(readFileSync(manifestPath, "utf-8"));
      const readmePath = join(skillDir, "README.md");
      const readme = existsSync(readmePath) ? readFileSync(readmePath, "utf-8") : "";

      // Parse semver string to integer (e.g., "2.0.0" → 2, "1.0.0" → 1)
      let versionInt = 1;
      const semverMatch = String(manifest.version || "1.0.0").match(/^(\d+)/);
      if (semverMatch) {
        versionInt = Math.max(1, parseInt(semverMatch[1], 10));
      }

      const skill = skillDb.bulkImport({
        id: entry.name,
        name: manifest.name || entry.name,
        description: manifest.description || "",
        version: versionInt,
        type: manifest.type || "knowledge",
        enabled: manifest.enabled !== false,
        triggers: manifest.triggers || [],
        tools: manifest.tools || [],
        readme,
      });

      console.log(`  ✅ ${entry.name}: 迁移成功 (v${skill.version}, ${skill.tools.length} tools)`);
      migrated++;
    } catch (error) {
      console.log(`  ❌ ${entry.name}: 迁移失败 - ${error instanceof Error ? error.message : error}`);
      failed++;
    }
  }

  console.log(`\n📊 迁移完成: ${migrated} 成功, ${skipped} 跳过, ${failed} 失败`);

  // Verify
  const allSkills = skillDb.getAll();
  console.log(`\n📋 数据库中共 ${allSkills.length} 个 skill:`);
  for (const s of allSkills) {
    console.log(`  - ${s.id} (v${s.version}, ${s.enabled ? "✅" : "❌"}, ${s.tools.length} tools)`);
  }
}

migrate();
