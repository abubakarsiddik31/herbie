import {
  ArrowLeftRight,
  BookMarked,
  BookOpen,
  Clock,
  CloudSun,
  Coins,
  Dog,
  Flame,
  GitFork,
  Globe,
  Laugh,
  MapPin,
  Quote,
  Rss,
  Search,
  Wifi,
  type LucideIcon,
} from "lucide-react";
import type { ToolParam } from "@/lib/types";

// A tool template is a pre-vetted starting point: free, keyless, read-only
// HTTPS APIs that render valid tool configs. Picking one prefills the editor;
// the user can adjust anything before saving.
export interface ToolTemplate {
  slug: string;
  title: string;
  category: string;
  tagline: string;
  icon: LucideIcon;
  tool: ToolTemplateConfig;
}

export interface ToolTemplateConfig {
  name: string;
  description: string;
  method: string;
  urlTemplate: string;
  params: ToolParam[];
  bodyTemplate: string;
  headers: Record<string, string>;
  requireApproval: boolean;
  enabled: boolean;
}

export const TEMPLATE_CATEGORIES = [
  "Weather & places",
  "Developer",
  "News & knowledge",
  "Finance",
  "Utilities",
  "Fun",
] as const;

export const TEMPLATES: ToolTemplate[] = [
  {
    slug: "weather-current",
    title: "Weather report",
    category: "Weather & places",
    tagline: "Temperature, humidity and wind for any coordinates",
    icon: CloudSun,
    tool: {
      name: "weather_current",
      description:
        "Get current weather for a location: temperature, humidity, and wind speed. Requires latitude and longitude in decimal degrees.",
      method: "GET",
      urlTemplate:
        "https://api.open-meteo.com/v1/forecast?current=temperature_2m,relative_humidity_2m,wind_speed_10m",
      params: [
        { name: "latitude", in: "query", type: "number", required: true, description: "Latitude in decimal degrees, e.g. 52.52" },
        { name: "longitude", in: "query", type: "number", required: true, description: "Longitude in decimal degrees, e.g. 13.41" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "geocode-city",
    title: "City lookup",
    category: "Weather & places",
    tagline: "City names to coordinates, ready for weather",
    icon: MapPin,
    tool: {
      name: "geocode_city",
      description:
        "Look up a city or place by name and get its coordinates, country, and timezone. Use this before weather_current when only a place name is known.",
      method: "GET",
      urlTemplate: "https://geocoding-api.open-meteo.com/v1/search?language=en&format=json",
      params: [
        { name: "city", in: "query", type: "string", required: true, description: "City or place name" },
        { name: "count", in: "query", type: "number", required: false, description: "Maximum number of results (default 10)" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "github-repo",
    title: "GitHub repo details",
    category: "Developer",
    tagline: "Stars, forks and language for any repo",
    icon: GitFork,
    tool: {
      name: "get_github_repo",
      description:
        "Get details for a GitHub repository: description, stars, forks, open issues, primary language, and default branch.",
      method: "GET",
      urlTemplate: "https://api.github.com/repos/{{owner}}/{{repo}}",
      params: [
        { name: "owner", in: "path", type: "string", required: true, description: "Repository owner (user or organization)" },
        { name: "repo", in: "path", type: "string", required: true, description: "Repository name" },
      ],
      bodyTemplate: "",
      headers: { Accept: "application/vnd.github+json" },
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "github-search",
    title: "GitHub search",
    category: "Developer",
    tagline: "Find repositories by keyword, sorted by stars",
    icon: Search,
    tool: {
      name: "search_github_repos",
      description: "Search GitHub repositories by keyword. Returns the top matches sorted by stars.",
      method: "GET",
      urlTemplate: "https://api.github.com/search/repositories?sort=stars&order=desc",
      params: [
        { name: "query", in: "query", type: "string", required: true, description: "Search keywords, e.g. go cli" },
        { name: "per_page", in: "query", type: "number", required: false, description: "Results per page, 1-100 (default 10)" },
      ],
      bodyTemplate: "",
      headers: { Accept: "application/vnd.github+json" },
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "read-webpage",
    title: "Webpage reader",
    category: "Developer",
    tagline: "Turn any public page into clean text",
    icon: Globe,
    tool: {
      name: "read_webpage",
      description:
        "Fetch a public web page and return its main content as clean text. Use for documentation pages, blog posts, and articles.",
      method: "GET",
      urlTemplate: "https://r.jina.ai/{{url}}",
      params: [
        { name: "url", in: "path", type: "string", required: true, description: "Full page URL including https://" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "hacker-news-search",
    title: "Hacker News search",
    category: "News & knowledge",
    tagline: "Full-text search over HN stories",
    icon: Search,
    tool: {
      name: "search_hacker_news",
      description: "Search Hacker News stories by keyword. Returns titles, URLs, points, and comment counts.",
      method: "GET",
      urlTemplate: "https://hn.algolia.com/api/v1/search?tags=story",
      params: [
        { name: "query", in: "query", type: "string", required: true, description: "Search keywords" },
        { name: "hitsPerPage", in: "query", type: "number", required: false, description: "Results per page (default 20)" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "hacker-news-top",
    title: "Hacker News front page",
    category: "News & knowledge",
    tagline: "What the HN front page looks like right now",
    icon: Flame,
    tool: {
      name: "hacker_news_front_page",
      description: "Get the current Hacker News front page: top stories with links, points, and comment counts.",
      method: "GET",
      urlTemplate: "https://hn.algolia.com/api/v1/search?tags=front_page",
      params: [
        { name: "hitsPerPage", in: "query", type: "number", required: false, description: "Results per page (default 20)" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "wikipedia-summary",
    title: "Wikipedia summary",
    category: "News & knowledge",
    tagline: "The lead section of any Wikipedia article",
    icon: BookOpen,
    tool: {
      name: "wikipedia_summary",
      description:
        "Get the summary section of a Wikipedia article. Use underscores for spaces in the title, e.g. Ada_Lovelace.",
      method: "GET",
      urlTemplate: "https://en.wikipedia.org/api/rest_v1/page/summary/{{title}}",
      params: [
        { name: "title", in: "path", type: "string", required: true, description: "Article title with underscores for spaces" },
      ],
      bodyTemplate: "",
      headers: { "User-Agent": "Herbie/1.0" },
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "wikipedia-search",
    title: "Wikipedia search",
    category: "News & knowledge",
    tagline: "Find articles by keyword",
    icon: BookOpen,
    tool: {
      name: "search_wikipedia",
      description: "Search Wikipedia articles by keyword and return titles and snippets of the best matches.",
      method: "GET",
      urlTemplate: "https://en.wikipedia.org/w/api.php?action=query&list=search&format=json",
      params: [
        { name: "srsearch", in: "query", type: "string", required: true, description: "Search keywords" },
        { name: "srlimit", in: "query", type: "number", required: false, description: "Maximum number of results (default 10)" },
      ],
      bodyTemplate: "",
      headers: { "User-Agent": "Herbie/1.0" },
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "rss-reader",
    title: "RSS reader",
    category: "News & knowledge",
    tagline: "Recent items from any blog or news feed",
    icon: Rss,
    tool: {
      name: "read_rss_feed",
      description:
        "Read an RSS or Atom feed and return its recent items with titles, links, and publish dates. Use for blogs, podcasts, and news feeds.",
      method: "GET",
      urlTemplate: "https://api.rss2json.com/v1/api.json",
      params: [
        { name: "rss_url", in: "query", type: "string", required: true, description: "The RSS or Atom feed URL" },
        { name: "count", in: "query", type: "number", required: false, description: "Number of items to return (default 10)" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "define-word",
    title: "Dictionary",
    category: "News & knowledge",
    tagline: "Definitions and phonetics for English words",
    icon: BookMarked,
    tool: {
      name: "define_word",
      description: "Get dictionary definitions, phonetics, and example usage for an English word.",
      method: "GET",
      urlTemplate: "https://api.dictionaryapi.dev/api/v2/entries/en/{{word}}",
      params: [
        { name: "word", in: "path", type: "string", required: true, description: "The English word to define" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "exchange-rates",
    title: "Exchange rates",
    category: "Finance",
    tagline: "Live FX rates from any base currency",
    icon: ArrowLeftRight,
    tool: {
      name: "get_exchange_rates",
      description:
        "Get current foreign-exchange rates relative to a base currency. Returns what one unit of the base currency is worth in many currencies.",
      method: "GET",
      urlTemplate: "https://open.er-api.com/v6/latest/{{base_currency}}",
      params: [
        { name: "base_currency", in: "path", type: "string", required: true, description: "Three-letter currency code, e.g. USD" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "crypto-price",
    title: "Crypto prices",
    category: "Finance",
    tagline: "Prices and 24h change for any coin",
    icon: Coins,
    tool: {
      name: "get_crypto_price",
      description:
        "Get the current price and 24h change for cryptocurrencies in fiat currencies. Ids are CoinGecko ids like bitcoin, ethereum, solana.",
      method: "GET",
      urlTemplate: "https://api.coingecko.com/api/v3/simple/price?include_24hr_change=true",
      params: [
        { name: "ids", in: "query", type: "string", required: true, description: "Comma-separated CoinGecko ids, e.g. bitcoin,ethereum" },
        { name: "vs_currencies", in: "query", type: "string", required: true, description: "Comma-separated fiat currencies, e.g. usd,eur" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "ip-lookup",
    title: "IP lookup",
    category: "Utilities",
    tagline: "Where an IP address lives and who runs it",
    icon: Wifi,
    tool: {
      name: "lookup_ip",
      description: "Look up the geographic location, ISP, and timezone for an IPv4 or IPv6 address.",
      method: "GET",
      urlTemplate: "https://ipwho.is/{{ip}}",
      params: [
        { name: "ip", in: "path", type: "string", required: true, description: "IPv4 or IPv6 address" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "current-time",
    title: "World clock",
    category: "Utilities",
    tagline: "Current time in any IANA time zone",
    icon: Clock,
    tool: {
      name: "get_current_time",
      description:
        "Get the current date and time in a specific IANA time zone, e.g. Europe/London, America/New_York, Asia/Tokyo.",
      method: "GET",
      urlTemplate: "https://timeapi.io/api/Time/current/zone",
      params: [
        { name: "timeZone", in: "query", type: "string", required: true, description: "IANA time zone name, e.g. Europe/London" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "random-joke",
    title: "Joke of the day",
    category: "Fun",
    tagline: "A clean joke on demand",
    icon: Laugh,
    tool: {
      name: "random_joke",
      description: "Get a random joke. Category: Any, Programming, Misc, Pun, Spooky, or Christmas.",
      method: "GET",
      urlTemplate: "https://v2.jokeapi.dev/joke/{{category}}?safe-mode",
      params: [
        { name: "category", in: "path", type: "string", required: true, description: "Joke category: Any, Programming, Misc, Pun, Spooky, or Christmas" },
      ],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "random-quote",
    title: "Quote",
    category: "Fun",
    tagline: "A short quote with its author",
    icon: Quote,
    tool: {
      name: "random_quote",
      description: "Get a random inspirational quote with its author.",
      method: "GET",
      urlTemplate: "https://dummyjson.com/quotes/random",
      params: [],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
  {
    slug: "random-dog",
    title: "Dog photo",
    category: "Fun",
    tagline: "A random dog photo, for morale",
    icon: Dog,
    tool: {
      name: "random_dog_image",
      description: "Get the URL of a random dog photo. Use for a light-hearted interlude.",
      method: "GET",
      urlTemplate: "https://dog.ceo/api/breeds/image/random",
      params: [],
      bodyTemplate: "",
      headers: {},
      requireApproval: false,
      enabled: true,
    },
  },
];

// Host of a template's API, for display; falls back to the raw template.
export function templateHost(urlTemplate: string): string {
  try {
    return new URL(urlTemplate.replace(/\{\{[A-Za-z_][A-Za-z0-9_]*\}\}/g, "x")).host;
  } catch {
    return urlTemplate;
  }
}
