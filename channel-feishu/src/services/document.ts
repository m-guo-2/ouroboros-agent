import { getClient } from "../client";
import type {
  CreateDocumentParams,
  CreateWikiNodeParams,
  DocumentBlock,
} from "../types";

const client = () => getClient();

// ==================== 文档操作 ====================

/**
 * 创建新文档
 * https://open.feishu.cn/document/server-docs/docs/docs/docx-v1/document/create
 */
export async function createDocument(params: CreateDocumentParams) {
  const res = await client().docx.document.create({
    data: {
      title: params.title,
      folder_token: params.folderToken,
    },
  });
  return res;
}

/**
 * 获取文档信息
 */
export async function getDocument(documentId: string) {
  const res = await client().docx.document.get({
    path: {
      document_id: documentId,
    },
  });
  return res;
}

/**
 * 获取文档纯文本内容
 */
export async function getDocumentRawContent(documentId: string) {
  const res = await client().docx.document.rawContent({
    path: {
      document_id: documentId,
    },
  });
  return res;
}

/**
 * 获取文档所有块
 */
export async function getDocumentBlocks(documentId: string) {
  const res = await client().docx.documentBlock.list({
    path: {
      document_id: documentId,
    },
    params: {
      page_size: 500,
    },
  });
  return res;
}

/**
 * 创建文档块（在文档末尾追加内容）
 */
export async function appendDocumentBlocks(
  documentId: string,
  blockId: string,
  blocks: DocumentBlock[]
) {
  // 将我们的简化格式转为飞书 API 格式
  const children = blocks.map(convertBlockToFeishuFormat);

  const res = await client().docx.documentBlockChildren.create({
    path: {
      document_id: documentId,
      block_id: blockId,
    },
    data: {
      children,
      index: -1, // 追加到末尾
    },
  });
  return res;
}

/**
 * 将简化的 block 格式转换为飞书 API 格式
 */
function convertBlockToFeishuFormat(block: DocumentBlock): any {
  switch (block.blockType) {
    case "paragraph": {
      const textRun = {
        content: block.text,
        text_element_style: {},
      };

      // 设置段落样式
      let headingLevel = 0;
      if (block.style === "heading1") headingLevel = 1;
      if (block.style === "heading2") headingLevel = 2;
      if (block.style === "heading3") headingLevel = 3;
      if (block.style === "heading4") headingLevel = 4;

      if (headingLevel > 0) {
        return {
          block_type: headingLevel + 2, // heading1=3, heading2=4, heading3=5, heading4=6
          heading: {
            elements: [{ text_run: textRun }],
          },
        };
      }

      return {
        block_type: 2, // paragraph
        paragraph: {
          elements: [{ text_run: textRun }],
        },
      };
    }

    case "code":
      return {
        block_type: 14, // code
        code: {
          elements: [
            {
              text_run: {
                content: block.code,
                text_element_style: {},
              },
            },
          ],
          style: {
            language: mapCodeLanguage(block.language),
          },
        },
      };

    case "callout":
      return {
        block_type: 19, // callout
        callout: {
          elements: [
            {
              text_run: {
                content: block.text,
                text_element_style: {},
              },
            },
          ],
        },
      };

    case "divider":
      return {
        block_type: 22, // divider
        divider: {},
      };

    default:
      return {
        block_type: 2,
        paragraph: {
          elements: [
            {
              text_run: {
                content: String((block as any).text || ""),
                text_element_style: {},
              },
            },
          ],
        },
      };
  }
}

/** 将语言名称映射到飞书代码块语言枚举 */
function mapCodeLanguage(lang?: string): number {
  const langMap: Record<string, number> = {
    plaintext: 1,
    abap: 2,
    ada: 3,
    apache: 4,
    bash: 7,
    c: 9,
    "c#": 10,
    "c++": 11,
    css: 15,
    dart: 18,
    dockerfile: 20,
    go: 24,
    html: 28,
    java: 30,
    javascript: 31,
    json: 33,
    kotlin: 35,
    markdown: 39,
    python: 49,
    ruby: 52,
    rust: 53,
    shell: 56,
    sql: 58,
    swift: 60,
    typescript: 63,
    xml: 66,
    yaml: 67,
  };
  return langMap[lang?.toLowerCase() || "plaintext"] || 1;
}

// ==================== 知识库操作 ====================

/**
 * 获取知识库列表
 */
export async function getWikiSpaces() {
  const res = await client().wiki.space.list({
    params: {
      page_size: 50,
    },
  });
  return res;
}

/**
 * 获取知识库节点信息
 */
export async function getWikiNode(spaceId: string, nodeToken: string) {
  const res = await client().wiki.spaceNode.list({
    params: {
      page_size: 50,
      parent_node_token: nodeToken,
    },
    path: {
      space_id: spaceId,
    },
  });
  return res;
}

/**
 * 创建知识库节点
 */
export async function createWikiNode(params: CreateWikiNodeParams) {
  const res = await client().wiki.spaceNode.create({
    path: {
      space_id: params.spaceId,
    },
    data: {
      obj_type: "docx",
      parent_node_token: params.parentNodeToken,
      node_type: params.nodeType || "origin",
      title: params.title,
    },
  });
  return res;
}

// ==================== 云空间/文件夹操作 ====================

/**
 * 获取根文件夹信息
 */
export async function getRootFolder() {
  const res = await client().drive.file.list({
    params: {
      page_size: 50,
    },
  });
  return res;
}

/**
 * 获取文件夹内容列表
 */
export async function getFolderContents(folderToken: string) {
  const res = await client().drive.file.list({
    params: {
      page_size: 50,
      folder_token: folderToken,
    },
  });
  return res;
}

/**
 * 创建文件夹
 */
export async function createFolder(
  name: string,
  folderToken?: string
) {
  const res = await client().drive.file.createFolder({
    data: {
      name,
      folder_token: folderToken || "",
    },
  });
  return res;
}
