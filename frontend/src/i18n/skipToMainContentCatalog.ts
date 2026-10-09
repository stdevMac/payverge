/** Slim 21-locale skip-link catalog. Do not import full guest-messages here — this sits on every route. */
export const SKIP_TO_MAIN_CONTENT: Record<string, string> = {
  "ar": "تخطي إلى المحتوى الرئيسي",
  "da": "Spring til hovedindhold",
  "de": "Zum Hauptinhalt springen",
  "en": "Skip to main content",
  "es-AR": "Saltar al contenido principal",
  "es": "Saltar al contenido principal",
  "fr": "Aller au contenu principal",
  "hi": "मुख्य सामग्री पर जाएँ",
  "it": "Vai al contenuto principale",
  "ja": "メインコンテンツへスキップ",
  "ko": "본문으로 건너뛰기",
  "nl": "Naar hoofdinhoud",
  "no": "Hopp til hovedinnhold",
  "pl": "Przejdź do głównej treści",
  "pt": "Pular para o conteúdo principal",
  "ru": "Перейти к основному содержанию",
  "sv": "Hoppa till huvudinnehåll",
  "th": "ข้ามไปยังเนื้อหาหลัก",
  "tr": "Ana içeriğe atla",
  "vi": "Bỏ qua đến nội dung chính",
  "zh": "跳到主要内容",
};

export function skipToMainContentLabel(locale: string | null | undefined): string | undefined {
  const raw = (locale || "").trim();
  if (!raw) return undefined;
  if (SKIP_TO_MAIN_CONTENT[raw]) return SKIP_TO_MAIN_CONTENT[raw];
  const short = raw.toLowerCase().split(/[-_]/)[0];
  if (short === "es" && SKIP_TO_MAIN_CONTENT["es"]) return SKIP_TO_MAIN_CONTENT["es"];
  return SKIP_TO_MAIN_CONTENT[short];
}
