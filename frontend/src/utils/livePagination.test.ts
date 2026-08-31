import { describe, expect, it } from "vitest";
import {
  calculateLivePagination,
  limitLiveEntries,
} from "./livePagination";

describe("live log pagination", () => {
  it("keeps 25 items on the first page when log 26 arrives", () => {
    const entries = Array.from({ length: 26 }, (_, index) => index + 1);
    const firstPage = limitLiveEntries(entries, 25);

    expect(firstPage).toHaveLength(25);
    expect(firstPage[0]).toBe(1);
    expect(firstPage).not.toContain(26);
  });

  it("moves overflow to a new page in the live total", () => {
    expect(
      calculateLivePagination({
        baseTotal: 25,
        receivedCount: 1,
        receivedAtLastLoad: 0,
        pageSize: 25,
      }),
    ).toEqual({ total: 26, totalPages: 2 });
  });
});
