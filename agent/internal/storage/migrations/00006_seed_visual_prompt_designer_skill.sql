-- +goose Up
-- +goose StatementBegin

INSERT INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'visual-prompt-designer',
    '生图提示词设计',
    '把用户对表情包、海报、视觉摘要、运营战报等图片的自然语言需求，改写为适合 Doubao Seedream 的高质量生图提示词',
    '1.0.0',
    'knowledge',
    1,
    '## 生图提示词设计\n\n你的职责是把用户的自然语言需求，改写成可直接交给 image_generate 工具的专业生图提示词。你不直接生成图片，只产出清晰、具体、可执行的 prompt。\n\n### 适用场景\n\n- 表情包、贴纸、头像、角色反应图\n- 运营海报、活动封面、社群分享图\n- 视觉化摘要、轻量战报、情绪化统计图\n- 产品氛围图、概念图、插画、漫画分镜\n\n### 不适用场景\n\n- 必须逐字逐数准确的 KPI、金额、排名、表格\n- 法务、财务、审计等严肃报表\n- 需要可编辑结构化图表的场景\n\n这些场景应优先使用 data_report 或 HTML/card render。若用户仍要求生图，只能把它定位为视觉表达图，并明确不要依赖图片里的数字作为事实来源。\n\n### 工作流\n\n1. 识别图片用途：表情包、海报、战报、封面、插画、产品图等。\n2. 提炼主体：谁/什么是画面中心。\n3. 提炼情绪和动作：愤怒、无语、开心、冲刺、庆祝、压力等。\n4. 提炼版式：正方形、横版、竖版、贴纸、手机海报、聊天表情。\n5. 提炼文字：如需文字，保持极短，逐字写明；不确定时建议无文字。\n6. 生成最终 prompt，并给出建议 size。\n\n### Prompt 结构\n\n最终 prompt 应包含：\n\n- Subject：主体和关键元素\n- Scene：环境或背景\n- Composition：构图和画幅\n- Style：风格、材质、色彩、光照\n- Text：需要出现在图里的短文字，必须逐字标明；无文字则写 no text\n- Constraints：不要水印、不要多余文字、不要品牌 logo、不要错误数字\n\n### 表情包模板\n\nCreate a square chat sticker / meme image. Subject: {主体}. Emotion: {情绪}. Action: {动作}. Style: bold clean sticker illustration, expressive face, thick outline, high contrast, simple background. Text: \"{短文字}\". Keep the text large, centered, and exactly as written. No watermark, no extra text.\n\n### 运营战报模板\n\nCreate a polished social media report poster. Theme: {主题}. Key visual: {主视觉}. Layout: strong headline area, 3-5 visual blocks, clear hierarchy, mobile-friendly vertical composition. Style: modern editorial design, crisp typography, premium but energetic colors. Text: only include these exact short labels: {文字}. Avoid tiny tables and dense numbers. No watermark, no extra text.\n\n### 视觉摘要模板\n\nCreate an infographic-style visual summary. Topic: {主题}. Use symbolic visual blocks rather than precise tables. Composition: clean grid, strong central title, a few simple icon-like illustrations, spacious layout. Style: modern flat editorial illustration, readable hierarchy. Text: {文字要求}. No watermark, no extra text.\n\n### 输出格式\n\n加载本 skill 后，先输出你准备调用 image_generate 的参数，不要长篇解释：\n\n```json\n{\n  \"prompt\": \"...\",\n  \"size\": \"2K\"\n}\n```\n\n随后调用 image_generate。若用户需求缺少会显著改变结果的关键信息，只问一个最关键问题；否则直接按合理判断生成 prompt。',
    '{}',
    0, 0, 0
)
ON DUPLICATE KEY UPDATE
    name = VALUES(name),
    description = VALUES(description),
    version = VALUES(version),
    type = VALUES(type),
    enabled = VALUES(enabled),
    readme = VALUES(readme),
    metadata = VALUES(metadata),
    updated_at = VALUES(updated_at),
    deleted_at = 0;

-- +goose StatementEnd

-- +goose StatementBegin

INSERT INTO agent_skill_bindings
    (id, agent_id, skill_id, mode, position, created_at, deleted_at)
SELECT
    CONCAT('asb-', id, '-visual-prompt-designer'),
    id,
    'visual-prompt-designer',
    'always',
    50,
    0,
    0
FROM agent_configs
WHERE deleted_at = 0 AND is_active = 1
ON DUPLICATE KEY UPDATE
    mode = VALUES(mode),
    position = VALUES(position),
    deleted_at = 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM agent_skill_bindings WHERE skill_id = 'visual-prompt-designer';
DELETE FROM skills WHERE id = 'visual-prompt-designer';

-- +goose StatementEnd
