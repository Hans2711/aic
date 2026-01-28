import type { ProviderName } from "../config";
import { Env, getEnv } from "../config";
import { generateText } from 'ai';
import { createOpenAI } from '@ai-sdk/openai';
import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { debugLog } from "../debug";

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

function createProviderInstance(provider: ProviderName, apiKey: string, baseUrl?: string) {
  switch (provider) {
    case "claude":
      return createAnthropic({ apiKey });
    case "gemini":
      return createGoogleGenerativeAI({ apiKey });
    case "custom":
      return createOpenAI({ 
        apiKey, 
        baseURL: baseUrl || getEnv(Env.CUSTOM_BASE_URL) || "http://127.0.0.1:1234",
        compatibility: 'compatible', // Enable compatibility mode for custom providers
      });
    case "openai":
    default:
      return createOpenAI({ apiKey });
  }
}

// Helper function to detect GPT-5 reasoning models
function isReasoningModel(model: string): boolean {
  const lowerModel = model.toLowerCase();
  return lowerModel.startsWith('gpt-5') || 
         lowerModel.startsWith('o1') || 
         lowerModel.startsWith('o3') ||
         lowerModel.startsWith('o4');
}

export function newProviderClient(provider: ProviderName, apiKey: string, baseUrl?: string): ProviderClient {
  const providerInstance = createProviderInstance(provider, apiKey, baseUrl);

  return {
    async chat({ model, messages, maxTokens, temperature, n = 1 }): Promise<CompletionResponse> {
      const choices: string[] = [];
      let rawResult: any;

      // Detect if this is a reasoning model
      const isReasoning = provider === "openai" && isReasoningModel(model);
      
      // For reasoning models, significantly increase token limit to allow both reasoning and output
      let effectiveMaxTokens = maxTokens;
      if (isReasoning && (!maxTokens || maxTokens < 3000)) {
        effectiveMaxTokens = 4000; // Increased from 1500 to 4000
        debugLog(`Reasoning model detected: ${model}, increasing maxTokens to ${effectiveMaxTokens}`);
      }

      // Handle multiple completions (n > 1) by making multiple requests
      for (let i = 0; i < n; i++) {
        try {
          // Create abort controller with 2-minute timeout
          const abortController = new AbortController();
          const timeoutId = setTimeout(() => abortController.abort(), 120000);

          // Build generateText options
          const generateOptions: any = {
            model: providerInstance(model),
            messages: messages.map(m => ({
              role: m.role,
              content: m.content,
            })),
            maxTokens: effectiveMaxTokens,
            temperature,
            abortSignal: abortController.signal,
          };

          // Add reasoning-specific options for OpenAI reasoning models
          if (isReasoning) {
            generateOptions.providerOptions = {
              openai: {
                reasoningEffort: 'low', // Use 'low' effort to balance speed and quality
              }
            };
            debugLog(`Using reasoningEffort: 'low' for model ${model}`);
          }

          const result = await generateText(generateOptions);

          clearTimeout(timeoutId);

          // Extract text from result
          debugLog(`AI SDK result text length: ${result.text?.length || 0}`);
          debugLog(`AI SDK finish reason: ${result.finishReason}`);
          debugLog(`AI SDK usage:`, JSON.stringify(result.usage));
          
          if (result.text && result.text.trim()) {
            choices.push(result.text.trim());
          } else {
            debugLog(`WARNING: Empty response from AI SDK for model ${model}`);
            
            // If we got empty response from reasoning model, throw error to trigger fallback
            if (isReasoning) {
              throw new Error(`Empty response from reasoning model ${model}. The model may be using all tokens for internal reasoning. Try using reasoningEffort: 'minimal' or a non-reasoning model.`);
            }
          }

          // Store raw result for metadata access
          rawResult = {
            ...result,
            usage: result.usage,
            finishReason: result.finishReason,
            providerMetadata: result.providerMetadata,
          };

          // Log reasoning tokens if available (for GPT-5 models)
          const reasoningTokens = (result as any).usage?.reasoningTokens;
          if (reasoningTokens !== undefined) {
            debugLog(`reasoning tokens: ${reasoningTokens}`);
          }

        } catch (error: any) {
          // Better error messages from AI SDK
          if (error.name === 'AbortError') {
            throw new Error(`Request timeout after 120 seconds for model ${model}`);
          }
          
          debugLog('AI SDK error:', error.message || error);
          throw new Error(`${provider} error: ${error.message || String(error)}`);
        }
      }

      return { choices, raw: rawResult };
    },

    async embed(_text: string): Promise<number[]> {
      // Embeddings not currently used in this application
      return [];
    },

    async countTokens(modelId: string, text: string): Promise<number> {
      try {
        // Use AI SDK's built-in token counting
        const model = providerInstance(modelId);
        
        // The AI SDK models have a countPromptTokens method
        if (typeof (model as any).doCountTokens === 'function') {
          const result = await (model as any).doCountTokens({ prompt: text });
          return result;
        }

        // For OpenAI models, we can estimate using their tokenizer
        // GPT models use roughly 4 characters per token
        // This is a reasonable approximation for most models
        const estimate = Math.ceil(text.length / 4);
        debugLog(`token count estimate for ${modelId}: ${estimate} tokens`);
        return estimate;
        
      } catch (error) {
        // Fallback to character-based estimation
        const estimate = Math.ceil(text.length / 4);
        debugLog(`token count fallback for ${modelId}: ${estimate} tokens`);
        return estimate;
      }
    },
  };
}
