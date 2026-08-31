// Adapted from beautifului.dev (MIT) by TurboProduct
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function StreamingText({ content, streaming }: { content: string; streaming?: boolean }) {
  return (
    <div className="text-sm leading-relaxed break-words [&_a]:font-medium [&_a]:underline [&_a]:underline-offset-4 [&_blockquote]:border-muted-foreground/40 [&_blockquote]:border-l-2 [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em] [&_h1,h2,h3]:mb-1 [&_h1,h2,h3]:font-semibold [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-1 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0 [&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-3 [&_pre]:text-xs [&_strong]:font-semibold [&_table]:w-full [&_table]:text-xs [&_td,th]:border [&_td,th]:border-border [&_td,th]:px-2 [&_td,th]:py-1 [&_ul]:list-disc [&_ul]:pl-5">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
      {streaming && (
        <span
          aria-hidden="true"
          className="ml-0.5 inline-block h-4 w-2 translate-y-0.5 animate-pulse rounded-[2px] bg-foreground"
        />
      )}
    </div>
  );
}
