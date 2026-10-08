/** @jest-environment node */
import { getTranslation } from "@/i18n/getTranslation";
import { skipToMainContentLabel } from "@/i18n/skipToMainContentCatalog";
test("skip link localized", () => {
  expect(getTranslation("common.skipToMainContent", "en")).toBe("Skip to main content");
  expect(getTranslation("common.skipToMainContent", "es")).toBe("Saltar al contenido principal");
  expect(getTranslation("common.skipToMainContent", "es-AR")).toBe("Saltar al contenido principal");
});

test("skip link uses guest catalog for non-operator locales (#519)", () => {
  expect(skipToMainContentLabel("ru")).toBe("Перейти к основному содержанию");
  expect(skipToMainContentLabel("th")).toBe("ข้ามไปยังเนื้อหาหลัก");
  expect(skipToMainContentLabel("zh")).toBe("跳到主要内容");
  expect(skipToMainContentLabel("fr")).toBe("Aller au contenu principal");
  expect(skipToMainContentLabel("es-AR")).toBe("Saltar al contenido principal");
});
