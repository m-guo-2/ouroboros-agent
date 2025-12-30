import { useState, useEffect, useRef, useCallback } from "react";
import { Sidebar } from "./components/Sidebar";
import { ChatMessage } from "./components/ChatMessage";
import { ChatInput } from "./components/ChatInput";
import { SettingsModal } from "./components/SettingsModal";
import { modelsApi, conversationsApi, streamChat } from "./api";
import type { Model, Conversation, Message } from "./api/types";
import "./App.css";

function App() {
  const [models, setModels] = useState<Model[]>([]);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [currentConversation, setCurrentConversation] = useState<Conversation | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [selectedModelId, setSelectedModelId] = useState<string>("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamingContent, setStreamingContent] = useState("");
  const [showSettings, setShowSettings] = useState(false);
  const [loading, setLoading] = useState(true);
  
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // 加载模型列表
  const loadModels = useCallback(async () => {
    const result = await modelsApi.getAll();
    if (result.success && result.data) {
      setModels(result.data);
      const enabled = result.data.filter((m) => m.enabled && m.configured);
      if (enabled.length > 0 && !selectedModelId) {
        setSelectedModelId(enabled[0].id);
      }
    }
  }, [selectedModelId]);

  // 加载会话列表
  const loadConversations = useCallback(async () => {
    const result = await conversationsApi.getAll();
    if (result.success && result.data) {
      setConversations(result.data);
    }
  }, []);

  // 初始化
  useEffect(() => {
    Promise.all([loadModels(), loadConversations()]).finally(() => {
      setLoading(false);
    });
  }, [loadModels, loadConversations]);

  // 自动滚动到底部
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, streamingContent]);

  // 创建新对话
  const handleNewChat = async () => {
    if (!selectedModelId) {
      setShowSettings(true);
      return;
    }
    
    const result = await conversationsApi.create(selectedModelId);
    if (result.success && result.data) {
      setCurrentConversation(result.data);
      setMessages([]);
      loadConversations();
    }
  };

  // 选择对话
  const handleSelectConversation = async (id: string) => {
    const result = await conversationsApi.getById(id);
    if (result.success && result.data) {
      setCurrentConversation(result.data);
      setMessages(result.data.messages || []);
      setSelectedModelId(result.data.modelId);
    }
  };

  // 删除对话
  const handleDeleteConversation = async (id: string) => {
    await conversationsApi.delete(id);
    if (currentConversation?.id === id) {
      setCurrentConversation(null);
      setMessages([]);
    }
    loadConversations();
  };

  // 切换模型
  const handleModelChange = async (modelId: string) => {
    setSelectedModelId(modelId);
    if (currentConversation) {
      await conversationsApi.switchModel(currentConversation.id, modelId);
    }
  };

  // 发送消息
  const handleSendMessage = async (content: string) => {
    if (!currentConversation) {
      // 自动创建新对话
      const result = await conversationsApi.create(selectedModelId);
      if (!result.success || !result.data) return;
      setCurrentConversation(result.data);
      await sendMessage(result.data.id, content);
    } else {
      await sendMessage(currentConversation.id, content);
    }
    loadConversations();
  };

  const sendMessage = async (conversationId: string, content: string) => {
    // 添加用户消息
    const userMessage: Message = { role: "user", content };
    setMessages((prev) => [...prev, userMessage]);
    
    setIsStreaming(true);
    setStreamingContent("");
    abortControllerRef.current = new AbortController();

    let fullContent = "";
    
    await streamChat(
      conversationId,
      content,
      (event) => {
        if (event.type === "text" && event.content) {
          fullContent += event.content;
          setStreamingContent(fullContent);
        } else if (event.type === "done") {
          setMessages((prev) => [...prev, { role: "assistant", content: fullContent }]);
          setStreamingContent("");
          setIsStreaming(false);
        } else if (event.type === "error") {
          setMessages((prev) => [
            ...prev,
            { role: "assistant", content: `错误: ${event.message}` },
          ]);
          setStreamingContent("");
          setIsStreaming(false);
        }
      },
      (error) => {
        setMessages((prev) => [
          ...prev,
          { role: "assistant", content: `错误: ${error.message}` },
        ]);
        setStreamingContent("");
        setIsStreaming(false);
      }
    );
  };

  // 停止生成
  const handleStopGeneration = () => {
    abortControllerRef.current?.abort();
    if (streamingContent) {
      setMessages((prev) => [...prev, { role: "assistant", content: streamingContent }]);
    }
    setStreamingContent("");
    setIsStreaming(false);
  };

  if (loading) {
    return (
      <div className="app loading">
        <div className="loading-spinner"></div>
        <p>加载中...</p>
      </div>
    );
  }

  const enabledModels = models.filter((m) => m.enabled && m.configured);
  const hasNoModels = enabledModels.length === 0;

  return (
    <div className="app">
      <Sidebar
        conversations={conversations}
        currentConversationId={currentConversation?.id || null}
        models={models}
        selectedModelId={selectedModelId}
        onNewChat={handleNewChat}
        onSelectConversation={handleSelectConversation}
        onDeleteConversation={handleDeleteConversation}
        onOpenSettings={() => setShowSettings(true)}
        onModelChange={handleModelChange}
      />

      <main className="chat-main">
        {hasNoModels ? (
          <div className="empty-state">
            <h2>欢迎使用 Agent</h2>
            <p>请先配置至少一个模型的 API Key</p>
            <button className="primary-button" onClick={() => setShowSettings(true)}>
              配置模型
            </button>
          </div>
        ) : !currentConversation && messages.length === 0 ? (
          <div className="empty-state">
            <h2>开始新对话</h2>
            <p>选择一个模型，然后输入你的问题</p>
          </div>
        ) : (
          <div className="messages-container">
            {messages.map((msg, index) => (
              <ChatMessage key={index} role={msg.role} content={msg.content} />
            ))}
            {isStreaming && streamingContent && (
              <ChatMessage role="assistant" content={streamingContent} isStreaming />
            )}
            <div ref={messagesEndRef} />
          </div>
        )}

        <ChatInput
          onSend={handleSendMessage}
          onStop={handleStopGeneration}
          disabled={hasNoModels}
          isStreaming={isStreaming}
          placeholder={hasNoModels ? "请先配置模型" : "输入消息..."}
        />
      </main>

      <SettingsModal
        isOpen={showSettings}
        onClose={() => setShowSettings(false)}
        models={models}
        onModelsUpdate={loadModels}
      />
    </div>
  );
}

export default App;
