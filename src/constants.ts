/**
 * Constants for token and context limits
 */
export const TOKEN_LIMITS = {
  /** Maximum tokens allowed in context window */
  MAX_CONTEXT_TOKENS: 90_000,
  /** Maximum characters allowed in context */
  MAX_CONTEXT_CHARS: 280_000,
  /** Maximum characters for summary output */
  MAX_SUMMARY_CHARS: 6_000,
  /** Token budget per chunk during summarization */
  CHUNK_BUDGET_TOKENS: 6_000,
  /** Token budget for final combined summary */
  FINAL_BUDGET_TOKENS: 12_000,
  /** Token estimate for small model threshold */
  SMALL_MODEL_THRESHOLD: 2_000,
} as const;

/**
 * Constants for diff processing and limits
 */
export const DIFF_LIMITS = {
  /** Line count threshold to omit raw diff entirely */
  RAW_DIFF_OMIT_LINE_THRESHOLD: 800,
  /** Character count threshold to omit raw diff entirely */
  RAW_DIFF_OMIT_CHAR_THRESHOLD: 160_000,
  /** Hard cap on total diff lines before truncation */
  RAW_DIFF_HARD_CAP_LINES: 300,
  /** Character cap for raw diff block */
  RAW_DIFF_CHAR_CAP: 80_000,
  /** Maximum characters for non-source file blocks */
  MAX_NON_SOURCE_BLOCK_CHARS: 120_000,
} as const;

/**
 * Constants for suggestion generation
 */
export const GENERATION_CONFIG = {
  /** Temperature for suggestion generation (higher = more diverse) */
  SUGGESTION_TEMPERATURE: 0.6,
  /** Max tokens for standard models */
  MAX_TOKENS_STANDARD: 768,
  /** Max tokens for reasoning models (e.g., gpt-5) */
  MAX_TOKENS_REASONING: 1500,
  /** Default number of suggestions in interactive mode */
  DEFAULT_SUGGESTIONS: 5,
  /** Default number of suggestions in non-interactive mode */
  DEFAULT_SUGGESTIONS_CI: 1,
  /** Min/max range for suggestion count */
  SUGGESTIONS_MIN: 1,
  SUGGESTIONS_MAX: 10,
  /** Min/max range for concurrency settings */
  CONCURRENCY_MIN: 1,
  CONCURRENCY_MAX: 20,
  /** Default concurrency for summarization */
  DEFAULT_SUMMARIZE_CONCURRENCY: 10,
  /** Default concurrency for suggestion generation */
  DEFAULT_SUGGESTION_CONCURRENCY: 10,
} as const;

/**
 * Constants for diff truncation
 */
export const TRUNCATION_TARGETS = {
  /** Truncation levels for progressive reduction */
  LINE_LIMITS: [600, 400, 200],
} as const;

/**
 * Non-source directory prefixes to filter from diffs
 */
export const NON_SOURCE_DIR_PREFIXES = [
  "dist/",
  "bin/",
  "build/",
  "out/",
  "coverage/",
  "node_modules/",
  "vendor/",
  "tmp/",
  "temp/",
] as const;

/**
 * File extensions for non-source files to filter from diffs
 */
export const NON_SOURCE_EXTENSIONS = [
  ".txt",
  ".log",
  ".csv",
  ".tsv",
  ".lock",
  ".patch",
  ".ico",
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".mp4",
  ".mp3",
  ".zip",
  ".tar",
  ".gz",
  ".tgz",
  ".xz",
  ".7z",
] as const;

/**
 * Default ignore prefixes for diff filtering
 */
export const DEFAULT_IGNORE_PREFIXES = [
  "dist/",
  "node_modules/",
  "build/",
  "out/",
  "coverage/",
  "target/",
  ".next/",
  ".turbo/",
] as const;

/**
 * Limits for display and processing
 */
export const DISPLAY_LIMITS = {
  /** Maximum files to show in file list */
  MAX_FILES_DISPLAY: 100,
  /** Maximum highlights to extract */
  MAX_HIGHLIGHTS: 10,
  /** Maximum impact candidates to show */
  MAX_IMPACT_FILES: 3,
  /** Maximum area summaries to show */
  MAX_AREA_SUMMARIES: 5,
  /** Maximum test files to display */
  MAX_TEST_FILES: 10,
  /** Maximum characters for issue body preview */
  MAX_ISSUE_BODY_CHARS: 200,
  /** Minimum keyword length for matching */
  MIN_KEYWORD_LENGTH: 3,
  /** Maximum heuristic highlights to extract if no explicit section */
  MAX_HEURISTIC_HIGHLIGHTS: 5,
  /** Maximum lines to show in commit message */
  MAX_COMMIT_MESSAGE_LINES: 2,
} as const;
