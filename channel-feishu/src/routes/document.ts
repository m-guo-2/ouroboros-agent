import { Router } from "express";
import * as documentService from "../services/document";
import type { ApiResponse } from "../types";

const router = Router();

// ==================== 文档操作 ====================

/** POST /api/feishu/document - 创建文档 */
router.post("/", async (req, res) => {
  try {
    const { title, folderToken } = req.body;
    if (!title) {
      res.status(400).json({ success: false, error: "title is required" } as ApiResponse);
      return;
    }

    const result = await documentService.createDocument({ title, folderToken });
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("创建文档失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/document/:documentId - 获取文档信息 */
router.get("/:documentId", async (req, res) => {
  try {
    const { documentId } = req.params;
    const result = await documentService.getDocument(documentId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取文档失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/document/:documentId/raw - 获取文档纯文本内容 */
router.get("/:documentId/raw", async (req, res) => {
  try {
    const { documentId } = req.params;
    const result = await documentService.getDocumentRawContent(documentId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取文档纯文本失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/document/:documentId/blocks - 获取文档所有块 */
router.get("/:documentId/blocks", async (req, res) => {
  try {
    const { documentId } = req.params;
    const result = await documentService.getDocumentBlocks(documentId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取文档块失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/document/:documentId/blocks - 追加文档内容 */
router.post("/:documentId/blocks", async (req, res) => {
  try {
    const { documentId } = req.params;
    const { blockId, blocks } = req.body;
    if (!blockId || !blocks || !Array.isArray(blocks)) {
      res.status(400).json({
        success: false,
        error: "blockId and blocks array are required",
      } as ApiResponse);
      return;
    }

    const result = await documentService.appendDocumentBlocks(
      documentId,
      blockId,
      blocks
    );
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("追加文档内容失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

// ==================== 知识库操作 ====================

/** GET /api/feishu/document/wiki/spaces - 获取知识库列表 */
router.get("/wiki/spaces", async (req, res) => {
  try {
    const result = await documentService.getWikiSpaces();
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取知识库列表失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/document/wiki/:spaceId/nodes - 获取知识库节点 */
router.get("/wiki/:spaceId/nodes", async (req, res) => {
  try {
    const { spaceId } = req.params;
    const { parentNodeToken } = req.query;
    const result = await documentService.getWikiNode(
      spaceId,
      parentNodeToken as string || ""
    );
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取知识库节点失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/document/wiki/node - 创建知识库节点 */
router.post("/wiki/node", async (req, res) => {
  try {
    const { spaceId, parentNodeToken, title, nodeType } = req.body;
    if (!spaceId || !title) {
      res.status(400).json({
        success: false,
        error: "spaceId and title are required",
      } as ApiResponse);
      return;
    }

    const result = await documentService.createWikiNode({
      spaceId,
      parentNodeToken,
      title,
      nodeType,
    });
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("创建知识库节点失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

// ==================== 云空间操作 ====================

/** GET /api/feishu/document/drive/files - 获取文件列表 */
router.get("/drive/files", async (req, res) => {
  try {
    const { folderToken } = req.query;
    const result = folderToken
      ? await documentService.getFolderContents(folderToken as string)
      : await documentService.getRootFolder();
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取文件列表失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/document/drive/folder - 创建文件夹 */
router.post("/drive/folder", async (req, res) => {
  try {
    const { name, folderToken } = req.body;
    if (!name) {
      res.status(400).json({ success: false, error: "name is required" } as ApiResponse);
      return;
    }

    const result = await documentService.createFolder(name, folderToken);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("创建文件夹失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

export default router;
