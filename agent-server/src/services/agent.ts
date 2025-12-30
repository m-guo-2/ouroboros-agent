import { getAdapter, ChatMessage, StreamChunk, ToolDefinition } from "./models";

export interface Conversation {
  id: string;
  title: string;
  modelId: string;
  messages: ChatMessage[];
  createdAt: Date;
  updatedAt: Date;
}

// 内存中的会话存储
const conversations: Map<string, Conversation> = new Map();

// 系统提示词
const DEFAULT_SYSTEM_PROMPT = `你是一个智能AI助手。请用简洁、专业的方式回答用户的问题。

你可以：
- 回答各种问题
- 帮助编程和代码分析
- 提供建议和解决方案
- 进行创意写作

请使用中文回答，除非用户使用其他语言提问。`;

// Agent 工具定义（预留扩展）
const agentTools: ToolDefinition[] = [
  // 后续可以添加更多工具
];

export function createConversation(modelId: string, title?: string): Conversation {
  const id = crypto.randomUUID();
  const conversation: Conversation = {
    id,
    title: title || "新对话",
    modelId,
    messages: [
      {
        role: "system",
        content: DEFAULT_SYSTEM_PROMPT,
      },
    ],
    createdAt: new Date(),
    updatedAt: new Date(),
  };
  conversations.set(id, conversation);
  return conversation;
}

export function getConversation(id: string): Conversation | undefined {
  return conversations.get(id);
}

export function getAllConversations(): Conversation[] {
  return Array.from(conversations.values()).sort(
    (a, b) => b.updatedAt.getTime() - a.updatedAt.getTime()
  );
}

export function deleteConversation(id: string): boolean {
  return conversations.delete(id);
}

export function updateConversationTitle(id: string, title: string): Conversation | null {
  const conv = conversations.get(id);
  if (!conv) return null;
  conv.title = title;
  conv.updatedAt = new Date();
  return conv;
}

export async function chat(
  conversationId: string,
  userMessage: string,
  onChunk: (chunk: StreamChunk) => void
): Promise<string> {
  const conversation = conversations.get(conversationId);
  if (!conversation) {
    throw new Error(`Conversation not found: ${conversationId}`);
  }

  // 添加用户消息
  conversation.messages.push({
    role: "user",
    content: userMessage,
  });

  try {
    // 获取模型适配器
    const adapter = getAdapter(conversation.modelId);

    // 流式调用
    let fullResponse = "";
    await adapter.stream(conversation.messages, agentTools, (chunk) => {
      if (chunk.type === "text" && chunk.content) {
        fullResponse += chunk.content;
      }
      onChunk(chunk);
    });

    // 添加助手回复
    conversation.messages.push({
      role: "assistant",
      content: fullResponse,
    });

    // 如果是第一条消息，自动生成标题
    if (conversation.messages.length === 3) {
      // system + user + assistant
      conversation.title = userMessage.slice(0, 30) + (userMessage.length > 30 ? "..." : "");
    }

    conversation.updatedAt = new Date();
    return fullResponse;
  } catch (error) {
    // 移除失败的用户消息
    conversation.messages.pop();
    throw error;
  }
}

export function switchModel(conversationId: string, modelId: string): Conversation | null {
  const conversation = conversations.get(conversationId);
  if (!conversation) return null;
  conversation.modelId = modelId;
  conversation.updatedAt = new Date();
  return conversation;
}
