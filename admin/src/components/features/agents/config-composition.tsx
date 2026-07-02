import { Bot, Boxes, FileText, Layers3, ShieldCheck, Users, Wrench } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useGroupAssignments, usePersonas } from "@/hooks/use-personas"
import type { AgentProfile, GroupAssignment, Persona, SkillListItem } from "@/api/types"

const BUILTIN_TOOL_GROUPS = [
  {
    label: "消息与企微",
    tools: [
      "send_channel_message",
      "wecom_send_message",
      "wecom_search_targets",
      "wecom_list_or_get_conversations",
      "wecom_parse_message",
      "wecom_get_group_detail",
      "wecom_get_contact_detail",
      "wecom_revoke_message",
    ],
  },
  {
    label: "Skill 系统",
    tools: ["load_skill", "load_skill_reference", "run_script", "complete_skill"],
  },
  {
    label: "沙箱与文件",
    tools: [
      "execute_command",
      "sandbox_set_env",
      "sandbox_write_file",
      "sandbox_read_file",
      "sandbox_list_files",
      "read_file",
      "write_file",
      "list_dir",
      "grep",
      "shell",
    ],
  },
  {
    label: "记忆与任务",
    tools: ["save_memory", "recall_context", "set_delayed_task", "cancel_delayed_task", "list_delayed_tasks"],
  },
  {
    label: "扩展能力",
    tools: [
      "run_subagent_async",
      "get_subagent_status",
      "cancel_subagent",
      "inspect_attachment",
      "transcribe_audio_attachment",
      "image_generate",
      "render_card",
      "tavily_search",
    ],
  },
]

interface ConfigCompositionProps {
  agentId: string
  agent: AgentProfile
  draft: {
    displayName: string
    systemPrompt: string
    model: string
    sandboxTemplateId: string
    skills: string[]
  }
  skills?: SkillListItem[]
  fullPrompt: string
  previewLoading: boolean
}

function skillName(id: string, skillsByID: Map<string, SkillListItem>) {
  return skillsByID.get(id)?.name ?? id
}

function effectivePersonaSkills(persona: Persona | undefined, agentSkills: string[]) {
  return persona?.skills ?? agentSkills
}

function effectiveGroupSkills(assignment: GroupAssignment, personasByID: Map<string, Persona>, agentSkills: string[]) {
  const persona = assignment.personaId ? personasByID.get(assignment.personaId) : undefined
  return effectivePersonaSkills(persona, agentSkills)
}

function effectiveSandbox(
  assignment: GroupAssignment,
  personasByID: Map<string, Persona>,
  agentSandboxTemplateId: string,
) {
  if (assignment.sandboxTemplateId) return { value: assignment.sandboxTemplateId, source: "群组覆盖" }
  const persona = assignment.personaId ? personasByID.get(assignment.personaId) : undefined
  if (persona?.sandboxTemplateId) return { value: persona.sandboxTemplateId, source: "Persona" }
  return { value: agentSandboxTemplateId, source: "Agent 默认" }
}

function SkillBadges({ ids, skillsByID }: { ids: string[]; skillsByID: Map<string, SkillListItem> }) {
  if (ids.length === 0) {
    return <span className="text-xs text-amber-600">无</span>
  }
  return (
    <div className="flex flex-wrap gap-1">
      {ids.map((id) => (
        <Badge key={id} variant="outline" className="max-w-[220px] truncate">
          {skillName(id, skillsByID)}
        </Badge>
      ))}
    </div>
  )
}

function PromptBlock({ title, value }: { title: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-slate-500">
        <FileText className="h-3.5 w-3.5" />
        {title}
      </div>
      <pre className="max-h-80 overflow-y-auto rounded-md border border-slate-200 bg-slate-50 p-3 text-xs leading-5 text-slate-700 whitespace-pre-wrap wrap-break-word">
        {value || "（空）"}
      </pre>
    </div>
  )
}

