import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { apiFetch } from "@/lib/api";
import type { SharedThread } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { BrandMark } from "@/components/BrandMark";
import { StreamingText } from "@/components/ai/StreamingText";
import { SourceCards, type CiteJump } from "@/features/chat/SourceCards";

export function SharedPage() {
  const { token } = useParams();
  const [citeJump, setCiteJump] = useState<(CiteJump & { msgId: string }) | null>(null);
  const thread = useQuery({
    queryKey: ["shared", token],
    queryFn: () => apiFetch<SharedThread>(`/api/shared/${token}`),
    retry: false,
  });

  if (thread.isPending) {
    return (
      <div className="mx-auto max-w-3xl space-y-4 p-6">
        <div className="h-7 w-1/2 animate-pulse rounded-md bg-muted" />
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-20 animate-pulse rounded-xl bg-muted" />
        ))}
      </div>
    );
  }

  if (thread.isError || !thread.data) {
    return (
      <div className="flex min-h-svh items-center justify-center p-4">
        <div className="max-w-sm space-y-3 text-center">
          <h1 className="text-2xl font-semibold">Link unavailable</h1>
          <p className="text-sm text-muted-foreground">
            This shared conversation does not exist or its link was revoked.
          </p>
          <Button asChild>
            <Link to="/">Open Golem</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-4 pt-6 pb-16">
      <header className="mb-6 flex items-center gap-2 border-b pb-4">
        <BrandMark className="size-7" />
        <h1 className="min-w-0 flex-1 truncate text-lg font-semibold">
          {thread.data.title || "Shared conversation"}
        </h1>
        <Badge variant="outline">Shared · view-only</Badge>
      </header>
      <div className="space-y-5">
        {thread.data.messages.map((m) =>
          m.role === "user" ? (
            <div key={m.id} className="flex justify-end">
              <div className="max-w-[75%] space-y-1.5">
                {m.images && m.images.length > 0 && (
                  <div className="flex flex-wrap justify-end gap-1.5">
                    {m.images.map((img, idx) => (
                      <img
                        key={idx}
                        src={img.dataUrl}
                        alt="attachment"
                        className="max-h-48 max-w-[16rem] rounded-xl border object-cover shadow-sm"
                      />
                    ))}
                  </div>
                )}
                {m.content && (
                  <div className="rounded-2xl rounded-br-md bg-primary px-4 py-2.5 whitespace-pre-wrap text-primary-foreground text-sm shadow-sm">
                    {m.content}
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div key={m.id} className="flex gap-2.5">
              <BrandMark className="mt-0.5 size-7 shrink-0" />
              <div className="min-w-0 flex-1 space-y-1 pt-0.5">
                <StreamingText
                  content={m.content}
                  streaming={false}
                  citations={
                    m.sources && m.sources.length > 0
                      ? {
                          count: m.sources.length,
                          onCite: (n) =>
                            setCiteJump((j) => ({ msgId: m.id, n, seq: (j?.seq ?? 0) + 1 })),
                        }
                      : undefined
                  }
                />
                {m.sources && m.sources.length > 0 && (
                  <SourceCards sources={m.sources} jump={citeJump?.msgId === m.id ? citeJump : null} />
                )}
                {m.truncated && (
                  <Badge variant="outline" className="text-muted-foreground text-xs">
                    stopped early
                  </Badge>
                )}
              </div>
            </div>
          ),
        )}
      </div>
    </div>
  );
}
