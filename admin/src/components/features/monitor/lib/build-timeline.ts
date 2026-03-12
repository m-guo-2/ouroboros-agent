import type { MessageData, ExecutionStep, CompactionData } from "@/api/types"
import type { MessageExchange, TimelineEvent, RoundData, IterationData, FlatEvent } from "./types"

function isTextAssistantMessage(message: MessageData): boolean {
  return message.role === "assistant" && message.messageType !== "structured" && !!message.content?.trim()
}

export function buildExchanges(messages: MessageData[]): MessageExchange[] {
  const exchanges: MessageExchange[] = []
  let i = 0
  let exchangeIdx = 0

  while (i < messages.length) {
    const msg = messages[i]

    if (msg.role === "user") {
      const exchange: MessageExchange = {
        userMessage: msg,
        traceId: msg.traceId,
        exchangeIndex: exchangeIdx++,
      }
      let j = i + 1
      let lastAssistantText: MessageData | undefined
      for (; j < messages.length && messages[j].role !== "user"; j++) {
        const candidate = messages[j]
        if (!isTextAssistantMessage(candidate)) continue
        if (!msg.traceId || !candidate.traceId || candidate.traceId === msg.traceId) {
          lastAssistantText = candidate
        }
      }
      if (lastAssistantText) {
        exchange.assistantMessage = lastAssistantText
        exchange.traceId = exchange.traceId ?? lastAssistantText.traceId
      }
      i = j
      exchanges.push(exchange)
    } else {
      if (!isTextAssistantMessage(msg)) {
        i += 1
        continue
      }
      exchanges.push({
        userMessage: {
          role: "user" as const,
          content: "(系统触发)",
        },
        assistantMessage: msg,
        traceId: msg.traceId,
        isSystemInitiated: true,
        exchangeIndex: exchangeIdx++,
      })
      i += 1
    }
  }
  return exchanges
}