export function ConfigComposition({
  agentId,
  agent,
  draft,
  skills,
  fullPrompt,
  previewLoading,
}: ConfigCompositionProps) {
  const { data: personas } = usePersonas(agentId)
  const { data: assignments } = useGroupAssignments(agentId)

  const agentSkills = draft.skills ?? []
  const skillsByID = new Map((skills ?? []).map((s) => [s.id, s]))
  const personasByID = new Map((personas ?? []).map((p) => [p.id, p]))
  const activePersonas = personas ?? []
  const activeAssignments = assignments ?? []

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Layers3 className="h-4 w-4 text-brand-600" />
            配置构成
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <div className="rounded-md border border-slate-200 p-3">
            <div className="mb-2 flex items-center gap-2 text-sm font-medium text-slate-900">
              <Bot className="h-4 w-4 text-slate-500" />
              Agent
            </div>
            <p className="text-sm text-slate-700">{draft.displayName || agent.displayName}</p>
            <p className="mt-1 text-xs text-slate-500">模型：{draft.model || agent.model || "未指定"}</p>
            <p className="mt-1 text-xs text-slate-500">环境：{draft.sandboxTemplateId || agent.sandboxTemplateId || "未指定"}</p>
          </div>
          <div className="rounded-md border border-slate-200 p-3">
            <div className="mb-2 flex items-center gap-2 text-sm font-medium text-slate-900">
              <ShieldCheck className="h-4 w-4 text-slate-500" />
              Persona
            </div>
            <p className="text-sm text-slate-700">{activePersonas.length} 个</p>
            <p className="mt-1 text-xs text-slate-500">可覆盖模型、环境、技能；不覆盖 system prompt。</p>
          </div>
          <div className="rounded-md border border-slate-200 p-3">
            <div className="mb-2 flex items-center gap-2 text-sm font-medium text-slate-900">
              <Users className="h-4 w-4 text-slate-500" />
              群组
            </div>
            <p className="text-sm text-slate-700">{activeAssignments.length} 个已分配</p>
            <p className="mt-1 text-xs text-slate-500">群组可指定 Persona，也可单独覆盖运行环境。</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">System Prompt</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 lg:grid-cols-2">
          <PromptBlock title="Agent 原始 Prompt" value={draft.systemPrompt} />
          <PromptBlock title="运行时最终 Prompt" value={previewLoading ? "加载中..." : fullPrompt} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Wrench className="h-4 w-4 text-brand-600" />
            内置 Tool
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {BUILTIN_TOOL_GROUPS.map((group) => (
            <div key={group.label} className="rounded-md border border-slate-200 p-3">
              <p className="mb-2 text-sm font-medium text-slate-900">{group.label}</p>
              <div className="flex flex-wrap gap-1">
                {group.tools.map((tool) => (
                  <Badge key={tool} variant="default" className="font-mono">
                    {tool}
                  </Badge>
                ))}
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Boxes className="h-4 w-4 text-brand-600" />
            Skill
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-md border border-slate-200 p-3">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm font-medium text-slate-900">Agent 默认技能</p>
              <Badge variant="brand">{agentSkills.length} 个</Badge>
            </div>
            <SkillBadges ids={agentSkills} skillsByID={skillsByID} />
          </div>

          {activePersonas.length > 0 && (
            <div className="space-y-2">
              <p className="text-sm font-medium text-slate-900">Persona 技能</p>
              {activePersonas.map((persona) => {
                const ids = effectivePersonaSkills(persona, agentSkills)
                return (
                  <div key={persona.id} className="rounded-md border border-slate-200 p-3">
                    <div className="mb-2 flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium text-slate-800">{persona.displayName}</span>
                      <Badge variant={persona.skills == null ? "outline" : "brand"}>
                        {persona.skills == null ? "继承 Agent" : `${ids.length} 个`}
                      </Badge>
                      {persona.model && <Badge variant="outline">{persona.model}</Badge>}
                      {persona.sandboxTemplateId && <Badge variant="outline">{persona.sandboxTemplateId}</Badge>}
                    </div>
                    <SkillBadges ids={ids} skillsByID={skillsByID} />
                  </div>
                )
              })}
            </div>
          )}

          {activeAssignments.length > 0 && (
            <div className="space-y-2">
              <p className="text-sm font-medium text-slate-900">群组最终生效</p>
              {activeAssignments.map((assignment) => {
                const persona = assignment.personaId ? personasByID.get(assignment.personaId) : undefined
                const sandbox = effectiveSandbox(assignment, personasByID, draft.sandboxTemplateId || agent.sandboxTemplateId || "")
                const ids = effectiveGroupSkills(assignment, personasByID, agentSkills)
                return (
                  <div key={assignment.id} className="rounded-md border border-slate-200 p-3">
                    <div className="mb-2 flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium text-slate-800">{assignment.groupName}</span>
                      <Badge variant="outline">{persona?.displayName ?? "未指定 Persona"}</Badge>
                      <Badge variant="outline">
                        {sandbox.source}：{sandbox.value || "未指定"}
                      </Badge>
                    </div>
                    <p className="mb-1.5 text-xs text-slate-500 font-mono break-all">{assignment.sessionKey}</p>
                    <SkillBadges ids={ids} skillsByID={skillsByID} />
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
