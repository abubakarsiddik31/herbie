import { useState } from "react";
import { Link } from "react-router";
import {
  ArrowRight,
  Bot,
  Check,
  ChevronRight,
  Code2,
  Copy,
  Cpu,
  Database,
  ExternalLink,
  Heart,
  Layers,
  Lock,
  MessageSquare,
  Play,
  ShieldCheck,
  Terminal,
  Workflow,
  Wrench,
  Zap,
} from "lucide-react";
import { AnimatedHerbieLogo, type HerbieMood } from "@/components/AnimatedHerbieLogo";
import { ThemeToggle } from "@/components/ThemeToggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useAuth } from "@/stores/auth";

export function LandingPage() {
  const token = useAuth((s) => s.accessToken);

  // Hero interactive mascot mood
  const [heroMood, setHeroMood] = useState<HerbieMood>("float");
  const [copiedQuickstart, setCopiedQuickstart] = useState(false);

  // Playground simulation state
  type PlaygroundScenario = "sandbox" | "mcp" | "workflow" | "rag" | "cost";
  const [playgroundTab, setPlaygroundTab] = useState<PlaygroundScenario>("sandbox");
  const [simStreaming, setSimStreaming] = useState(false);
  const [simText, setSimText] = useState(
    "Executed Python in isolated Code Sandbox: Total Future Value = $86,419.23 (Principal: $57,000.00, Compound Interest: $29,419.23). Evaluated in 12ms with zero host network exposure."
  );

  // Herbie Story soundboard state
  const [storyQuote, setStoryQuote] = useState<string>(
    "“Hey! I'm Herbie — inspired by Fantastic Four's iconic Baxter Building robot ally!”"
  );
  const [storyMood, setStoryMood] = useState<HerbieMood>("waving");

  // Quickstart terminal copy
  const handleCopyCommand = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedQuickstart(true);
    setTimeout(() => setCopiedQuickstart(false), 2000);
  };

  // Playground simulation driver
  const runSimulation = (tab: PlaygroundScenario) => {
    setPlaygroundTab(tab);
    setSimStreaming(true);
    setSimText("");

    const scripts: Record<PlaygroundScenario, string> = {
      sandbox:
        "Executed Python in isolated Code Sandbox: Total Future Value = $86,419.23 (Principal: $57,000.00, Compound Interest: $29,419.23). Evaluated in 12ms with zero host network exposure.",
      mcp:
        "Coordinated multi-hop MCP apps via standard JSON-RPC 2.0: Checked Google Calendar (2 events today), inspected GitHub PR #42 ('feat: add streaming rag'), and posted an approval ping to #general on Slack.",
      workflow:
        "Pipeline #wf-release triggered! Executed Webhook Ingress -> Summarized git commit diff with Claude 4.5 Sonnet -> Validated code in Sandbox -> Dispatched Slack release notice in 280ms.",
      rag:
        "Retrieved 3 document chunks from /docs/specs/rag-pipeline.md (Cosine Similarity: 0.94). Herbie organizes project documents with hierarchical chunking, storing relative parsed text alongside vector embeddings in SQLite.",
      cost:
        "Calculated session ledger: 420 prompt tokens + 168 completion tokens on Claude 4.5 Sonnet. Total execution cost: $0.000142. Zero markup applied.",
    };

    const target = scripts[tab];
    let index = 0;
    const interval = setInterval(() => {
      index += 4;
      if (index >= target.length) {
        setSimText(target);
        setSimStreaming(false);
        clearInterval(interval);
      } else {
        setSimText(target.slice(0, index));
      }
    }, 18);
  };

  return (
    <div className="relative min-h-screen bg-background text-foreground selection:bg-brand/20 selection:text-brand font-sans antialiased overflow-x-hidden">
      {/* ====================================================================
          STICKY APP HEADER / NAVBAR
          ==================================================================== */}
      <header className="sticky top-0 z-50 w-full border-b border-border/40 bg-background/80 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          {/* Brand Lockup */}
          <Link to="/" className="flex items-center gap-3 group outline-none">
            <div className="size-8 rounded-xl bg-brand/10 dark:bg-brand/20 p-1 flex items-center justify-center ring-1 ring-brand/30 group-hover:scale-105 transition-transform">
              <AnimatedHerbieLogo mood="float" size="sm" interactive={false} />
            </div>
            <div className="flex flex-col">
              <div className="flex items-center gap-1.5">
                <span className="font-bold tracking-tight text-lg leading-none">Herbie</span>
                <Badge variant="outline" className="text-[10px] px-1.5 py-0 h-4 border-brand-border text-brand font-mono">
                  v0.1.0
                </Badge>
              </div>
              <span className="text-[11px] text-muted-foreground leading-none mt-0.5">The Free AI Workspace</span>
            </div>
          </Link>

          {/* Center Navigation Links (Desktop) */}
          <nav className="hidden md:flex items-center gap-6 text-xs font-medium text-muted-foreground">
            <a href="#features" className="hover:text-foreground transition-colors">
              Features
            </a>
            <a href="#why-herbie" className="hover:text-foreground transition-colors flex items-center gap-1">
              Why Herbie?
              <span className="size-1.5 rounded-full bg-brand animate-pulse" />
            </a>
            <a href="#playground" className="hover:text-foreground transition-colors">
              Interactive Demo
            </a>
            <a href="#workflows" className="hover:text-foreground transition-colors">
              Workflows
            </a>
            <a href="#architecture" className="hover:text-foreground transition-colors">
              Architecture
            </a>
            <a href="#quickstart" className="hover:text-foreground transition-colors">
              Quickstart
            </a>
          </nav>

          {/* Right Action Controls */}
          <div className="flex items-center gap-2.5">
            {/* GitHub Repo Button */}
            <a
              href="https://github.com/abubakarsiddik31/herbie"
              target="_blank"
              rel="noreferrer"
              className="hidden sm:inline-flex items-center gap-1.5 rounded-lg border border-border/80 bg-background/50 px-2.5 py-1.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
            >
              <svg className="size-3.5 fill-current" viewBox="0 0 24 24">
                <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
              </svg>
              <span>Star</span>
            </a>

            {/* Light / Dark Mode Toggle */}
            <ThemeToggle />

            {/* Main Auth / App Button */}
            {token ? (
              <Button asChild variant="brand" size="sm" className="gap-1.5 shadow-xs">
                <Link to="/chat">
                  <span>Open App</span>
                  <ArrowRight className="size-3.5" />
                </Link>
              </Button>
            ) : (
              <div className="flex items-center gap-1.5">
                <Button asChild variant="ghost" size="sm">
                  <Link to="/login">Sign in</Link>
                </Button>
                <Button asChild variant="brand" size="sm" className="gap-1.5 shadow-xs">
                  <Link to="/chat">
                    <span>Launch</span>
                    <ArrowRight className="size-3.5" />
                  </Link>
                </Button>
              </div>
            )}
          </div>
        </div>
      </header>

      {/* ====================================================================
          HERO SECTION: THE AI WORKSPACE WITH A SOUL
          ==================================================================== */}
      <section className="relative pt-12 pb-20 md:pt-20 md:pb-28 overflow-hidden">
        {/* Background Ambient Aura */}
        <div className="pointer-events-none absolute -top-40 left-1/2 -translate-x-1/2 size-[650px] rounded-full bg-brand/10 dark:bg-brand/15 blur-[120px]" />
        <div className="pointer-events-none absolute top-1/2 -left-40 size-[450px] rounded-full bg-emerald-500/5 blur-[100px]" />

        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 relative z-10">
          <div className="flex flex-col items-center text-center space-y-6 max-w-4xl mx-auto">
            {/* Announcement Pill */}
            <div className="inline-flex items-center gap-2 rounded-full border border-brand-border/80 bg-brand-muted/40 dark:bg-brand/10 px-3.5 py-1 text-xs font-medium text-brand">
              <span className="flex size-2 rounded-full bg-brand animate-ping" />
              <span>Free & Open Source AI Workspace</span>
              <span className="text-muted-foreground/60">·</span>
              <span className="text-foreground">Go 1.24 + React 19</span>
            </div>

            {/* Massive Distinctive Headline */}
            <h1 className="text-4xl sm:text-6xl md:text-7xl font-extrabold tracking-tight text-foreground leading-[1.08]">
              The AI Workspace <br className="hidden sm:inline" />
              <span className="text-brand">With a Soul.</span>
            </h1>

            {/* Grounded Pitch Copy */}
            <p className="max-w-2xl text-base sm:text-lg text-muted-foreground leading-relaxed">
              Tired of $20/month walled gardens? <strong className="text-foreground font-semibold">Herbie</strong> gives you a sovereign, local-first workspace for multi-model chat, visual agent workflows, grounded project RAG, and MCP tools—running 100% on your infrastructure with zero token markups.
            </p>

            {/* Interactive Hero Mascot Stage */}
            <div className="w-full pt-4 pb-2 flex flex-col items-center">
              <div className="relative p-6 sm:p-8 rounded-3xl bg-radial from-brand-muted/20 via-background to-transparent border border-border/40 shadow-xl dark:shadow-2xl">
                {/* Floating Benefit Badges */}
                <div className="hidden lg:flex absolute -left-20 top-12 items-center gap-2 rounded-xl bg-card/90 border border-border px-3 py-1.5 text-xs font-medium shadow-md backdrop-blur-sm animate-in fade-in slide-in-from-left duration-700">
                  <Zap className="size-4 text-brand" />
                  <span>Go Backend: &lt;1ms SSE Latency</span>
                </div>

                <div className="hidden lg:flex absolute -right-20 top-24 items-center gap-2 rounded-xl bg-card/90 border border-border px-3 py-1.5 text-xs font-medium shadow-md backdrop-blur-sm animate-in fade-in slide-in-from-right duration-700">
                  <ShieldCheck className="size-4 text-emerald-500" />
                  <span>100% Local: Zero Telemetry Leaked</span>
                </div>

                <div className="hidden lg:flex absolute -left-16 bottom-10 items-center gap-2 rounded-xl bg-card/90 border border-border px-3 py-1.5 text-xs font-medium shadow-md backdrop-blur-sm animate-in fade-in slide-in-from-left duration-700">
                  <Workflow className="size-4 text-sky-500" />
                  <span>Autonomous Visual Pipelines</span>
                </div>

                {/* Animated Herbie Mascot */}
                <AnimatedHerbieLogo
                  mood={heroMood}
                  size="hero"
                  interactive={true}
                  showWordmark={true}
                />

                {/* Interactive Mood Selector Bar */}
                <div className="mt-4 flex flex-wrap items-center justify-center gap-1.5 pt-3 border-t border-border/40">
                  <span className="text-[11px] text-muted-foreground mr-1">Try Mood:</span>
                  {(
                    [
                      { id: "float", label: "Hover", icon: "🟢" },
                      { id: "jumping", label: "Jump", icon: "🦘" },
                      { id: "waving", label: "Wave", icon: "👋" },
                      { id: "thinking", label: "Radar", icon: "📡" },
                      { id: "coding", label: "Code", icon: "💻" },
                      { id: "celebrate", label: "Victory", icon: "🎯" },
                    ] as const
                  ).map((m) => (
                    <button
                      key={m.id}
                      onClick={() => setHeroMood(m.id)}
                      className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium transition-all ${
                        heroMood === m.id
                          ? "bg-brand text-brand-foreground shadow-xs scale-105"
                          : "bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground"
                      }`}
                    >
                      <span>{m.icon}</span>
                      <span>{m.label}</span>
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Primary Action Button Row */}
            <div className="flex flex-col sm:flex-row items-center gap-3 pt-2">
              <Button asChild variant="brand" size="lg" className="gap-2 px-6 h-12 text-sm shadow-md font-medium">
                <Link to="/chat">
                  <span>Start Chatting Free</span>
                  <ArrowRight className="size-4" />
                </Link>
              </Button>

              <Button
                asChild
                variant="outline"
                size="lg"
                className="h-12 px-5 text-sm gap-2 border-border/80 hover:border-brand/40"
              >
                <a href="#playground">
                  <Play className="size-3.5 text-brand fill-brand" />
                  <span>Test Drive Playground</span>
                </a>
              </Button>

              <Button
                asChild
                variant="ghost"
                size="lg"
                className="h-12 px-4 text-sm gap-2 text-muted-foreground hover:text-foreground"
              >
                <a href="#why-herbie">
                  <span>Why Herbie? (Fantastic 4)</span>
                  <ChevronRight className="size-4" />
                </a>
              </Button>
            </div>

            {/* One-Line Quickstart Terminal Card */}
            <div className="w-full max-w-xl pt-4">
              <div className="rounded-2xl border border-border/80 bg-card/80 p-3.5 shadow-sm text-left backdrop-blur-sm">
                <div className="flex items-center justify-between pb-2 border-b border-border/40 text-[11px] text-muted-foreground font-mono">
                  <div className="flex items-center gap-1.5">
                    <span className="size-2.5 rounded-full bg-destructive/60" />
                    <span className="size-2.5 rounded-full bg-amber-500/60" />
                    <span className="size-2.5 rounded-full bg-emerald-500/60" />
                    <span className="ml-1 text-foreground font-semibold">Quickstart (Docker)</span>
                  </div>
                  <span className="text-[10px] text-brand">Ready in 30 seconds</span>
                </div>

                <div className="flex items-center justify-between pt-2.5 font-mono text-xs text-foreground">
                  <div className="overflow-x-auto whitespace-nowrap pr-2">
                    <span className="text-brand select-none">$ </span>
                    <span>git clone https://github.com/abubakarsiddik31/herbie.git && cd herbie && docker compose up -d</span>
                  </div>
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    onClick={() =>
                      handleCopyCommand(
                        "git clone https://github.com/abubakarsiddik31/herbie.git && cd herbie && docker compose up -d"
                      )
                    }
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    title="Copy to clipboard"
                  >
                    {copiedQuickstart ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ====================================================================
          STORY SECTION: "WHY HERBIE? (INSPIRED BY FANTASTIC FOUR)"
          ==================================================================== */}
      <section id="why-herbie" className="py-20 md:py-28 border-y border-border/40 bg-muted/20 relative">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          {/* Section Header */}
          <div className="max-w-3xl mx-auto text-center space-y-3 mb-14">
            <Badge variant="brand" className="px-3 py-1 text-xs">
              Mascot & Fantastic Four Heritage
            </Badge>
            <h2 className="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-foreground">
              Meet Herbie. <br className="hidden sm:inline" />
              <span className="text-brand">Inspired by Fantastic Four’s Iconic Robot Ally.</span>
            </h2>
            <p className="text-sm sm:text-base text-muted-foreground leading-relaxed">
              Why does an autonomous AI workspace carry the name Herbie? Because every great team deserves a loyal, hyper-capable laboratory companion.
            </p>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-center">
            {/* Story Editorial Details (Left 7 Cols) */}
            <div className="lg:col-span-7 space-y-6">
              {/* Card 1: Fantastic Four & H.E.R.B.I.E. Origin */}
              <div className="p-5 sm:p-6 rounded-2xl border border-border/80 bg-card shadow-xs space-y-2">
                <div className="flex items-center gap-2 text-brand font-semibold text-sm">
                  <Bot className="size-4" />
                  <span>1. The Fantastic Four Inspiration (H.E.R.B.I.E.)</span>
                </div>
                <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                  In Marvel’s <strong className="text-foreground">Fantastic Four</strong>, Mister Fantastic (Reed Richards) created <strong className="text-foreground">H.E.R.B.I.E.</strong> (<em>Humanoid Experimental Robot, B-type, Integrated Electronics</em>) inside the Baxter Building to help the team analyze deep scientific calculations, automate lab systems, and provide loyal support in the face of impossible challenges.
                </p>
                <p className="text-xs text-muted-foreground/80 leading-relaxed pt-1">
                  We engineered Herbie with that exact same spirit: your friendly, indefatigable desktop companion capable of orchestrating multi-model reasoning, visual workflows, and complex document intelligence.
                </p>
              </div>

              {/* Card 2: The Speech-Bubble Head */}
              <div className="p-5 sm:p-6 rounded-2xl border border-border/80 bg-card shadow-xs space-y-2">
                <div className="flex items-center gap-2 text-brand font-semibold text-sm">
                  <MessageSquare className="size-4" />
                  <span>2. The Bubble-Bot Silhouette</span>
                </div>
                <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                  Herbie’s head isn’t a cold chrome cube or a featureless corporate sphere. It’s an <strong className="text-foreground">open speech bubble with a conversational tail</strong>. It encapsulates software that listens before it speaks, designed for real human collaboration rather than opaque black-box pronouncements.
                </p>
              </div>

              {/* Card 3: The Moniker & Nicknames */}
              <div className="p-5 sm:p-6 rounded-2xl border border-border/80 bg-card shadow-xs space-y-2">
                <div className="flex items-center gap-2 text-brand font-semibold text-sm">
                  <Heart className="size-4" />
                  <span>3. The Name (And the Occasional “Herbey” / “Harbey” Mix-Up!)</span>
                </div>
                <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                  Sometimes users casually misspell or pronounce it <em className="text-foreground font-medium">“Herbey”</em> or affectionately nickname him <em className="text-foreground font-medium">“Harbey”</em>, or notice <code className="font-mono text-foreground text-[11px] bg-muted px-1.5 py-0.5 rounded">HarveyAvatar.tsx</code> in our UI components. But he is 100% <strong className="text-foreground">Herbie</strong>—always ready to jump into action when your pipelines fire!
                </p>
              </div>

              {/* Card 4: Anti-Corporate AI */}
              <div className="p-5 sm:p-6 rounded-2xl border border-border/80 bg-card shadow-xs space-y-2">
                <div className="flex items-center gap-2 text-brand font-semibold text-sm">
                  <ShieldCheck className="size-4" />
                  <span>4. The Antidote to Corporate AI Monoliths</span>
                </div>
                <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                  Commercial AI giants name their tools like clinical Latin deities or high-frequency trading firms. Herbie is your unpretentious desktop ally: runs locally on your machine, never trains on your confidential code, never charges a 500% middleman token markup, and jumps with genuine joy when your workflow succeeds.
                </p>
              </div>
            </div>

            {/* Interactive Herbie Interactive Card (Right 5 Cols) */}
            <div className="lg:col-span-5">
              <div className="rounded-3xl border border-brand-border/60 bg-gradient-to-b from-card via-card to-brand-muted/10 p-6 sm:p-8 shadow-lg flex flex-col items-center text-center relative overflow-hidden">
                <div className="absolute top-3 right-3">
                  <Badge variant="brand" className="text-[10px]">
                    Interactive Mascot
                  </Badge>
                </div>

                <div className="py-2">
                  <AnimatedHerbieLogo
                    mood={storyMood}
                    size="lg"
                    interactive={true}
                    bubbleText={storyQuote}
                    onClick={() => {
                      const quotes = [
                        "“H.E.R.B.I.E. online! Ready to assist your Baxter Building experiments!”",
                        "“Inspired by Fantastic Four — engineered with high-concurrency Go!”",
                        "“Zero telemetry, zero vendor lock-in. Just pure open source speed!”",
                        "“Call me Herbie (even if you accidentally type Herbey)!”",
                        "“Bring your own API keys or run Ollama locally for $0.00!”",
                      ];
                      const nextQuote = quotes[Math.floor(Math.random() * quotes.length)];
                      setStoryQuote(nextQuote);
                      setStoryMood("celebrate");
                      setTimeout(() => setStoryMood("waving"), 1200);
                    }}
                  />
                </div>

                <div className="mt-5 space-y-2 max-w-sm">
                  <h3 className="text-lg font-bold text-foreground">Click Herbie to Chat!</h3>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    Click the mascot to cycle through Herbie's thoughts on open source, his Fantastic Four roots, token transparency, and sovereign AI.
                  </p>
                </div>

                {/* Herbie Anatomy Checklist */}
                <div className="w-full mt-6 pt-5 border-t border-border/60 grid grid-cols-2 gap-2.5 text-left text-xs">
                  <div className="p-2.5 rounded-xl bg-muted/40 border border-border/40 space-y-0.5">
                    <span className="font-semibold text-foreground text-[11px] block">📡 Sensor Antenna</span>
                    <span className="text-[10px] text-muted-foreground">Pings live MCP & web tools</span>
                  </div>
                  <div className="p-2.5 rounded-xl bg-muted/40 border border-border/40 space-y-0.5">
                    <span className="font-semibold text-foreground text-[11px] block">💬 Bubble Visor</span>
                    <span className="text-[10px] text-muted-foreground">Speaks in streaming markdown</span>
                  </div>
                  <div className="p-2.5 rounded-xl bg-muted/40 border border-border/40 space-y-0.5">
                    <span className="font-semibold text-foreground text-[11px] block">🦘 Jump Thrusters</span>
                    <span className="text-[10px] text-muted-foreground">Celebrates finished tasks</span>
                  </div>
                  <div className="p-2.5 rounded-xl bg-muted/40 border border-border/40 space-y-0.5">
                    <span className="font-semibold text-foreground text-[11px] block">🔒 Private Vault</span>
                    <span className="text-[10px] text-muted-foreground">Local SQLite storage</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ====================================================================
          PLAYGROUND SECTION: INTERACTIVE TEST DRIVE SIMULATION
          ==================================================================== */}
      <section id="playground" className="py-20 md:py-28 relative">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="max-w-3xl mx-auto text-center space-y-3 mb-12">
            <Badge variant="outline" className="text-brand border-brand-border px-3 py-1 text-xs">
              Live Simulator
            </Badge>
            <h2 className="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-foreground">
              See Herbie in Action
            </h2>
            <p className="text-sm sm:text-base text-muted-foreground leading-relaxed">
              Test drive how Herbie coordinates multi-hop MCP tools, visual pipelines, vector RAG lookups, and token ledgers in real time.
            </p>
          </div>

          {/* Interactive Chat & Tool Sandbox Container */}
          <div className="max-w-4xl mx-auto rounded-3xl border border-border bg-card shadow-lg overflow-hidden">
            {/* Simulation Scenario Tabs */}
            <div className="flex flex-wrap items-center border-b border-border bg-muted/30 p-2 gap-1.5">
              {(
                [
                  { id: "sandbox", label: "Code Sandbox (Python / AST)", icon: Terminal },
                  { id: "mcp", label: "MCP Ecosystem (GitHub/Slack/Calendar)", icon: Layers },
                  { id: "workflow", label: "Autonomous Agent Pipeline", icon: Workflow },
                  { id: "rag", label: "Project Document RAG", icon: Database },
                  { id: "cost", label: "Token & Cost Ledger", icon: Zap },
                ] as const
              ).map((t) => {
                const Icon = t.icon;
                return (
                  <button
                    key={t.id}
                    onClick={() => runSimulation(t.id)}
                    className={`flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-medium transition-all ${
                      playgroundTab === t.id
                        ? "bg-brand text-brand-foreground shadow-xs"
                        : "text-muted-foreground hover:text-foreground hover:bg-muted"
                    }`}
                  >
                    <Icon className="size-3.5" />
                    <span>{t.label}</span>
                  </button>
                );
              })}
            </div>

            {/* Simulated Chat Interface */}
            <div className="p-6 sm:p-8 space-y-6">
              {/* User Prompt Message */}
              <div className="flex items-start gap-3.5 justify-end">
                <div className="max-w-md rounded-2xl bg-brand text-brand-foreground px-4 py-2.5 text-xs sm:text-sm shadow-xs">
                  {playgroundTab === "sandbox" && "Calculate the compound growth of $25,000 at 9.2% over 5 years with quarterly compounding in Python, and verify with math AST."}
                  {playgroundTab === "mcp" && "Search GitHub PRs for 'streaming-rag', inspect pending approvals, and notify the team on Slack."}
                  {playgroundTab === "workflow" && "Trigger the automated code review and release pipeline for v0.1.0!"}
                  {playgroundTab === "rag" && "How does Herbie structure relative parsed text storage in RAG?"}
                  {playgroundTab === "cost" && "What did my last Claude 4.5 Sonnet reasoning turn cost?"}
                </div>
                <div className="size-8 rounded-full bg-secondary flex items-center justify-center text-xs font-bold shrink-0">
                  You
                </div>
              </div>

              {/* Tool Execution Chip (Simulated) */}
              <div className="flex items-center gap-2 text-xs text-muted-foreground pl-11">
                <div className="inline-flex items-center gap-1.5 rounded-lg border border-border/80 bg-muted/50 px-2.5 py-1 font-mono text-[11px]">
                  <Wrench className="size-3 text-brand" />
                  <span>
                    {playgroundTab === "sandbox" && "Calling: code_sandbox.execute(lang='python', timeout=3000)"}
                    {playgroundTab === "mcp" && "Coordinating: github.search_prs() -> mcp.calendar_events() -> slack.send_notification()"}
                    {playgroundTab === "workflow" && "Firing: workflow.execute(pipeline_id='wf-ci-release')"}
                    {playgroundTab === "rag" && "Executing: rag.vector_search(query='relative storage')"}
                    {playgroundTab === "cost" && "Calculating: telemetry.audit_session_cost()"}
                  </span>
                </div>
                <span className="text-[10px] text-emerald-500 font-semibold flex items-center gap-1">
                  <Check className="size-3" />
                  200 OK (Go Engine: 18ms)
                </span>
              </div>

              {/* Herbie Assistant Response Message */}
              <div className="flex items-start gap-3.5">
                <div className="size-8 rounded-xl bg-brand/10 dark:bg-brand/20 p-0.5 flex items-center justify-center shrink-0 ring-1 ring-brand/30">
                  <AnimatedHerbieLogo mood={simStreaming ? "jumping" : "float"} size="sm" interactive={false} />
                </div>
                <div className="flex-1 space-y-2">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-xs text-foreground">Herbie</span>
                    <Badge variant="outline" className="text-[10px] px-1.5 py-0 h-4 border-brand-border text-brand font-mono">
                      Claude 4.5 Sonnet · Go SSE
                    </Badge>
                  </div>

                  <div className="rounded-2xl border border-border/80 bg-card p-4 text-xs sm:text-sm text-foreground leading-relaxed shadow-xs min-h-16">
                    <p>{simText}</p>
                    {simStreaming && (
                      <span className="inline-block size-2 rounded-full bg-brand animate-pulse ml-1 align-baseline" />
                    )}
                  </div>
                </div>
              </div>
            </div>

            {/* Bottom Bar: Action Trigger */}
            <div className="border-t border-border bg-muted/20 px-6 py-3 flex items-center justify-between text-xs text-muted-foreground">
              <span>Simulated live response using Herbie's Go agent runtime.</span>
              <Button asChild variant="brand" size="xs" className="gap-1">
                <Link to="/chat">
                  <span>Open Full Workspace</span>
                  <ArrowRight className="size-3" />
                </Link>
              </Button>
            </div>
          </div>
        </div>
      </section>

      {/* ====================================================================
          CORE FEATURES BENTO GRID
          ==================================================================== */}
      <section id="features" className="py-20 md:py-28 border-t border-border/40 bg-muted/10 relative">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="max-w-3xl mx-auto text-center space-y-3 mb-16">
            <Badge variant="brand" className="px-3 py-1 text-xs">
              Capabilities
            </Badge>
            <h2 className="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-foreground">
              Everything You Need in One Sovereign App
            </h2>
            <p className="text-sm sm:text-base text-muted-foreground leading-relaxed">
              Engineered with Nordic precision. No fluff, no dark patterns, no walled gardens.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {/* Card 1: Multi-Model Intelligence */}
            <Card className="p-6 sm:p-7 space-y-4 border-border/80 hover:border-brand/40 transition-colors bg-card/60 backdrop-blur-sm">
              <div className="size-10 rounded-xl bg-brand/10 dark:bg-brand/20 flex items-center justify-center text-brand">
                <Cpu className="size-5" />
              </div>
              <h3 className="text-lg font-bold text-foreground">Multi-Model Polyglot</h3>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                Connect Claude 4.5 Sonnet, GPT 5.6, Gemini 3.5 Flash, DeepSeek R1, or offline Ollama instances. You supply your keys, you pay the labs directly at raw API cost with zero hidden markups.
              </p>
              <div className="pt-2 flex flex-wrap gap-1.5 font-mono text-[10px]">
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Claude 4.5 Sonnet</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">GPT 5.6</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Gemini 3.5 Flash</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Ollama & DeepSeek</span>
              </div>
            </Card>

            {/* Card 2: Visual Workflow Engine */}
            <Card className="p-6 sm:p-7 space-y-4 border-border/80 hover:border-brand/40 transition-colors bg-card/60 backdrop-blur-sm">
              <div className="size-10 rounded-xl bg-sky-500/10 flex items-center justify-center text-sky-500">
                <Workflow className="size-5" />
              </div>
              <h3 className="text-lg font-bold text-foreground">Autonomous Visual Pipelines</h3>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                Build node-based agent pipelines with visual graph execution. Ingest webhooks, route logic conditionally, run multi-model AI transforms, and post alerts to Slack or GitHub without writing glue code.
              </p>
              <div className="pt-2 flex flex-wrap gap-1.5 font-mono text-[10px]">
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Visual Graph Canvas</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Webhook Ingress</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">AI Nodes</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Conditionals</span>
              </div>
            </Card>

            {/* Card 3: Grounded Project RAG */}
            <Card className="p-6 sm:p-7 space-y-4 border-border/80 hover:border-brand/40 transition-colors bg-card/60 backdrop-blur-sm">
              <div className="size-10 rounded-xl bg-emerald-500/10 flex items-center justify-center text-emerald-500">
                <Database className="size-5" />
              </div>
              <h3 className="text-lg font-bold text-foreground">Grounded Project RAG</h3>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                Organize documents into dedicated project workspaces. Herbie chunks, embeds, and parses PDFs, markdown, and code repos—searching your local files first before checking the web.
              </p>
              <div className="pt-2 flex flex-wrap gap-1.5 font-mono text-[10px]">
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Hierarchical Chunking</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Parsed Viewer</span>
              </div>
            </Card>

            {/* Card 4: Model Context Protocol (MCP) & Sandbox */}
            <Card className="p-6 sm:p-7 space-y-4 border-border/80 hover:border-brand/40 transition-colors bg-card/60 backdrop-blur-sm">
              <div className="size-10 rounded-xl bg-violet-500/10 flex items-center justify-center text-violet-500">
                <Layers className="size-5" />
              </div>
              <h3 className="text-lg font-bold text-foreground">MCP Ecosystem & Code Sandbox</h3>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                Herbie is both an MCP client and an MCP server. Run safe Python, JavaScript, and math in an isolated Code Sandbox, read Google Calendars, manage GitHub pull requests, crawl webpages with clean markdown extraction, or connect custom JSON-RPC external MCP servers.
              </p>
              <div className="pt-2 flex flex-wrap gap-1.5 font-mono text-[10px]">
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Code Sandbox (Python/JS)</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">GitHub & Slack</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Google Calendar</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">JSON-RPC 2.0</span>
              </div>
            </Card>

            {/* Card 5: Exact Cost Ledger */}
            <Card className="p-6 sm:p-7 space-y-4 border-border/80 hover:border-brand/40 transition-colors bg-card/60 backdrop-blur-sm">
              <div className="size-10 rounded-xl bg-amber-500/10 flex items-center justify-center text-amber-500">
                <Zap className="size-5" />
              </div>
              <h3 className="text-lg font-bold text-foreground">Transparent Cost Ledger</h3>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                Track every token and millicent. Each message displays input/output token counts, dollar costs, and execution latency. View aggregated usage charts and audit logs in your usage dashboard.
              </p>
              <div className="pt-2 flex flex-wrap gap-1.5 font-mono text-[10px]">
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Millicent Precision</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Live Telemetry</span>
              </div>
            </Card>

            {/* Card 6: 100% Self-Hosted & Sovereign */}
            <Card className="p-6 sm:p-7 space-y-4 border-border/80 hover:border-brand/40 transition-colors bg-card/60 backdrop-blur-sm">
              <div className="size-10 rounded-xl bg-rose-500/10 flex items-center justify-center text-rose-500">
                <Lock className="size-5" />
              </div>
              <h3 className="text-lg font-bold text-foreground">Local SQLite & Zero Telemetry</h3>
              <p className="text-xs sm:text-sm text-muted-foreground leading-relaxed">
                Herbie stores everything in an embedded SQLite database with WAL mode on your disk. No remote telemetry pings, no cloud surveillance, and no surprise account closures.
              </p>
              <div className="pt-2 flex flex-wrap gap-1.5 font-mono text-[10px]">
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">SQLite WAL</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Docker Ready</span>
                <span className="px-2 py-0.5 rounded bg-muted text-muted-foreground">Single Binary</span>
              </div>
            </Card>
          </div>
        </div>
      </section>

      {/* ====================================================================
          FEATURE COMPARISON TABLE
          ==================================================================== */}
      <section className="py-20 md:py-28 relative">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="max-w-3xl mx-auto text-center space-y-3 mb-14">
            <Badge variant="outline" className="text-brand border-brand-border px-3 py-1 text-xs">
              Uncompromising Freedom
            </Badge>
            <h2 className="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-foreground">
              Herbie vs. The $20/Month Cloud Lock-in
            </h2>
            <p className="text-sm sm:text-base text-muted-foreground leading-relaxed">
              Why settle for an expensive, closed subscription when you can own your AI stack?
            </p>
          </div>

          <div className="max-w-4xl mx-auto overflow-x-auto rounded-2xl border border-border bg-card shadow-sm">
            <table className="w-full text-left text-xs sm:text-sm">
              <thead>
                <tr className="border-b border-border bg-muted/40 font-semibold text-muted-foreground">
                  <th className="py-3.5 px-4 sm:px-6">Capability</th>
                  <th className="py-3.5 px-4 sm:px-6 text-brand font-bold">Herbie (Open Source)</th>
                  <th className="py-3.5 px-4 sm:px-6">Commercial SaaS ($20/mo)</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/60">
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">License & Code</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> 100% Free & Open Source (MIT)
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Closed source & proprietary</td>
                </tr>
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">Model Freedom</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> Any Model (Claude 4.5, GPT 5.6, Gemini 3.5, Ollama)
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Locked to one vendor</td>
                </tr>
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">Token Cost Markup</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> 0% (Pay lab directly or $0 offline)
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Flat $20/mo with rate caps</td>
                </tr>
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">Visual Workflows</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> Integrated visual node pipeline canvas
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Not included</td>
                </tr>
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">Data Privacy</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> 100% On-Premise SQLite
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Stored on 3rd party servers</td>
                </tr>
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">Custom Tools & MCP</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> Full MCP 2.0 + Isolated Code Sandbox
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Restricted marketplace</td>
                </tr>
                <tr>
                  <td className="py-3.5 px-4 sm:px-6 font-medium text-foreground">Mascot & Personality</td>
                  <td className="py-3.5 px-4 sm:px-6 text-brand font-semibold flex items-center gap-1.5">
                    <Check className="size-4 text-emerald-500" /> Joyful jumping Herbie! 🦘
                  </td>
                  <td className="py-3.5 px-4 sm:px-6 text-muted-foreground">Sterile spinner</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ====================================================================
          ARCHITECTURE & ENGINEERING DEEP DIVE
          ==================================================================== */}
      <section id="architecture" className="py-20 md:py-28 border-t border-border/40 bg-muted/15 relative">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="max-w-3xl mx-auto text-center space-y-3 mb-14">
            <Badge variant="brand" className="px-3 py-1 text-xs">
              System Architecture
            </Badge>
            <h2 className="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-foreground">
              Powered by Go Concurrency & React 19
            </h2>
            <p className="text-sm sm:text-base text-muted-foreground leading-relaxed">
              Why is Herbie so light on memory yet lightning fast? Under the hood, Herbie pairs a high-concurrency Go backend with a modern TypeScript frontend.
            </p>
          </div>

          <div className="max-w-4xl mx-auto p-6 sm:p-8 rounded-3xl border border-border bg-card shadow-md space-y-6">
            {/* Visual Architecture Topology */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-center text-xs">
              <div className="p-4 rounded-2xl bg-muted/40 border border-border/60 space-y-2">
                <span className="font-mono text-brand font-bold text-sm block">Frontend</span>
                <span className="text-foreground font-semibold block">React 19 + Vite</span>
                <p className="text-muted-foreground text-[11px] leading-relaxed">
                  Tailwind CSS v4 with OKLCH Nordic Sage tokens, sub-millisecond SSE stream parser, responsive canvas.
                </p>
              </div>

              <div className="p-4 rounded-2xl bg-brand/10 border border-brand-border/60 space-y-2 relative">
                <div className="absolute -top-2.5 left-1/2 -translate-x-1/2">
                  <Badge variant="brand" className="text-[9px] px-1.5 py-0 h-4">
                    Core Engine
                  </Badge>
                </div>
                <span className="font-mono text-brand font-bold text-sm block">Backend</span>
                <span className="text-foreground font-semibold block">Go 1.24+ (Golem)</span>
                <p className="text-muted-foreground text-[11px] leading-relaxed">
                  High-concurrency goroutines, zero-garbage JSON streaming, MCP protocol dispatcher, and WAL SQLite.
                </p>
              </div>

              <div className="p-4 rounded-2xl bg-muted/40 border border-border/60 space-y-2">
                <span className="font-mono text-brand font-bold text-sm block">Data & Tools</span>
                <span className="text-foreground font-semibold block">SQLite + MCP Ecosystem</span>
                <p className="text-muted-foreground text-[11px] leading-relaxed">
                  Local document storage, vector embedding indexing, Google Calendar, GitHub, and Slack integrations.
                </p>
              </div>
            </div>

            <div className="pt-4 border-t border-border/60 flex flex-col sm:flex-row items-center justify-between text-xs text-muted-foreground gap-3">
              <div className="flex items-center gap-2">
                <span className="size-2 rounded-full bg-emerald-500" />
                <span>Zero bloated Python dependencies</span>
                <span className="text-muted-foreground/40">·</span>
                <span>Idle memory footprint &lt;35MB RAM</span>
              </div>
              <Button asChild variant="outline" size="sm" className="gap-1 text-xs">
                <a href="https://github.com/abubakarsiddik31/herbie" target="_blank" rel="noreferrer">
                  <span>View Source on GitHub</span>
                  <ExternalLink className="size-3" />
                </a>
              </Button>
            </div>
          </div>
        </div>
      </section>

      {/* ====================================================================
          QUICKSTART SECTION: DEPLOYMENT GUIDES
          ==================================================================== */}
      <section id="quickstart" className="py-20 md:py-28 relative">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="max-w-3xl mx-auto text-center space-y-3 mb-12">
            <Badge variant="outline" className="text-brand border-brand-border px-3 py-1 text-xs">
              Get Started
            </Badge>
            <h2 className="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-foreground">
              Deploy in Seconds
            </h2>
            <p className="text-sm sm:text-base text-muted-foreground leading-relaxed">
              Run locally or ship to your private cloud with one command.
            </p>
          </div>

          <div className="max-w-3xl mx-auto space-y-4">
            <div className="rounded-2xl border border-border bg-card p-5 sm:p-6 shadow-sm space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Terminal className="size-4 text-brand" />
                  <h3 className="text-sm font-bold text-foreground">Option A: Docker Compose (Recommended)</h3>
                </div>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() =>
                    handleCopyCommand(
                      "git clone https://github.com/abubakarsiddik31/herbie.git\ncd herbie\ncp .env.example .env\ndocker compose up -d"
                    )
                  }
                  className="text-xs text-muted-foreground gap-1"
                >
                  <Copy className="size-3" /> Copy
                </Button>
              </div>
              <div className="rounded-xl bg-muted/60 p-3 font-mono text-xs text-foreground overflow-x-auto">
                <p className="text-muted-foreground"># Clone and launch full stack (Go backend + React frontend)</p>
                <p>git clone https://github.com/abubakarsiddik31/herbie.git</p>
                <p>cd herbie</p>
                <p>cp .env.example .env</p>
                <p className="text-brand font-semibold">docker compose up -d</p>
              </div>
            </div>

            <div className="rounded-2xl border border-border bg-card p-5 sm:p-6 shadow-sm space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Code2 className="size-4 text-brand" />
                  <h3 className="text-sm font-bold text-foreground">Option B: Local Dev (Go + Vite)</h3>
                </div>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => handleCopyCommand("make dev")}
                  className="text-xs text-muted-foreground gap-1"
                >
                  <Copy className="size-3" /> Copy
                </Button>
              </div>
              <div className="rounded-xl bg-muted/60 p-3 font-mono text-xs text-foreground overflow-x-auto">
                <p className="text-muted-foreground"># Run both backend and frontend concurrently</p>
                <p className="text-brand font-semibold">make dev</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ====================================================================
          CALL TO ACTION BANNER
          ==================================================================== */}
      <section className="py-16 md:py-24 border-t border-border/40 bg-gradient-to-b from-brand-muted/20 via-background to-background relative overflow-hidden">
        <div className="mx-auto max-w-5xl px-4 sm:px-6 lg:px-8 text-center space-y-6 relative z-10">
          <div className="flex justify-center">
            <AnimatedHerbieLogo mood="celebrate" size="md" interactive={true} />
          </div>

          <h2 className="text-3xl sm:text-5xl font-extrabold tracking-tight text-foreground">
            Ready to Build With Herbie?
          </h2>

          <p className="max-w-xl mx-auto text-sm sm:text-base text-muted-foreground leading-relaxed">
            Take back ownership of your AI workspace. Zero markups, zero telemetry, full visual automation, and pure open source.
          </p>

          <div className="pt-2 flex flex-col sm:flex-row items-center justify-center gap-3">
            <Button asChild variant="brand" size="lg" className="px-8 h-12 text-sm shadow-md">
              <Link to="/chat">
                <span>Launch Herbie Workspace</span>
                <ArrowRight className="size-4 ml-1.5" />
              </Link>
            </Button>
            <Button asChild variant="outline" size="lg" className="px-6 h-12 text-sm">
              <a href="https://github.com/abubakarsiddik31/herbie" target="_blank" rel="noreferrer">
                <span>Star on GitHub</span>
              </a>
            </Button>
          </div>
        </div>
      </section>

      {/* ====================================================================
          FOOTER
          ==================================================================== */}
      <footer className="border-t border-border/40 bg-card/40 py-10 text-xs text-muted-foreground">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-2.5">
            <div className="size-5 rounded-md bg-brand/10 p-0.5 flex items-center justify-center">
              <AnimatedHerbieLogo mood="float" size="sm" interactive={false} />
            </div>
            <span className="font-semibold text-foreground">Herbie AI</span>
            <span>· MIT License · Built with Go & React 19</span>
          </div>

          <div className="flex items-center gap-6">
            <a href="#why-herbie" className="hover:text-foreground transition-colors">
              Why Herbie?
            </a>
            <a href="#features" className="hover:text-foreground transition-colors">
              Features
            </a>
            <a
              href="https://github.com/abubakarsiddik31/herbie"
              target="_blank"
              rel="noreferrer"
              className="hover:text-foreground transition-colors"
            >
              GitHub
            </a>
            <a
              href="https://github.com/abubakarsiddik31/herbie/blob/main/docs/design-system.md"
              target="_blank"
              rel="noreferrer"
              className="hover:text-foreground transition-colors"
            >
              Design System
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
