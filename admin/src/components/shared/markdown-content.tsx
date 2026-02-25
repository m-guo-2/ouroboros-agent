import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { oneLight } from "react-syntax-highlighter/dist/esm/styles/prism"

interface MarkdownContentProps {
  content: string
  className?: string
}

export function MarkdownContent({ content, className }: MarkdownContentProps) {
  return (
    <div className={className}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          code({ className: codeClassName, children, ...props }) {
            const match = /language-(\w+)/.exec(codeClassName || "")
            const isInline = !match
            if (isInline) {
              return (
                <code className="rounded bg-slate-100 px-1.5 py-0.5 text-[13px] font-mono text-slate-800" {...props}>
                  {children}
                </code>
              )
            }
            return (
              <SyntaxHighlighter
                style={oneLight}
                language={match[1]}
                PreTag="div"
                className="!rounded-lg !text-[13px] !my-3"
              >
                {String(children).replace(/\n$/, "")}
              </SyntaxHighlighter>
            )
          },
          p({ children }) {
            return <p className="mb-2 last:mb-0 text-sm leading-relaxed">{children}</p>
          },
          ul({ children }) {
            return <ul className="mb-2 list-disc pl-5 text-sm">{children}</ul>
          },
          ol({ children }) {
            return <ol className="mb-2 list-decimal pl-5 text-sm">{children}</ol>
          },
          h1({ children }) {
            return <h1 className="mb-2 text-lg font-semibold">{children}</h1>
          },
          h2({ children }) {
            return <h2 className="mb-2 text-base font-semibold">{children}</h2>
          },
          h3({ children }) {
            return <h3 className="mb-2 text-sm font-semibold">{children}</h3>
          },
          a({ children, href }) {
            return (
              <a href={href} className="text-brand-600 hover:underline" target="_blank" rel="noopener noreferrer">
                {children}
              </a>
            )
          },
          table({ children }) {
            return (
              <div className="overflow-x-auto my-2">
                <table className="min-w-full text-sm border border-slate-200 rounded">{children}</table>
              </div>
            )
          },
          th({ children }) {
            return <th className="border-b border-slate-200 bg-slate-50 px-3 py-1.5 text-left text-xs font-medium">{children}</th>
          },
          td({ children }) {
            return <td className="border-b border-slate-100 px-3 py-1.5">{children}</td>
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}
