import { useState } from "react"
import { Link } from "react-router-dom"
import { Bot, Plus, Pencil, Trash2, Zap } from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { EmptyState } from "@/components/layout/empty-state"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { ChannelBadge } from "@/components/shared/channel-badge"
import { useAgents, useDeleteAgent } from "@/hooks/use-agents"
import { AgentFormDialog } from "./agent-form"

export function AgentList() {
  const { data: agents, isLoading } = useAgents()
  const deleteMutation = useDeleteAgent()
  const [showCreate, setShowCreate] = useState(false)

  if (isLoading) {
    return (
      <div>
        <PageHeader title="Agents" description="管理 AI Agent 配置" />
        <div className="mt-6 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-40 rounded-lg" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Agents"
        description="管理 AI Agent 配置"
        actions={
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4" />
            新建 Agent
          </Button>
        }
      />

      {!agents || agents.length === 0 ? (
        <EmptyState
          icon={Bot}
          title="还没有 Agent"
          description="创建你的第一个 AI Agent"
          action={
            <Button onClick={() => setShowCreate(true)}>
              <Plus className="h-4 w-4" />
              新建 Agent
            </Button>
          }
          className="mt-12"
        />
      ) : (
        <div className="mt-6 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {agents.map((agent) => (
            <Card key={agent.id} className="group hover:shadow-md transition-shadow duration-200">
              <CardContent className="p-4">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-50">
                      <Bot className="h-5 w-5 text-brand-600" />
                    </div>
                    <div>
                      <h3 className="text-sm font-semibold text-slate-900">{agent.displayName}</h3>
                      <p className="text-xs text-slate-400">{agent.id.slice(0, 8)}</p>
                    </div>
                  </div>
                  <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Link to={`/agents/${agent.id}`}>
                      <Button variant="ghost" size="icon-sm"><Pencil className="h-3.5 w-3.5" /></Button>
                    </Link>
                    {!agent.isDefault && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => { if (confirm("确认删除?")) deleteMutation.mutate(agent.id) }}
                      >
                        <Trash2 className="h-3.5 w-3.5 text-red-500" />
                      </Button>
                    )}
                  </div>
                </div>

                {/* Tags */}
                <div className="mt-3 flex flex-wrap items-center gap-1.5">
                  {agent.isActive !== false && (
                    <Badge variant="success">
                      <Zap className="h-3 w-3 mr-0.5" />活跃
                    </Badge>
                  )}
                  {agent.isDefault && <Badge variant="brand">默认</Badge>}
                  {agent.channels?.map((ch) => (
                    <ChannelBadge key={`${ch.type}-${ch.identifier}`} channel={ch.type} />
                  ))}
                </div>

                {/* Model & Skills count */}
                <div className="mt-2 flex flex-wrap gap-2 text-xs text-slate-500">
                  {agent.model && (
                    <span className="font-mono bg-slate-100 px-1.5 py-0.5 rounded">{agent.model}</span>
                  )}
                  {agent.skills && agent.skills.length > 0 && (
                    <span>{agent.skills.length} 个技能</span>
                  )}
                </div>

                {/* System prompt preview */}
                {agent.systemPrompt && (
                  <p className="mt-2 text-xs text-slate-400 line-clamp-2">
                    {agent.systemPrompt}
                  </p>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <AgentFormDialog open={showCreate} onOpenChange={setShowCreate} />
    </div>
  )
}
