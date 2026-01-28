export function stripLeadingListMarker(s: string): string {
  const orig = s;
  let t = s.trimStart();
  for (let i = 0; i < 2; i++) {
    if (!t) break;
    const m = t.match(/^(\d{1,4}[\.\:\)\]]|[-*+])\s+/);
    if (m) {
      t = t.slice(m[0].length);
      continue;
    }
    break;
  }
  return t || orig;
}
