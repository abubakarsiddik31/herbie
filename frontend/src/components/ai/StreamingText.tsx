// Adapted from beautifului.dev (MIT) by TurboProduct
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import { CodeBlock } from "@/components/ai/CodeBlock";
import { remarkCitations } from "@/features/chat/citations";

export interface CitationLinking {
  /** Number of sources attached to the message; brackets above this stay plain text. */
  count: number;
  onCite: (n: number) => void;
}

export function StreamingText({
  content,
  streaming,
  citations,
}: {
  content: string;
  streaming?: boolean;
  citations?: CitationLinking;
}) {
  return (
    <div className="text-sm leading-relaxed break-words [&_a]:font-medium [&_a]:underline [&_a]:underline-offset-4 [&_blockquote]:border-muted-foreground/40 [&_blockquote]:border-l-2 [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em] [&_h1,h2,h3]:mb-1 [&_h1,h2,h3]:font-semibold [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-1 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0 [&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-3 [&_pre]:text-xs [&_strong]:font-semibold [&_table]:w-full [&_table]:text-xs [&_td,th]:border [&_td,th]:border-border [&_td,th]:px-2 [&_td,th]:py-1 [&_ul]:list-disc [&_ul]:pl-5">
      <ReactMarkdown
        remarkPlugins={citations ? [remarkGfm, () => remarkCitations(citations.count)] : [remarkGfm]}
        // Let the internal cite: scheme through; everything else gets the
        // default protocol sanitization.
        urlTransform={(url) => (url.startsWith("cite:") ? url : defaultUrlTransform(url))}
        components={{
          pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
          a: ({ href, children }) => {
            const cite = citations && href?.startsWith("cite:") ? parseInt(href.slice(5), 10) : NaN;
            if (citations && Number.isInteger(cite)) {
              return (
                <button
                  type="button"
                  aria-label={`View source ${cite}`}
                  title={`View source ${cite}`}
                  onClick={() => citations.onCite(cite)}
                  className="cursor-pointer font-medium text-primary underline underline-offset-4"
                >
                  {children}
                </button>
              );
            }
            return <a href={href}>{children}</a>;
          },
        }}
      >
        {content}
      </ReactMarkdown>
      {streaming && (
        <span
          aria-hidden="true"
          className="ml-0.5 inline-block h-4 w-2 translate-y-0.5 animate-pulse rounded-[2px] bg-foreground"
        />
      )}
    </div>
  );
}
