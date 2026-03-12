import type { MessageData, ExecutionStep, CompactionData } from "@/api/types"

export interface ToolPair {
  call: ExecutionStep
  result?: ExecutionStep
}

export interface IterationData {
  iteration: number
  llmCall?: ExecutionStep
  systemSteps: ExecutionStep[]
  thinkings: ExecutionStep[]
  toolPairs: ToolPair[]
  contentSteps: ExecutionStep[]
  errorSteps: ExecutionStep[]
}

export type FlatEvent =
  | { type: "model-output"; thinkings: ExecutionStep[]; llmCall?: ExecutionStep }
  | { type: "tool-call"; step: ExecutionStep }
  | { type: "tool-result"; step: ExecutionStep; callStep?: ExecutionStep }
  | { type: "error"; step: ExecutionStep }

export interface MessageExchange {
  userMessage: Pick<MessageData, "role" | "content"> & Partial<MessageData>
  traceId?: string
  assistantMessage?: MessageData
  isSystemInitiated?: boolean
  exchangeIndex: number
}

export type TimelineEvent =
  | { type: "user-message"; message: MessageData; exchangeIndex: number }
  | { type: "assistant-message"; message: MessageData; traceId?: string; exchangeIndex: number }
  | { type: "processing"; traceId?: string; exchangeIndex: number }
  | { type: "compaction"; data: CompactionData }
  | { type: "absorb"; round: number; count: number; traceId: string; timestamp: number }

export interface RoundData {
  roundNumber: number
  absorbedCount?: number
  steps: ExecutionStep[]
}
