import { ThumbsDown, ThumbsUp } from "lucide-react";
import { cn } from "@/lib/utils";
import { useMessageFeedback } from "./useConversations";

interface FeedbackButtonsProps {
  conversationId: string | null;
  messageId: string;
  value?: "up" | "down";
  onChange: (rating: "up" | "down" | undefined) => void;
}

// Thumbs are a toggle: clicking the active rating clears it. The parent owns
// the message state so the optimistic update is instant; a failed request
// restores the previous value.
export function FeedbackButtons({ conversationId, messageId, value, onChange }: FeedbackButtonsProps) {
  const feedback = useMessageFeedback(conversationId);

  const rate = (next: "up" | "down") => {
    const previous = value;
    const rating = value === next ? null : next;
    onChange(rating ?? undefined);
    feedback.mutate(
      { messageId, rating },
      { onError: () => onChange(previous) },
    );
  };

  return (
    <span className="flex items-center">
      {(["up", "down"] as const).map((rating) => {
        const active = value === rating;
        const Icon = rating === "up" ? ThumbsUp : ThumbsDown;
        return (
          <button
            key={rating}
            type="button"
            aria-label={rating === "up" ? "Good response" : "Bad response"}
            aria-pressed={active}
            title={rating === "up" ? "Good response" : "Bad response"}
            onClick={() => rate(rating)}
            className={cn(
              "rounded-md p-1 transition-colors",
              active
                ? rating === "up"
                  ? "text-emerald-600"
                  : "text-destructive"
                : "text-muted-foreground/60 hover:text-foreground",
            )}
          >
            <Icon className="size-3.5" />
          </button>
        );
      })}
    </span>
  );
}
