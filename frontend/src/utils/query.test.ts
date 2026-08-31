import { describe, expect, it } from "vitest";
import { syncSearchParams } from "./query";

describe("syncSearchParams", () => {
  it("preserves the page when an empty search remains empty", () => {
    const result = syncSearchParams(new URLSearchParams("page=3"), "");
    expect(result.get("page")).toBe("3");
  });

  it("returns to the first page only when the text changes", () => {
    const result = syncSearchParams(
      new URLSearchParams("page=3&search=erro"),
      "fatal",
    );
    expect(result.get("page")).toBe("1");
    expect(result.get("search")).toBe("fatal");
  });
});
