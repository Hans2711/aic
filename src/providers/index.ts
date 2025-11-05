import type { ProviderName } from "../config";
import { Env, getEnv } from "../config";

export type ChatMessage = { role: "system" | "user" | "assistant"; content: string };

export interface CompletionResponse {
  choices: string[];
  raw?: unknown;
}

export interface ProviderClient {
  chat(opts: {
    model: string;
    messages: ChatMessage[];
    maxTokens?: number;
    temperature?: number;
    n?: number;
  }): Promise<CompletionResponse>;
  embed(text: string): Promise<number[]>;
  countTokens?(model: string, text: string): Promise<number>;
}

export function getApiKeyForProvider(provider: ProviderName): string {
  switch (provider) {
    case "claude": return getEnv(Env.CLAUDE_API_KEY);
    case "gemini": return getEnv(Env.GEMINI_API_KEY);
    case "custom": return getEnv(Env.CUSTOM_API_KEY); // may be empty
    default: return getEnv(Env.OPENAI_API_KEY);
  }
}

export function newProviderClient(provider: ProviderName, apiKey: string, baseUrl?: string): ProviderClient {
  async function openaiChat(model: string, messages: ChatMessage[], maxTokens?: number, temperature?: number, n?: number): Promise<CompletionResponse> {
    // Prefer the appropriate token budget key based on model family
    const useCompletionKey = /\bgpt-5\b/i.test(model) || /\bo4\b/i.test(model);
    const makeBody = (useMaxCompletion: boolean, includeTemp: boolean) => {
      const b: any = { model, messages };
      if (includeTemp && typeof temperature === "number") b.temperature = temperature;
      if (maxTokens) {
        if (useMaxCompletion) b.max_completion_tokens = maxTokens; else b.max_tokens = maxTokens;
      }
      if (n && n > 1) b.n = n;
      return b;
    };
    let includeTemp = true;
    let body: any = makeBody(useCompletionKey, includeTemp);
    let resp = await fetch("https://api.openai.com/v1/chat/completions", {
      method: "POST",
      headers: { "Authorization": `Bearer ${apiKey}`, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    let data: any = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      const msg = data?.error?.message || "";
      // Retry switching token key if server complains
      if (/Unsupported parameter:\s*'max_tokens'/i.test(msg) && maxTokens) {
        body = makeBody(true, includeTemp);
        resp = await fetch("https://api.openai.com/v1/chat/completions", {
          method: "POST",
          headers: { "Authorization": `Bearer ${apiKey}`, "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        data = await resp.json().catch(() => ({}));
      } else if (/Unsupported parameter:\s*'max_completion_tokens'/i.test(msg) && maxTokens) {
        body = makeBody(false, includeTemp);
        resp = await fetch("https://api.openai.com/v1/chat/completions", {
          method: "POST",
          headers: { "Authorization": `Bearer ${apiKey}`, "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        data = await resp.json().catch(() => ({}));
      } else if (/Unsupported value:\s*'temperature'/i.test(msg) && includeTemp) {
        // Remove temperature if the model only supports default
        includeTemp = false;
        body = makeBody(useCompletionKey, includeTemp);
        resp = await fetch("https://api.openai.com/v1/chat/completions", {
          method: "POST",
          headers: { "Authorization": `Bearer ${apiKey}`, "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        data = await resp.json().catch(() => ({}));
      }
    }
    if (!resp.ok) throw new Error(`openai http ${resp.status}: ${data?.error?.message || resp.statusText}`);
    const choices = (data.choices || []).map((c: any) => (c?.message?.content || "").trim());
    return { choices, raw: data };
  }

  async function claudeChat(model: string, messages: ChatMessage[], maxTokens?: number, temperature?: number): Promise<CompletionResponse> {
    const sys = messages.find((m) => m.role === "system")?.content || "";
    const userMsgs = messages.filter((m) => m.role !== "system").map((m) => ({ role: m.role, content: m.content }));
    const body: any = { model, messages: userMsgs };
    if (sys) body.system = sys;
    body.max_tokens = maxTokens ?? 256;
    if (temperature !== undefined) body.temperature = temperature;
    const resp = await fetch("https://api.anthropic.com/v1/messages", {
      method: "POST",
      headers: {
        "x-api-key": apiKey,
        "content-type": "application/json",
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify(body),
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(`claude http ${resp.status}: ${data?.error?.message || resp.statusText}`);
    const text = (data?.content || []).map((c: any) => (c?.text || "")).join("");
    return { choices: [text.trim()], raw: data };
  }

  async function geminiChat(model: string, messages: ChatMessage[], maxTokens?: number, temperature?: number, n?: number): Promise<CompletionResponse> {
    const sys = messages.find((m) => m.role === "system")?.content || "";
    const contents = messages.filter((m) => m.role !== "system").map((m) => ({ role: m.role === "assistant" ? "model" : m.role, parts: [{ text: m.content }] }));
    const genConfig: any = {};
    if (maxTokens) genConfig.maxOutputTokens = maxTokens;
    if (temperature !== undefined) genConfig.temperature = temperature;
    if (n && n > 1) genConfig.candidateCount = n;
    genConfig.responseMimeType = "text/plain";
    if (contents.length === 0) contents.push({ role: "user", parts: [{ text: "" }] });
    const body: any = { contents, generationConfig: genConfig };
    if (sys) body.systemInstruction = { parts: [{ text: sys }] };
    const url = `https://generativelanguage.googleapis.com/v1beta/models/${encodeURIComponent(model)}:generateContent?key=${encodeURIComponent(apiKey)}`;
    const resp = await fetch(url, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
    const data = await resp.json();
    if (!resp.ok) throw new Error(`gemini http ${resp.status}: ${data?.error?.message || resp.statusText}`);
    const choices = (data?.candidates || []).map((c: any) => (c?.content?.parts || []).map((p: any) => p?.text || "").join("").trim());
    return { choices: choices.length ? choices : [""], raw: data };
  }

  async function customChat(model: string, messages: ChatMessage[], maxTokens?: number, temperature?: number, n?: number): Promise<CompletionResponse> {
    const base = (baseUrl || getEnv(Env.CUSTOM_BASE_URL) || "http://127.0.0.1:1234").replace(/\/$/, "");
    const path = getEnv(Env.CUSTOM_CHAT_COMPLETIONS_PATH) || "/v1/chat/completions";
    const url = base + (path.startsWith("/") ? path : "/" + path);
    let body: any = { model, messages, temperature };
    if (maxTokens) body.max_tokens = maxTokens;
    if (n && n > 1) body.n = n;
    const headers: Record<string, string> = { "content-type": "application/json" };
    if (apiKey) headers["Authorization"] = `Bearer ${apiKey}`;
    let resp = await fetch(url, { method: "POST", headers, body: JSON.stringify(body) });
    let data: any = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      const msg = typeof data === 'string' ? data : data?.error?.message || "";
      // Retry handling for compatibility
      if (/Unsupported parameter: 'max_tokens'/i.test(msg)) {
        body = { model, messages, temperature };
        if (maxTokens) body.max_completion_tokens = maxTokens;
        resp = await fetch(url, { method: "POST", headers, body: JSON.stringify(body) });
        data = await resp.json().catch(() => ({}));
      } else if (/Unsupported value: 'temperature'/i.test(msg) && temperature !== undefined) {
        body = { model, messages };
        if (maxTokens) body.max_tokens = maxTokens;
        resp = await fetch(url, { method: "POST", headers, body: JSON.stringify(body) });
        data = await resp.json().catch(() => ({}));
      }
      if (!resp.ok) throw new Error(`custom http ${resp.status}: ${typeof data === 'string' ? data : data?.error?.message || resp.statusText}`);
    }
    // Merge choices content (and strip reasoning tags if present)
    const join = (data?.choices || []).map((c: any) => (c?.message?.content || "")).join("\n");
    const cleaned = stripReasoning(join).trim();
    if (cleaned) return { choices: [cleaned], raw: data };
    const choices = (data?.choices || []).map((c: any) => (c?.message?.content || "").trim());
    return { choices, raw: data };
  }

  function stripReasoning(s: string): string {
    return s.replace(/<think\b[^>]*>[\s\S]*?<\/think>/gi, "").replace(/<\/??think\b[^>]*>/gi, "");
  }

  return {
    async chat({ model, messages, maxTokens, temperature, n = 1 }): Promise<CompletionResponse> {
      const choices: string[] = [];
      let raw: unknown = undefined;
      const run = async () => {
        if (provider === "claude") return await claudeChat(model, messages, maxTokens, temperature);
        if (provider === "gemini") return await geminiChat(model, messages, maxTokens, temperature, n);
        if (provider === "custom") return await customChat(model, messages, maxTokens, temperature, n);
        return await openaiChat(model, messages, maxTokens, temperature, n);
      };
      for (let i = 0; i < Math.max(1, n); i++) {
        const res = await run();
        raw = res.raw;
        for (const c of res.choices) {
          const t = (c || "").trim();
          if (t) choices.push(t);
        }
      }
      return { choices, raw };
    },

    async embed(_text: string): Promise<number[]> {
      return [];
    },
    async countTokens(_modelId: string, text: string): Promise<number> {
      // Fallback rough estimate: ~4 chars per token
      return Math.ceil([...text].length / 4);
    },
  };
}
