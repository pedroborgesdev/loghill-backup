import { describe, expect, it } from "vitest";
import {
  calculateLivePagination,
  limitLiveEntries,
} from "./livePagination";

describe("paginacao dos logs ao vivo", () => {
  it("mantem 25 itens na primeira pagina quando chega o log 26", () => {
    const entries = Array.from({ length: 26 }, (_, index) => index + 1);
    const firstPage = limitLiveEntries(entries, 25);

    expect(firstPage).toHaveLength(25);
    expect(firstPage[0]).toBe(1);
    expect(firstPage).not.toContain(26);
  });

  it("move o excedente para uma nova pagina no total ao vivo", () => {
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
