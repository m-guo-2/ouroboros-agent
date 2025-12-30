import { useState, useEffect } from "react";
import { X, Eye, EyeOff, Check, AlertCircle, RefreshCw, ChevronDown } from "lucide-react";
import type { Model } from "../api/types";
import { modelsApi, type AvailableModel } from "../api/client";

interface SettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  models: Model[];
  onModelsUpdate: () => void;
}

// Updated: force rebuild v2
export function SettingsModal({
  isOpen,
  onClose,
  models,
  onModelsUpdate,
}: SettingsModalProps) {
  const [selectedModel, setSelectedModel] = useState<Model | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [selectedModelId, setSelectedModelId] = useState("");
  const [showApiKey, setShowApiKey] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loadingModels, setLoadingModels] = useState(false);
  const [availableModels, setAvailableModels] = useState<AvailableModel[]>([]);
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);

  useEffect(() => {
    if (selectedModel) {
      setApiKey("");
      setBaseUrl(selectedModel.baseUrl || "");
      setSelectedModelId(selectedModel.model || "");
      setShowApiKey(false);
      setMessage(null);
      setAvailableModels([]);
    }
  }, [selectedModel]);

  useEffect(() => {
    if (models.length > 0 && !selectedModel) {
      setSelectedModel(models[0]);
    }
  }, [models, selectedModel]);

  if (!isOpen) return null;

  const fetchModels = async () => {
    if (!selectedModel) return;

    setLoadingModels(true);
    setMessage(null);

    try {
      const result = await modelsApi.getAvailableModels(selectedModel.id);
      if (result.success && result.data) {
        setAvailableModels(result.data);
        if (result.data.length > 0) {
          setMessage({ type: "success", text: `获取到 ${result.data.length} 个可用模型` });
        }
      } else {
        setMessage({ type: "error", text: result.error || "获取模型列表失败" });
      }
    } catch {
      setMessage({ type: "error", text: "网络错误" });
    } finally {
      setLoadingModels(false);
    }
  };

  const handleSave = async () => {
    if (!selectedModel) return;

    setSaving(true);
    setMessage(null);

    try {
      const updates: Record<string, unknown> = {};
      if (apiKey) updates.apiKey = apiKey;
      if (baseUrl !== selectedModel.baseUrl) updates.baseUrl = baseUrl;
      if (selectedModelId && selectedModelId !== selectedModel.model) {
        updates.model = selectedModelId;
      }

      const result = await modelsApi.update(selectedModel.id, updates);
      if (result.success) {
        setMessage({ type: "success", text: "保存成功" });
        setApiKey("");
        onModelsUpdate();
      } else {
        setMessage({ type: "error", text: result.error || "保存失败" });
      }
    } catch {
      setMessage({ type: "error", text: "网络错误" });
    } finally {
      setSaving(false);
    }
  };

  const handleToggleEnabled = async (model: Model) => {
    try {
      await modelsApi.update(model.id, { enabled: !model.enabled });
      onModelsUpdate();
    } catch {
      // 忽略错误
    }
  };

  const hasChanges = apiKey || baseUrl !== (selectedModel?.baseUrl || "") || selectedModelId !== (selectedModel?.model || "");

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content settings-modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>模型设置</h2>
          <button className="close-button" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="settings-layout">
          <div className="model-list">
            {models.map((model) => (
              <div
                key={model.id}
                className={`model-list-item ${
                  selectedModel?.id === model.id ? "active" : ""
                }`}
                onClick={() => setSelectedModel(model)}
              >
                <div className="model-info">
                  <span className="model-name">{model.name}</span>
                  <span className={`model-status ${model.configured ? "configured" : ""}`}>
                    {model.configured ? "已配置" : "未配置"}
                  </span>
                </div>
                <label className="toggle-switch" onClick={(e) => e.stopPropagation()}>
                  <input
                    type="checkbox"
                    checked={model.enabled}
                    onChange={() => handleToggleEnabled(model)}
                  />
                  <span className="toggle-slider"></span>
                </label>
              </div>
            ))}
          </div>

          {selectedModel && (
            <div className="model-config">
              <h3>{selectedModel.name}</h3>
              <p className="model-provider">Provider: {selectedModel.provider}</p>

              <div className="form-group">
                <label>API Key</label>
                <div className="input-with-icon">
                  <input
                    type={showApiKey ? "text" : "password"}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder={selectedModel.hasApiKey ? "已配置（输入新值覆盖）" : "请输入 API Key"}
                  />
                  <button
                    className="icon-button"
                    onClick={() => setShowApiKey(!showApiKey)}
                  >
                    {showApiKey ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </div>
              </div>

              {selectedModel.provider !== "claude" && (
                <div className="form-group">
                  <label>API Base URL</label>
                  <input
                    type="text"
                    value={baseUrl}
                    onChange={(e) => setBaseUrl(e.target.value)}
                    placeholder="可选，留空使用默认地址"
                  />
                </div>
              )}

              <div className="form-group">
                <label>
                  模型选择
                  <button
                    className="fetch-models-btn"
                    onClick={fetchModels}
                    disabled={loadingModels || !selectedModel.configured}
                    title={selectedModel.configured ? "获取可用模型列表" : "请先配置 API Key"}
                  >
                    <RefreshCw size={14} className={loadingModels ? "spinning" : ""} />
                    获取模型列表
                  </button>
                </label>
                
                {availableModels.length > 0 ? (
                  <div className="model-select-wrapper">
                    <select
                      value={selectedModelId}
                      onChange={(e) => setSelectedModelId(e.target.value)}
                      className="model-select"
                    >
                      {availableModels.map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name || m.id}
                        </option>
                      ))}
                    </select>
                    <ChevronDown size={16} className="select-arrow" />
                  </div>
                ) : (
                  <input
                    type="text"
                    value={selectedModelId}
                    onChange={(e) => setSelectedModelId(e.target.value)}
                    placeholder="输入模型名称或点击上方按钮获取列表"
                  />
                )}
                
                {selectedModel.model && selectedModelId !== selectedModel.model && (
                  <span className="model-change-hint">
                    当前: {selectedModel.model} → 新: {selectedModelId}
                  </span>
                )}
              </div>

              {message && (
                <div className={`message ${message.type}`}>
                  {message.type === "success" ? (
                    <Check size={16} />
                  ) : (
                    <AlertCircle size={16} />
                  )}
                  {message.text}
                </div>
              )}

              <button
                className="save-button"
                onClick={handleSave}
                disabled={saving || !hasChanges}
              >
                {saving ? "保存中..." : "保存配置"}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
