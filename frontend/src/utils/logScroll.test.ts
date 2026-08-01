import { describe, expect, it } from "vitest";
import { isLogScrollKey, shouldFreezeLogViewport } from "./logScroll";

describe("shouldFreezeLogViewport", () => {
  it("mantém a lista congelada durante qualquer interação do usuário", () => {
    expect(
      shouldFreezeLogViewport({
        autoScroll: true,
        pinnedToLatest: true,
        userInteracting: true,
      }),
    ).toBe(true);
  });

  it("só aplica imediatamente quando está ancorado e sem interação", () => {
    expect(
      shouldFreezeLogViewport({
        autoScroll: true,
        pinnedToLatest: true,
        userInteracting: false,
      }),
    ).toBe(false);
  });

  it("congela a janela quando o Follow esta desligado", () => {
    expect(
      shouldFreezeLogViewport({
        autoScroll: false,
        pinnedToLatest: true,
        userInteracting: false,
      }),
    ).toBe(true);
  });

  it("reconhece apenas teclas que movimentam a janela de logs", () => {
    expect(isLogScrollKey("PageDown")).toBe(true);
    expect(isLogScrollKey("ArrowUp")).toBe(true);
    expect(isLogScrollKey("Enter")).toBe(false);
  });
});
