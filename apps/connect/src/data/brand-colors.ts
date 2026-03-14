/**
 * Brand colors for providers.
 * Keys must match provider IDs from the registry (models.json).
 * Providers not listed here get the default fallback color.
 */
export const providerBrandColors: Record<string, string> = {
  // ── Major providers ──
  openai: '#0D0D0D',
  anthropic: '#D4A373',
  google: '#4285F4',
  mistral: '#FF7000',
  groq: '#F55036',
  deepseek: '#4D6BFE',
  cohere: '#39594D',
  perplexity: '#20808D',
  xai: '#0D0D0D',
  'amazon-bedrock': '#FF9900',
  azure: '#0078D4',
  'azure-cognitive-services': '#0078D4',
  nvidia: '#76B900',
  'fireworks-ai': '#6720FF',
  togetherai: '#054FE5',
  cerebras: '#1E40AF',
  huggingface: '#FFD21E',
  deepinfra: '#7C3AED',
  'novita-ai': '#3B82F6',
  openrouter: '#6366F1',

  // ── Cloud / infra ──
  'google-vertex': '#4285F4',
  'google-vertex-anthropic': '#4285F4',
  scaleway: '#4F0599',
  ovhcloud: '#000E9C',
  vultr: '#007BFC',
  'sap-ai-core': '#0070F2',
  stackit: '#5F1B7A',
  nebius: '#5D35FF',
  'io-net': '#0D0D0D',
  baseten: '#0D0D0D',

  // ── GitHub / GitLab / Vercel ──
  'github-copilot': '#24292E',
  'github-models': '#24292E',
  gitlab: '#FC6D26',
  vercel: '#0D0D0D',
  v0: '#0D0D0D',

  // ── Chinese providers ──
  alibaba: '#FF6A00',
  'alibaba-cn': '#FF6A00',
  'alibaba-coding-plan': '#FF6A00',
  'alibaba-coding-plan-cn': '#FF6A00',
  xiaomi: '#FF6900',
  minimax: '#D01316',
  'minimax-cn': '#D01316',
  'minimax-coding-plan': '#D01316',
  'minimax-cn-coding-plan': '#D01316',
  zhipuai: '#3B5BDB',
  'zhipuai-coding-plan': '#3B5BDB',
  moonshotai: '#5046E4',
  'moonshotai-cn': '#5046E4',
  'kimi-for-coding': '#5046E4',
  stepfun: '#D4A017',
  siliconflow: '#6E29F6',
  'siliconflow-cn': '#6E29F6',
  modelscope: '#6240FF',
  drun: '#2563EB',
  bailing: '#1677FF',
  'qiniu-ai': '#2B6CB0',
  'qihang-ai': '#3B82F6',
  iflowcn: '#FF8500',
  'kuae-cloud-coding-plan': '#3B82F6',
  'zai': '#3B82F6',
  'zai-coding-plan': '#3B82F6',

  // ── Meta / Llama ──
  llama: '#0668E1',

  // ── Inference / routing ──
  friendli: '#2A62DB',
  helicone: '#0EA5E9',
  chutes: '#536AF5',
  cortecs: '#6366F1',
  fastrouter: '#1F36DF',
  kilo: '#617A91',
  inference: '#1A365D',
  inception: '#159999',
  requesty: '#6366F1',
  zenmux: '#6366F1',
  submodel: '#6366F1',
  morph: '#6366F1',

  // ── Cloudflare ──
  'cloudflare-ai-gateway': '#F6821C',
  'cloudflare-workers-ai': '#F6821C',
  'cloudferro-sherlock': '#FD822C',

  // ── Other notable ──
  evroc: '#0B78FB',
  firmware: '#E74860',
  poe: '#5946D2',
  wandb: '#FFCC33',
  upstage: '#4D65FF',
  'privatemode-ai': '#7C3AED',
  'ollama-cloud': '#0D0D0D',
  lmstudio: '#10B981',
  'nano-gpt': '#10B981',
  replicate: '#1A1A2E',
  synthetic: '#0D0D0D',
  venice: '#7C3AED',
  vivgrid: '#3B82F6',
  meganova: '#6366F1',
  lucidquery: '#3B82F6',
  nova: '#3B82F6',
  moark: '#3B82F6',
  berget: '#2563EB',
  '302ai': '#3B82F6',
  aihubmix: '#6366F1',
  abacus: '#2563EB',
  jiekou: '#3B82F6',
  'opencode': '#0D0D0D',
  'opencode-go': '#0D0D0D',
  'perplexity-agent': '#20808D',
}

export const DEFAULT_BRAND_COLOR = '#71717A'

export function getProviderColor(providerId: string): string {
  return providerBrandColors[providerId] ?? DEFAULT_BRAND_COLOR
}
