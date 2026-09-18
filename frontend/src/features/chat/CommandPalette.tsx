import { useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useTheme } from "next-themes";
import {
  Download,
  FileUp,
  Gauge,
  MessageSquare,
  Moon,
  Plus,
  Search,
  Settings,
  Share2,
  SlidersHorizontal,
  Sun,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { Conversation } from "@/lib/types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  conversations: Conversation[] | undefined;
  activeConversation: Conversation | undefined;
  onNewChat: () => void;
  onExport: () => void;
  onShare: () => void;
  onOpenSettings: () => void;
  onOpenShortcuts: () => void;
}

interface PaletteItem {
  id: string;
  category: "Actions" | "Navigation" | "Conversations";
  title: string;
  subtitle?: string;
  icon: LucideIcon;
  run: () => void;
}

export function CommandPalette({
  open,
  onOpenChange,
  conversations,
  activeConversation,
  onNewChat,
  onExport,
  onShare,
  onOpenSettings,
  onOpenShortcuts,
}: Props) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const navigate = useNavigate();
  const { theme, setTheme } = useTheme();
  const inputRef = useRef<HTMLInputElement>(null);

  const items = useMemo<PaletteItem[]>(() => {
    const list: PaletteItem[] = [
      {
        id: "new-chat",
        category: "Actions",
        title: "New chat",
        icon: Plus,
        run: onNewChat,
      },
      {
        id: "toggle-theme",
        category: "Actions",
        title: `Toggle theme (currently ${theme ?? "system"})`,
        icon: theme === "dark" ? Sun : Moon,
        run: () => setTheme(theme === "dark" ? "light" : "dark"),
      },
    ];

    if (activeConversation) {
      list.push(
        {
          id: "export-chat",
          category: "Actions",
          title: "Export conversation as Markdown",
          icon: Download,
          run: onExport,
        },
        {
          id: "share-chat",
          category: "Actions",
          title: "Share conversation link",
          icon: Share2,
          run: onShare,
        },
        {
          id: "chat-settings",
          category: "Actions",
          title: "Conversation settings (model & prompt)",
          icon: SlidersHorizontal,
          run: onOpenSettings,
        },
      );
    }

    list.push(
      {
        id: "nav-documents",
        category: "Navigation",
        title: "Documents library",
        icon: FileUp,
        run: () => navigate("/documents"),
      },
      {
        id: "nav-tools",
        category: "Navigation",
        title: "Custom HTTP tools",
        icon: Wrench,
        run: () => navigate("/tools"),
      },
      {
        id: "nav-usage",
        category: "Navigation",
        title: "Token usage & costs",
        icon: Gauge,
        run: () => navigate("/usage"),
      },
      {
        id: "nav-preferences",
        category: "Navigation",
        title: "Preferences (custom instructions)",
        icon: Settings,
        run: () => navigate("/preferences"),
      },
    );

    if (conversations && conversations.length > 0) {
      for (const c of conversations.slice(0, 15)) {
        list.push({
          id: `conv-${c.id}`,
          category: "Conversations",
          title: c.title || "Untitled conversation",
          subtitle: c.model || undefined,
          icon: MessageSquare,
          run: () => navigate(`/chat/${c.id}`),
        });
      }
    }

    list.push({
      id: "open-shortcuts",
      category: "Navigation",
      title: "Keyboard shortcuts",
      icon: Search,
      run: onOpenShortcuts,
    });

    const q = query.trim().toLowerCase();
    if (!q) return list;
    return list.filter(
      (item) =>
        item.title.toLowerCase().includes(q) ||
        item.category.toLowerCase().includes(q) ||
        (item.subtitle && item.subtitle.toLowerCase().includes(q)),
    );
  }, [
    activeConversation,
    conversations,
    navigate,
    onExport,
    onNewChat,
    onOpenSettings,
    onOpenShortcuts,
    onShare,
    query,
    setTheme,
    theme,
  ]);

  const activeIndex = items.length === 0 ? 0 : Math.min(selectedIndex, items.length - 1);

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((prev) => (items.length === 0 ? 0 : (prev + 1) % items.length));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((prev) => (items.length === 0 ? 0 : (prev - 1 + items.length) % items.length));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (items[activeIndex]) {
        onOpenChange(false);
        items[activeIndex].run();
      }
    }
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setQuery("");
      setSelectedIndex(0);
    }
    onOpenChange(nextOpen);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton={false} className="overflow-hidden p-0 sm:max-w-lg">
        <div className="flex items-center border-b px-3">
          <Search className="mr-2 size-4 shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
            onKeyDown={handleKeyDown}
            placeholder="Type a command or search conversations…"
            aria-label="Command palette input"
            className="h-12 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            autoFocus
          />
        </div>
        <ScrollArea className="max-h-80">
          <div className="p-2">
            {items.length === 0 ? (
              <p className="p-4 text-center text-sm text-muted-foreground">No commands found.</p>
            ) : (
              <div className="space-y-1">
                {items.map((item, idx) => {
                  const Icon = item.icon;
                  const isSelected = idx === activeIndex;
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => {
                        onOpenChange(false);
                        item.run();
                      }}
                      onMouseEnter={() => setSelectedIndex(idx)}
                      className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                        isSelected
                          ? "bg-accent text-accent-foreground"
                          : "text-foreground hover:bg-accent/50"
                      }`}
                    >
                      <Icon className="size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{item.title}</span>
                      {item.subtitle && (
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {item.subtitle}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
