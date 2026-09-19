import {
  Activity,
  Bot,
  Coins,
  GitBranch,
  Globe,
  MessageSquare,
  Newspaper,
  Webhook,
  type LucideIcon,
} from "lucide-react";
import type { WorkflowEdge, WorkflowNode } from "@/lib/types";

export interface WorkflowExample {
  id: string;
  title: string;
  category: "AI & Content" | "DevOps & GitHub" | "Alerts & Messaging" | "Utilities & Tools";
  tagline: string;
  description: string;
  icon: LucideIcon;
  badge: string;
  triggerType: string;
  exposeAsTool?: boolean;
  toolName?: string;
  toolDescription?: string;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
}

export const EXAMPLE_CATEGORIES = [
  "All",
  "AI & Content",
  "DevOps & GitHub",
  "Alerts & Messaging",
  "Utilities & Tools",
] as const;

export const EXAMPLE_WORKFLOWS: WorkflowExample[] = [
  {
    id: "github-issue-triage",
    title: "GitHub Issue Triage & AI Summary",
    category: "DevOps & GitHub",
    tagline: "Classify incoming GitHub issues with AI and alert Slack",
    description:
      "Captures new GitHub issue webhooks, runs Gemini to classify if it's a critical bug or feature request, and routes an alert to Slack or Discord.",
    icon: GitBranch,
    badge: "Popular",
    triggerType: "webhook",
    nodes: [
      {
        id: "webhook-trigger",
        type: "webhook",
        name: "GitHub Webhook",
        position: { x: 80, y: 160 },
        data: {
          secret: "",
        },
      },
      {
        id: "extract-issue",
        type: "code_transform",
        name: "Extract Issue Fields",
        position: { x: 320, y: 160 },
        data: {
          fields: {
            title: "{{ $json.body.issue.title }}",
            body: "{{ $json.body.issue.body }}",
            author: "{{ $json.body.issue.user.login }}",
            url: "{{ $json.body.issue.html_url }}",
          },
        },
      },
      {
        id: "ai-classify",
        type: "llm_prompt",
        name: "AI Triage & Classification",
        position: { x: 560, y: 160 },
        data: {
          model: "gemini-2.5-flash",
          prompt:
            "Analyze this GitHub issue and return JSON with keys 'type' (bug/feature/question) and 'summary' (max 2 sentences):\nTitle: {{ $json.title }}\nBody: {{ $json.body }}",
          systemPrompt: "You are a senior triage engineer.",
          jsonOutput: true,
        },
      },
      {
        id: "check-is-bug",
        type: "condition",
        name: "Is Critical Bug?",
        position: { x: 800, y: 160 },
        data: {
          variable: "{{ $json.json.type }}",
          operator: "equals",
          value: "bug",
        },
      },
      {
        id: "slack-urgent",
        type: "slack",
        name: "Slack Urgent Bug Alert",
        position: { x: 1040, y: 80 },
        data: {
          webhookUrl: "{{ $credentials.slack.webhook_url }}",
          text: "🚨 *Critical Bug Reported by {{ $nodes['Extract Issue Fields'].author }}*\n*Issue:* {{ $nodes['Extract Issue Fields'].title }}\n*AI Summary:* {{ $nodes['AI Triage & Classification'].text }}\n*Link:* {{ $nodes['Extract Issue Fields'].url }}",
        },
      },
      {
        id: "discord-notify",
        type: "discord",
        name: "Discord General Channel",
        position: { x: 1040, y: 260 },
        data: {
          webhookUrl: "{{ $credentials.discord.webhook_url }}",
          content: "✨ New Feature/Question from {{ $nodes['Extract Issue Fields'].author }}: {{ $nodes['Extract Issue Fields'].title }}",
        },
      },
    ],
    edges: [
      { id: "e1", source: "webhook-trigger", target: "extract-issue" },
      { id: "e2", source: "extract-issue", target: "ai-classify" },
      { id: "e3", source: "ai-classify", target: "check-is-bug" },
      { id: "e4", source: "check-is-bug", target: "slack-urgent", sourceHandle: "true" },
      { id: "e5", source: "check-is-bug", target: "discord-notify", sourceHandle: "false" },
    ],
  },
  {
    id: "hacker-news-digest",
    title: "Hacker News AI Research Digest",
    category: "AI & Content",
    tagline: "Search tech news via API and synthesize key takeaways",
    description:
      "Queries the Hacker News search API for any keyword, feeds the top discussions into an LLM prompt, and returns a structured markdown brief.",
    icon: Newspaper,
    badge: "Research",
    triggerType: "manual",
    nodes: [
      {
        id: "start-query",
        type: "manual",
        name: "Topic Input",
        position: { x: 80, y: 150 },
        data: {
          query: "Artificial Intelligence",
        },
      },
      {
        id: "search-hn",
        type: "http_request",
        name: "Fetch HN Stories",
        position: { x: 320, y: 150 },
        data: {
          method: "GET",
          url: "https://hn.algolia.com/api/v1/search?tags=story&hitsPerPage=5&query={{ $json.query }}",
        },
      },
      {
        id: "ai-summarize",
        type: "llm_prompt",
        name: "Synthesize Key Trends",
        position: { x: 560, y: 150 },
        data: {
          model: "gemini-2.5-flash",
          prompt:
            "Here are recent top stories about '{{ $nodes['Topic Input'].query }}':\n{{ $json.data.hits }}\n\nWrite a 3-bullet executive brief highlighting the prevailing developer sentiment, controversies, and key insights.",
          systemPrompt: "You are a tech analyst preparing a concise morning briefing.",
        },
      },
      {
        id: "format-brief",
        type: "code_transform",
        name: "Format Output",
        position: { x: 800, y: 150 },
        data: {
          fields: {
            topic: "{{ $nodes['Topic Input'].query }}",
            analysis: "{{ $json.text }}",
            generatedAt: "{{ $json.model }}",
          },
        },
      },
    ],
    edges: [
      { id: "e1", source: "start-query", target: "search-hn" },
      { id: "e2", source: "search-hn", target: "ai-summarize" },
      { id: "e3", source: "ai-summarize", target: "format-brief" },
    ],
  },
  {
    id: "weather-agent-tool",
    title: "Live Weather Assistant Tool",
    category: "Utilities & Tools",
    tagline: "Exposed as a native tool for the chatbot agent to call",
    description:
      "Configured as a callable Herbie Agent Tool. When a user asks about the weather in chat, the agent automatically executes this workflow.",
    icon: Bot,
    badge: "Chatbot Tool",
    triggerType: "chat_agent",
    exposeAsTool: true,
    toolName: "get_city_weather",
    toolDescription: "Gets live current temperature and windspeed for any city coordinates",
    nodes: [
      {
        id: "agent-trigger",
        type: "chat_agent",
        name: "Chat Agent Trigger",
        position: { x: 80, y: 150 },
        data: {
          toolName: "get_city_weather",
          description: "Gets live temperature and wind for coordinates",
        },
      },
      {
        id: "fetch-meteo",
        type: "http_request",
        name: "Open-Meteo Weather API",
        position: { x: 320, y: 150 },
        data: {
          method: "GET",
          url: "https://api.open-meteo.com/v1/forecast?latitude=51.50&longitude=-0.12&current=temperature_2m,wind_speed_10m",
        },
      },
      {
        id: "format-weather",
        type: "code_transform",
        name: "Format Response",
        position: { x: 560, y: 150 },
        data: {
          fields: {
            temperature: "{{ $json.data.current.temperature_2m }} °C",
            windSpeed: "{{ $json.data.current.wind_speed_10m }} km/h",
            time: "{{ $json.data.current.time }}",
          },
        },
      },
    ],
    edges: [
      { id: "e1", source: "agent-trigger", target: "fetch-meteo" },
      { id: "e2", source: "fetch-meteo", target: "format-weather" },
    ],
  },
  {
    id: "bitcoin-rate-alert",
    title: "Crypto Price Threshold Monitor",
    category: "Alerts & Messaging",
    tagline: "Checks CoinGecko rates and alerts when price exceeds limit",
    description:
      "Fetches real-time cryptocurrency exchange rates from CoinGecko, checks if Bitcoin crossed a price target, and posts alerts.",
    icon: Coins,
    badge: "Finance",
    triggerType: "manual",
    nodes: [
      {
        id: "timer-start",
        type: "manual",
        name: "Trigger Check",
        position: { x: 80, y: 160 },
        data: {},
      },
      {
        id: "coingecko-api",
        type: "http_request",
        name: "Fetch Bitcoin USD",
        position: { x: 320, y: 160 },
        data: {
          method: "GET",
          url: "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd",
        },
      },
      {
        id: "check-price",
        type: "condition",
        name: "Price > $50,000?",
        position: { x: 560, y: 160 },
        data: {
          variable: "{{ $json.data.bitcoin.usd }}",
          operator: "greater_than",
          value: 50000,
        },
      },
      {
        id: "slack-bull-alert",
        type: "slack",
        name: "Slack Bull Alert",
        position: { x: 800, y: 80 },
        data: {
          webhookUrl: "{{ $credentials.slack.webhook_url }}",
          text: "🚀 *Bitcoin Bull Alert*: BTC is currently at ${{ $nodes['Fetch Bitcoin USD'].data.bitcoin.usd }}!",
        },
      },
      {
        id: "normal-status",
        type: "code_transform",
        name: "Normal Status Log",
        position: { x: 800, y: 240 },
        data: {
          fields: {
            status: "within_normal_range",
            currentPrice: "{{ $nodes['Fetch Bitcoin USD'].data.bitcoin.usd }}",
          },
        },
      },
    ],
    edges: [
      { id: "e1", source: "timer-start", target: "coingecko-api" },
      { id: "e2", source: "coingecko-api", target: "check-price" },
      { id: "e3", source: "check-price", target: "slack-bull-alert", sourceHandle: "true" },
      { id: "e4", source: "check-price", target: "normal-status", sourceHandle: "false" },
    ],
  },
  {
    id: "api-uptime-monitor",
    title: "API Uptime Monitor & Failover Alert",
    category: "DevOps & GitHub",
    tagline: "Ping healthcheck endpoint with automatic incident alert",
    description:
      "Performs an HTTP GET against a healthcheck URL. If the response code is not 200, sends an emergency alert to Slack and Discord.",
    icon: Activity,
    badge: "DevOps",
    triggerType: "manual",
    nodes: [
      {
        id: "health-start",
        type: "manual",
        name: "Check Health",
        position: { x: 80, y: 160 },
        data: {},
      },
      {
        id: "ping-service",
        type: "http_request",
        name: "Ping Service Healthz",
        position: { x: 320, y: 160 },
        data: {
          method: "GET",
          url: "https://httpbin.org/status/200",
        },
      },
      {
        id: "check-status-200",
        type: "condition",
        name: "Status is 200 OK?",
        position: { x: 560, y: 160 },
        data: {
          variable: "{{ $json.status }}",
          operator: "equals",
          value: 200,
        },
      },
      {
        id: "healthy-response",
        type: "code_transform",
        name: "Healthy Receipt",
        position: { x: 800, y: 80 },
        data: {
          fields: {
            uptime: "operational",
            checkedAt: "{{ $nodes['Check Health'].triggeredAt }}",
          },
        },
      },
      {
        id: "outage-alert",
        type: "slack",
        name: "Post Outage Alert",
        position: { x: 800, y: 240 },
        data: {
          webhookUrl: "{{ $credentials.slack.webhook_url }}",
          text: "⚠️ *CRITICAL OUTAGE DETECTED*: Healthcheck endpoint returned {{ $nodes['Ping Service Healthz'].status }}!",
        },
      },
    ],
    edges: [
      { id: "e1", source: "health-start", target: "ping-service" },
      { id: "e2", source: "ping-service", target: "check-status-200" },
      { id: "e3", source: "check-status-200", target: "healthy-response", sourceHandle: "true" },
      { id: "e4", source: "check-status-200", target: "outage-alert", sourceHandle: "false" },
    ],
  },
  {
    id: "webhook-normalizer-response",
    title: "Webhook Transformer & Custom Response",
    category: "Utilities & Tools",
    tagline: "Transform payload and return custom HTTP status and headers",
    description:
      "Accepts external webhooks, maps fields into standardized format, and returns custom HTTP response code (201 Created) with confirmation body.",
    icon: Webhook,
    badge: "API Bridge",
    triggerType: "webhook",
    nodes: [
      {
        id: "wh-in",
        type: "webhook",
        name: "Incoming Webhook",
        position: { x: 80, y: 150 },
        data: {},
      },
      {
        id: "wh-map",
        type: "code_transform",
        name: "Standardize Customer Payload",
        position: { x: 340, y: 150 },
        data: {
          fields: {
            eventId: "evt_{{ $nodes['Incoming Webhook'].headers['x-request-id'] }}",
            customerEmail: "{{ $json.body.email }}",
            plan: "{{ $json.body.plan }}",
            processed: true,
          },
        },
      },
      {
        id: "wh-out",
        type: "webhook_response",
        name: "HTTP 201 Response",
        position: { x: 600, y: 150 },
        data: {
          statusCode: 201,
          headers: {
            "Content-Type": "application/json",
            "X-Server": "HerbieWorkflow",
          },
          body: {
            status: "accepted",
            record: "{{ $json }}",
          },
        },
      },
    ],
    edges: [
      { id: "e1", source: "wh-in", target: "wh-map" },
      { id: "e2", source: "wh-map", target: "wh-out" },
    ],
  },
  {
    id: "customer-sentiment-router",
    title: "Customer Sentiment & Support Routing",
    category: "AI & Content",
    tagline: "Analyze feedback tone with LLM and escalate angry tickets",
    description:
      "Scores customer feedback messages from 1 to 10 for frustration. If frustration is high, automatically flags the incident on Slack for immediate manager attention.",
    icon: MessageSquare,
    badge: "Customer Success",
    triggerType: "webhook",
    nodes: [
      {
        id: "ticket-webhook",
        type: "webhook",
        name: "Support Ticket Webhook",
        position: { x: 80, y: 160 },
        data: {},
      },
      {
        id: "sentiment-prompt",
        type: "llm_prompt",
        name: "Analyze Customer Emotion",
        position: { x: 320, y: 160 },
        data: {
          model: "gemini-2.5-flash",
          prompt:
            "Analyze customer message:\n\"{{ $json.body.message }}\"\n\nReturn a JSON object with:\n- \"frustration_score\": integer from 1 to 10\n- \"summary\": concise 1-line reason",
          jsonOutput: true,
        },
      },
      {
        id: "is-escalated",
        type: "condition",
        name: "Frustration Score >= 7?",
        position: { x: 560, y: 160 },
        data: {
          variable: "{{ $json.json.frustration_score }}",
          operator: "greater_than",
          value: 6,
        },
      },
      {
        id: "escalate-slack",
        type: "slack",
        name: "Escalate to Support Lead",
        position: { x: 800, y: 80 },
        data: {
          webhookUrl: "{{ $credentials.slack.webhook_url }}",
          text: "🔥 *Angry Customer Alert* (Score: {{ $nodes['Analyze Customer Emotion'].json.frustration_score }}/10)\nReason: {{ $nodes['Analyze Customer Emotion'].json.summary }}\nCustomer: {{ $nodes['Support Ticket Webhook'].body.email }}",
        },
      },
      {
        id: "normal-ticket",
        type: "webhook_response",
        name: "Standard Response",
        position: { x: 800, y: 240 },
        data: {
          statusCode: 200,
          body: {
            queued: true,
            priority: "normal",
          },
        },
      },
    ],
    edges: [
      { id: "e1", source: "ticket-webhook", target: "sentiment-prompt" },
      { id: "e2", source: "sentiment-prompt", target: "is-escalated" },
      { id: "e3", source: "is-escalated", target: "escalate-slack", sourceHandle: "true" },
      { id: "e4", source: "is-escalated", target: "normal-ticket", sourceHandle: "false" },
    ],
  },
  {
    id: "discord-github-releases",
    title: "GitHub Release Community Announcer",
    category: "Alerts & Messaging",
    tagline: "Post release changelogs to Discord community channels",
    description:
      "When a new release is published on GitHub, formats the changelog notes and posts an announcement to Discord.",
    icon: Globe,
    badge: "Community",
    triggerType: "webhook",
    nodes: [
      {
        id: "release-hook",
        type: "webhook",
        name: "Release Published Webhook",
        position: { x: 80, y: 150 },
        data: {},
      },
      {
        id: "extract-release",
        type: "code_transform",
        name: "Format Release Note",
        position: { x: 340, y: 150 },
        data: {
          fields: {
            title: "🚀 Herbie v{{ $json.body.release.tag_name }} is out!",
            notes: "{{ $json.body.release.body }}",
            url: "{{ $json.body.release.html_url }}",
          },
        },
      },
      {
        id: "post-discord",
        type: "discord",
        name: "Post to Discord #announcements",
        position: { x: 600, y: 150 },
        data: {
          webhookUrl: "{{ $credentials.discord.webhook_url }}",
          username: "Release Bot",
          content: "🎉 **{{ $json.title }}**\n\n{{ $json.notes }}\n\nView on GitHub: {{ $json.url }}",
        },
      },
    ],
    edges: [
      { id: "e1", source: "release-hook", target: "extract-release" },
      { id: "e2", source: "extract-release", target: "post-discord" },
    ],
  },
];
