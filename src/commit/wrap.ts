// Wrap text only between sentences (never mid-sentence). If a single sentence
// exceeds the width, we keep it on one line instead of breaking it.
export function wrapText(text: string, width: number): string {
  if (width <= 0) return text;
  // Preserve explicit newlines by processing paragraph-by-paragraph
  const paragraphs = text.split("\n");
  const out: string[] = [];
  for (const para of paragraphs) {
    if (!para.trim()) { out.push(para); continue; }
    const sentences = splitSentences(para);
    if (sentences.length === 0) { out.push(para); continue; }
    let line = "";
    for (const s of sentences) {
      const candidate = line ? (line + " " + s) : s;
      if (line && candidate.length > width) {
        // Start a new line at sentence boundary
        out.push(line);
        line = s; // sentence may exceed width; keep whole
      } else {
        line = candidate;
      }
    }
    if (line) out.push(line);
  }
  return out.join("\n");
}

function splitSentences(s: string): string[] {
  // Split on whitespace following common sentence terminators . ! ?
  // Keep punctuation with the sentence. Handles simple cases; avoids breaking on abbreviations heuristically.
  const parts: string[] = [];
  let buffer = "";
  for (let i = 0; i < s.length; i++) {
    const ch = s[i];
    buffer += ch;
    if (ch === "." || ch === "!" || ch === "?") {
      // Look ahead for space(s)
      let j = i + 1;
      while (j < s.length && s[j] === " ") j++;
      // End of sentence when followed by space or end-of-string
      if (j > i + 1 || j === s.length) {
        parts.push(buffer.trim());
        buffer = "";
        i = j - 1;
      }
    }
  }
  const tail = buffer.trim();
  if (tail) parts.push(tail);
  return parts;
}

export function wrapCommitMessage(msg: string): string {
  return wrapText(msg, 80);
}
