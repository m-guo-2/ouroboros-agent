import { Plus, MessageSquare, Settings, Trash2, ChevronDown } from "lucide-react";
import type { Conversation, Model } from "../api/types";

interface SidebarProps {
  conversations: Conversation[];
  currentConversationId: string | null;
  models: Model[];
  selectedModelId: string;
  onNewChat: () => void;
  onSelectConversation: (id: string) => void;
  onDeleteConversation: (id: string) => void;
  onOpenSettings: () => void;
  onModelChange: (modelId: string) => void;
}

export function Sidebar({
  conversations,
  currentConversationId,
  models,
  selectedModelId,
  onNewChat,
  onSelectConversation,
  onDeleteConversation,
  onOpenSettings,
  onModelChange,
}: SidebarProps) {
  const enabledModels = models.filter((m) => m.enabled && m.configured);

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <h1 className="logo">Agent</h1>
        <button className="new-chat-button" onClick={onNewChat}>
          <Plus size={18} />
          <span>新对话</span>
        </button>
      </div>

      <div className="model-selector">
        <label className="model-label">模型</label>
        <div className="select-wrapper">
          <select
            value={selectedModelId}
            onChange={(e) => onModelChange(e.target.value)}
            className="model-select"
          >
            {enabledModels.length === 0 ? (
              <option value="">请先配置模型</option>
            ) : (
              enabledModels.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.name}
                </option>
              ))
            )}
          </select>
          <ChevronDown size={16} className="select-icon" />
        </div>
      </div>

      <div className="conversations-list">
        <div className="conversations-header">历史对话</div>
        {conversations.length === 0 ? (
          <div className="no-conversations">暂无对话</div>
        ) : (
          conversations.map((conv) => (
            <div
              key={conv.id}
              className={`conversation-item ${
                conv.id === currentConversationId ? "active" : ""
              }`}
              onClick={() => onSelectConversation(conv.id)}
            >
              <MessageSquare size={16} />
              <span className="conversation-title">{conv.title}</span>
              <button
                className="delete-button"
                onClick={(e) => {
                  e.stopPropagation();
                  onDeleteConversation(conv.id);
                }}
                title="删除对话"
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))
        )}
      </div>

      <div className="sidebar-footer">
        <button className="settings-button" onClick={onOpenSettings}>
          <Settings size={18} />
          <span>模型设置</span>
        </button>
      </div>
    </aside>
  );
}
