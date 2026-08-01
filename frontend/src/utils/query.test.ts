import { describe, expect, it } from "vitest";
import { syncSearchParams } from "./query";

describe("syncSearchParams", () => {
  it("preserva a página quando uma busca vazia continua vazia", () => {
    const result = syncSearchParams(new URLSearchParams("page=3"), "");
    expect(result.get("page")).toBe("3");
  });

  it("volta à primeira página somente quando o texto muda", () => {
    const result = syncSearchParams(
      new URLSearchParams("page=3&search=erro"),
      "fatal",
    );
    expect(result.get("page")).toBe("1");
    expect(result.get("search")).toBe("fatal");
  });
});
