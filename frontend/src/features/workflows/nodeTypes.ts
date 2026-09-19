import {
  ArrowRightCircle,
  Bot,
  Braces,
  Clock,
  GitBranch,
  GitFork,
  Globe,
  MessageSquare,
  Play,
  Webhook,
  Wrench,
  type LucideIcon,
} from "lucide-react";

export interface NodeDefinition {
  type: string;
  category: "trigger" | "tool" | "ai" | "logic" | "output";
  label: string;
  description: string;
  icon: LucideIcon;
  color: string; // Tailwind border/badge color
  bgColor: string;
  defaultData: Record<string, unknown>;
}

export const NODE_DEFINITIONS: NodeDefinition[] = [
  // Triggers
  {
    type: "manual",
    category: "trigger",
    label: "Manual Trigger",
    description: "Start workflow manually on click or via test payload",
    icon: Play,
    color: "text-emerald-500",
    bgColor: "bg-emerald-500/10 border-emerald-500/30",
    defaultData: {},
  },
  {
    type: "webhook",
    category: "trigger",
    label: "Webhook Trigger",
    description: "Triggers on incoming HTTP POST/GET request to a unique URL",
    icon: Webhook,
    color: "text-purple-500",
    bgColor: "bg-purple-500/10 border-purple-500/30",
    defaultData: {
      path: "",
      secret: "",
    },
  },
  {
    type: "chat_agent",
    category: "trigger",
    label: "Chat Agent Tool Trigger",
    description: "Exposes this workflow as a callable tool for the chat assistant",
    icon: Bot,
    color: "text-indigo-500",
    bgColor: "bg-indigo-500/10 border-indigo-500/30",
    defaultData: {
      toolName: "my_workflow_tool",
      description: "Performs an automated workflow action",
    },
  },

  // External Tools & APIs
  {
    type: "http_request",
    category: "tool",
    label: "HTTP Request",
    description: "Send any REST API request (GET, POST, PUT, DELETE, PATCH)",
    icon: Globe,
    color: "text-blue-500",
    bgColor: "bg-blue-500/10 border-blue-500/30",
    defaultData: {
      method: "GET",
      url: "https://api.example.com/data",
      headers: {},
      auth: { type: "none" },
      body: "",
    },
  },
  {
    type: "golem_tool",
    category: "tool",
    label: "Herbie Tool",
    description: "Execute a tool saved in your custom tools directory",
    icon: Wrench,
    color: "text-sky-500",
    bgColor: "bg-sky-500/10 border-sky-500/30",
    defaultData: {
      toolName: "",
      args: {},
    },
  },
  {
    type: "github",
    category: "tool",
    label: "GitHub",
    description: "Create issues, comments, or inspect repository data",
    icon: GitBranch,
    color: "text-neutral-400",
    bgColor: "bg-neutral-500/10 border-neutral-500/30",
    defaultData: {
      action: "create_issue",
      owner: "",
      repo: "",
      title: "",
      body: "",
      token: "{{ $credentials.github.token }}",
    },
  },
  {
    type: "slack",
    category: "tool",
    label: "Slack Message",
    description: "Post a message or alert to a Slack channel via webhook",
    icon: MessageSquare,
    color: "text-amber-500",
    bgColor: "bg-amber-500/10 border-amber-500/30",
    defaultData: {
      webhookUrl: "",
      text: "Hello from Herbie Workflow! Result: {{ $json }}",
    },
  },
  {
    type: "discord",
    category: "tool",
    label: "Discord Webhook",
    description: "Send alerts or messages to a Discord channel webhook",
    icon: MessageSquare,
    color: "text-violet-500",
    bgColor: "bg-violet-500/10 border-violet-500/30",
    defaultData: {
      webhookUrl: "",
      content: "Herbie notification: {{ $json }}",
      username: "Herbie Bot",
    },
  },

  // AI & LLM
  {
    type: "llm_prompt",
    category: "ai",
    label: "LLM Prompt",
    description: "Prompt an AI model (Gemini, OpenAI, Anthropic) with inputs",
    icon: Bot,
    color: "text-fuchsia-500",
    bgColor: "bg-fuchsia-500/10 border-fuchsia-500/30",
    defaultData: {
      model: "gemini-2.5-flash",
      prompt: "Summarize the following data into 3 key points:\n{{ $json }}",
      systemPrompt: "You are an expert concise assistant.",
      jsonOutput: false,
    },
  },

  // Logic & Flow Control
  {
    type: "condition",
    category: "logic",
    label: "If / Condition",
    description: "Branch execution path based on expressions (True / False)",
    icon: GitFork,
    color: "text-yellow-500",
    bgColor: "bg-yellow-500/10 border-yellow-500/30",
    defaultData: {
      variable: "{{ $json.status }}",
      operator: "equals",
      value: "success",
    },
  },
  {
    type: "code_transform",
    category: "logic",
    label: "Data Transform",
    description: "Transform and map fields using JSON and expression templates",
    icon: Braces,
    color: "text-teal-500",
    bgColor: "bg-teal-500/10 border-teal-500/30",
    defaultData: {
      fields: {
        summary: "{{ $json.text }}",
        timestamp: "{{ $json.triggeredAt }}",
      },
    },
  },
  {
    type: "delay",
    category: "logic",
    label: "Delay",
    description: "Pause execution for a specified number of seconds",
    icon: Clock,
    color: "text-orange-500",
    bgColor: "bg-orange-500/10 border-orange-500/30",
    defaultData: {
      seconds: 2,
    },
  },

  // Outputs
  {
    type: "webhook_response",
    category: "output",
    label: "Webhook Response",
    description: "Return a custom HTTP status and JSON response to webhook caller",
    icon: ArrowRightCircle,
    color: "text-rose-500",
    bgColor: "bg-rose-500/10 border-rose-500/30",
    defaultData: {
      statusCode: 200,
      headers: { "Content-Type": "application/json" },
      body: {
        success: true,
        data: "{{ $json }}",
      },
    },
  },
];

export function getNodeDefinition(type: string): NodeDefinition {
  const found = NODE_DEFINITIONS.find((n) => n.type === type);
  if (found) return found;
  return {
    type,
    category: "tool",
    label: type,
    description: "Custom node step",
    icon: Wrench,
    color: "text-foreground",
    bgColor: "bg-muted border-border",
    defaultData: {},
  };
}
