import * as React from "react"
import { cn } from "@/lib/utils"

type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement> & {
  autoResize?: boolean
  maxAutoResizeRows?: number
}

const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, autoResize = true, maxAutoResizeRows, onChange, ...props }, ref) => {
    const textareaRef = React.useRef<HTMLTextAreaElement | null>(null)

    const setRefs = React.useCallback(
      (node: HTMLTextAreaElement | null) => {
        textareaRef.current = node
        if (typeof ref === "function") {
          ref(node)
          return
        }
        if (ref) {
          ref.current = node
        }
      },
      [ref]
    )

    const resizeToContent = React.useCallback(() => {
      if (!autoResize || !textareaRef.current) return
      const textarea = textareaRef.current
      textarea.style.height = "auto"
      const nextHeight = textarea.scrollHeight

      if (!maxAutoResizeRows) {
        textarea.style.height = `${nextHeight}px`
        textarea.style.overflowY = "hidden"
        return
      }

      const styles = window.getComputedStyle(textarea)
      const lineHeight = Number.parseFloat(styles.lineHeight) || 20
      const paddingTop = Number.parseFloat(styles.paddingTop) || 0
      const paddingBottom = Number.parseFloat(styles.paddingBottom) || 0
      const borderTop = Number.parseFloat(styles.borderTopWidth) || 0
      const borderBottom = Number.parseFloat(styles.borderBottomWidth) || 0
      const maxHeight = lineHeight * maxAutoResizeRows + paddingTop + paddingBottom + borderTop + borderBottom

      textarea.style.height = `${Math.min(nextHeight, maxHeight)}px`
      textarea.style.overflowY = nextHeight > maxHeight ? "auto" : "hidden"
    }, [autoResize, maxAutoResizeRows])

    React.useLayoutEffect(() => {
      resizeToContent()
    }, [resizeToContent, props.value, props.defaultValue])

    const handleChange = (event: React.ChangeEvent<HTMLTextAreaElement>) => {
      resizeToContent()
      onChange?.(event)
    }

    return (
      <textarea
        className={cn(
          "flex min-h-[80px] w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 transition-colors duration-150 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:border-brand-500 disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500",
          autoResize && "overflow-hidden",
          className
        )}
        ref={setRefs}
        onChange={handleChange}
        {...props}
      />
    )
  }
)
Textarea.displayName = "Textarea"

export { Textarea }