export function buildTimeline(
  exchanges: MessageExchange[],
  compactions: CompactionData[],
): TimelineEvent[] {
  const events: TimelineEvent[] = []

  const compactionsByTime = [...compactions].sort(
    (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
  )
  let cIdx = 0

  for (const exchange of exchanges) {
    const exchangeTime = exchange.userMessage.createdAt
      ? new Date(exchange.userMessage.createdAt).getTime()
      : 0

    while (cIdx < compactionsByTime.length) {
      const cTime = new Date(compactionsByTime[cIdx].createdAt).getTime()
      if (cTime < exchangeTime) {
        events.push({ type: "compaction", data: compactionsByTime[cIdx] })
        cIdx++
      } else {
        break
      }
    }

    events.push({
      type: "user-message",
      message: exchange.userMessage as MessageData,
      exchangeIndex: exchange.exchangeIndex,
    })

    if (exchange.assistantMessage) {
      events.push({
        type: "assistant-message",
        message: exchange.assistantMessage,
        traceId: exchange.assistantMessage.traceId,
        exchangeIndex: exchange.exchangeIndex,
      })
    } else if (exchange.traceId) {
      events.push({
        type: "processing",
        traceId: exchange.traceId,
        exchangeIndex: exchange.exchangeIndex,
      })
    }
  }

  while (cIdx < compactionsByTime.length) {
    events.push({ type: "compaction", data: compactionsByTime[cIdx] })
    cIdx++
  }

  return events
}

export function splitIntoRounds(steps: ExecutionStep[]): RoundData[] {
  const rounds: RoundData[] = []
  let current: ExecutionStep[] = []
  let roundNum = 1

  for (const step of steps) {
    if (step.type === "absorb") {
      rounds.push({ roundNumber: roundNum, steps: current })
      roundNum++
      current = []
      rounds.push({
        roundNumber: roundNum,
        absorbedCount: step.absorbedCount,
        steps: [],
      })
      continue
    }
    current.push(step)
  }

  if (rounds.length === 0) {
    return [{ roundNumber: 1, steps: current }]
  }

  const lastRound = rounds[rounds.length - 1]
  lastRound.steps = current
  return rounds.filter(r => r.steps.length > 0 || r.absorbedCount)
}

export function groupStepsByIteration(steps: ExecutionStep[]): IterationData[] {
  const iterMap = new Map<number, IterationData>()
  const pendingToolCalls = new Map<string, { iterIdx: number; pairIdx: number }>()

  const getOrCreate = (iter: number): IterationData => {
    if (!iterMap.has(iter)) {
      iterMap.set(iter, {
        iteration: iter,
        systemSteps: [],
        thinkings: [],
        toolPairs: [],
        contentSteps: [],
        errorSteps: [],
      })
    }
    return iterMap.get(iter)!
  }

  for (const step of steps) {
    if (step.type === "absorb" || step.type === "compact") continue
    const iter = step.iteration ?? 0
    const data = getOrCreate(iter)

    if (step.type === "llm_call") {
      data.llmCall = step
    } else if (step.type === "thinking") {
      if (step.source === "system") data.systemSteps.push(step)
      else data.thinkings.push(step)
    } else if (step.type === "tool_call") {
      const pairIdx = data.toolPairs.length
      data.toolPairs.push({ call: step })
      if (step.toolCallId) pendingToolCalls.set(step.toolCallId, { iterIdx: iter, pairIdx })
    } else if (step.type === "tool_result") {
      const loc = step.toolCallId ? pendingToolCalls.get(step.toolCallId) : undefined
      if (loc && iterMap.has(loc.iterIdx)) {
        iterMap.get(loc.iterIdx)!.toolPairs[loc.pairIdx].result = step
        if (step.toolCallId) pendingToolCalls.delete(step.toolCallId)
      }
    } else if (step.type === "content") {
      data.contentSteps.push(step)
    } else if (step.type === "error") {
      data.errorSteps.push(step)
    }
  }

  return Array.from(iterMap.values()).sort((a, b) => a.iteration - b.iteration)
}

/**
 * Flatten trace steps into a sequential list of events for display.
 * Merges thinking + llm_call of the same iteration into a single "model-output" event.
 * Skips absorb/compact (handled elsewhere).
 */
export function flattenSteps(steps: ExecutionStep[]): FlatEvent[] {
  const events: FlatEvent[] = []
  const toolCallMap = new Map<string, ExecutionStep>()

  // Group thinkings and llm_call by iteration
  const iterThinkings = new Map<number, ExecutionStep[]>()
  const iterLLMCall = new Map<number, ExecutionStep>()

  for (const step of steps) {
    if (step.type === "absorb" || step.type === "compact") continue

    if (step.type === "thinking") {
      const iter = step.iteration ?? 0
      if (!iterThinkings.has(iter)) iterThinkings.set(iter, [])
      iterThinkings.get(iter)!.push(step)
    } else if (step.type === "llm_call") {
      iterLLMCall.set(step.iteration ?? 0, step)
    } else if (step.type === "tool_call") {
      if (step.toolCallId) toolCallMap.set(step.toolCallId, step)
    }
  }

  // Build events in timestamp order
  const sorted = [...steps].filter(s => s.type !== "absorb" && s.type !== "compact")
  const emittedIterations = new Set<number>()

  for (const step of sorted) {
    const iter = step.iteration ?? 0

    if (step.type === "thinking" || step.type === "llm_call") {
      if (emittedIterations.has(iter)) continue
      emittedIterations.add(iter)
      events.push({
        type: "model-output",
        thinkings: iterThinkings.get(iter) ?? [],
        llmCall: iterLLMCall.get(iter),
      })
    } else if (step.type === "tool_call") {
      events.push({ type: "tool-call", step })
    } else if (step.type === "tool_result") {
      const callStep = step.toolCallId ? toolCallMap.get(step.toolCallId) : undefined
      events.push({ type: "tool-result", step, callStep })
    } else if (step.type === "error") {
      events.push({ type: "error", step })
    }
  }

  return events
}
