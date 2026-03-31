import { useState } from "react"
import { Plus, Trash2, Zap } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { useHookEvents } from "@/hooks/use-agents"
import { useSkills } from "@/hooks/use-skills"
import type { Hook, HookAction } from "@/api/types"

interface AgentHooksProps {
  hooks: Hook[]
  onChange: (hooks: Hook[]) => void
}

export function AgentHooks({ hooks, onChange }: AgentHooksProps) {
  const { data: hookEvents } = useHookEvents()
  const { data: skills } = useSkills()
  const [adding, setAdding] = useState(false)
  const [formEvent, setFormEvent] = useState("")
  const [formCustomEvent, setFormCustomEvent] = useState("")
  const [formActions, setFormActions] = useState<HookAction[]>([{ type: "activate_skill", skillId: "" }])

  const eventIsCustom = formEvent === "__custom__"
  const effectiveEvent = eventIsCustom ? formCustomEvent : formEvent

  const removeHook = (index: number) => {
    onChange(hooks.filter((_, i) => i !== index))
  }

  const resetForm = () => {
    setFormEvent("")
    setFormCustomEvent("")
    setFormActions([{ type: "activate_skill", skillId: "" }])
    setAdding(false)
  }

  const addHook = () => {
    if (!effectiveEvent) return
    const validActions = formActions.filter((a) => a.skillId)
    if (validActions.length === 0) return
    onChange([...hooks, { event: effectiveEvent, actions: validActions }])
    resetForm()
  }

  const updateFormAction = (index: number, patch: Partial<HookAction>) => {
    setFormActions((prev) => prev.map((a, i) => (i === index ? { ...a, ...patch } : a)))
  }

  const removeFormAction = (index: number) => {
    setFormActions((prev) => prev.filter((_, i) => i !== index))
  }

  const canSubmit = effectiveEvent && formActions.some((a) => a.skillId)

  const getEventLabel = (name: string) => {
    const ev = hookEvents?.find((e) => e.name === name)
    return ev ? ev.label : null
  }

  if (hooks.length === 0 && !adding) {
    return (
      <Card>
        <CardContent className="p-6">
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-slate-100 mb-4">
              <Zap className="h-7 w-7 text-slate-400" />
            </div>
            <p className="text-sm font-semibold text-slate-700 mb-1.5">还没有配置 Hook</p>
            <p className="text-xs text-slate-400 max-w-xs mb-5 leading-relaxed">
              Hook 可以在特定事件发生时自动激活临时技能。
              例如：新会话开始时自动加载「破冰引导」技能。
            </p>
            <Button size="sm" onClick={() => setAdding(true)}>
              <Plus className="h-4 w-4" />
              添加 Hook
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>事件 Hook</CardTitle>
            <p className="text-xs text-slate-400 mt-1">当事件触发时，自动执行配置的动作</p>
          </div>
          {!adding && (
            <Button size="sm" onClick={() => setAdding(true)}>
              <Plus className="h-3.5 w-3.5" />
              添加
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-2.5">
        {hooks.map((hook, idx) => (
          <div
            key={idx}
            className="rounded-lg border border-slate-200 p-3.5 hover:border-slate-300 transition-colors"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Badge variant="brand" className="font-mono text-xs px-2.5 py-0.5">
                  <Zap className="h-3 w-3 mr-1" />
                  {hook.event}
                </Badge>
                {getEventLabel(hook.event) && (
                  <span className="text-xs text-slate-400">{getEventLabel(hook.event)}</span>
                )}
              </div>
              <button
                type="button"
                onClick={() => removeHook(idx)}
                className="p-1 rounded-md text-slate-400 hover:text-red-500 hover:bg-red-50 transition-colors cursor-pointer"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="mt-2.5 pl-3 border-l-2 border-slate-100 space-y-1.5">
              {hook.actions.map((action, aIdx) => (
                <div key={aIdx} className="flex items-center gap-2 text-xs">
                  <span className="text-slate-400">activate_skill</span>
                  <span className="text-slate-300">&rarr;</span>
                  <span className="font-medium text-brand-700 bg-brand-50 px-2 py-0.5 rounded">
                    {action.skillId}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ))}

        {adding && (
          <div className="rounded-lg border border-dashed border-slate-300 bg-slate-50/50 p-4 space-y-3">
            {/* Event selector */}
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-slate-600">事件</label>
              <select
                className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
                value={formEvent}
                onChange={(e) => {
                  setFormEvent(e.target.value)
                  if (e.target.value !== "__custom__") setFormCustomEvent("")
                }}
              >
                <option value="">选择事件...</option>
                {hookEvents?.map((ev) => (
                  <option key={ev.name} value={ev.name}>
                    {ev.label}（{ev.name}）
                  </option>
                ))}
                <option value="__custom__">自定义事件...</option>
              </select>
              {eventIsCustom && (
                <input
                  type="text"
                  className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-brand-500"
                  placeholder="输入自定义事件名 e.g. member_joined"
                  value={formCustomEvent}
                  onChange={(e) => setFormCustomEvent(e.target.value)}
                  autoFocus
                />
              )}
              {!eventIsCustom && formEvent && hookEvents && (
                <p className="text-[11px] text-slate-400 pl-0.5">
                  {hookEvents.find((e) => e.name === formEvent)?.description}
                </p>
              )}
            </div>

            {/* Actions */}
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-slate-600">动作</label>
              {formActions.map((action, aIdx) => (
                <div key={aIdx} className="flex items-center gap-2">
                  <select
                    className="h-8 rounded-md border border-slate-300 bg-white px-2 text-xs focus:outline-none focus:ring-2 focus:ring-brand-500 w-36 shrink-0"
                    value={action.type}
                    onChange={(e) => updateFormAction(aIdx, { type: e.target.value })}
                  >
                    <option value="activate_skill">activate_skill</option>
                  </select>
                  <span className="text-slate-300 text-xs">&rarr;</span>
                  <select
                    className="flex-1 h-8 rounded-md border border-slate-300 bg-white px-2 text-xs focus:outline-none focus:ring-2 focus:ring-brand-500"
                    value={action.skillId ?? ""}
                    onChange={(e) => updateFormAction(aIdx, { skillId: e.target.value })}
                  >
                    <option value="">选择技能...</option>
                    {skills?.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                  </select>
                  {formActions.length > 1 && (
                    <button
                      type="button"
                      onClick={() => removeFormAction(aIdx)}
                      className="p-1 rounded text-slate-400 hover:text-red-500 hover:bg-red-50 transition-colors cursor-pointer"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
              ))}
              <button
                type="button"
                onClick={() => setFormActions((prev) => [...prev, { type: "activate_skill", skillId: "" }])}
                className="text-xs text-brand-600 hover:text-brand-700 font-medium cursor-pointer mt-1 flex items-center gap-1"
              >
                <Plus className="h-3 w-3" />
                添加动作
              </button>
            </div>

            {/* Form actions */}
            <div className="flex justify-end gap-2 pt-2 border-t border-slate-200">
              <Button variant="secondary" size="sm" onClick={resetForm}>
                取消
              </Button>
              <Button size="sm" onClick={addHook} disabled={!canSubmit}>
                确认添加
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
