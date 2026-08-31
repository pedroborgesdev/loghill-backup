import { describe, expect, it } from "vitest";
import { isLogScrollKey, shouldFreezeLogViewport } from "./logScroll";

describe("shouldFreezeLogViewport", () => {
  it("keeps the list frozen during any user interaction", () => {
    expect(
      shouldFreezeLogViewport({
        autoScroll: true,
        pinnedToLatest: true,
        userInteracting: true,
      }),
    ).toBe(true);
  });

  it("applies immediately only when anchored and idle", () => {
    expect(
      shouldFreezeLogViewport({
        autoScroll: true,
        pinnedToLatest: true,
        userInteracting: false,
      }),
    ).toBe(false);
  });

  it("freezes the viewport when Follow is off", () => {
    expect(
      shouldFreezeLogViewport({
        autoScroll: false,
        pinnedToLatest: true,
        userInteracting: false,
      }),
    ).toBe(true);
  });

  it("recognizes only keys that move the log viewport", () => {
    expect(isLogScrollKey("PageDown")).toBe(true);
    expect(isLogScrollKey("ArrowUp")).toBe(true);
    expect(isLogScrollKey("Enter")).toBe(false);
  });
});
