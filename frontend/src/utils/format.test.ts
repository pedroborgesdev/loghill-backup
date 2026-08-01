import { describe,expect,it } from "vitest";
import { formatBytes,formatNumber } from "./format";
describe("formatters",()=>{it("formata bytes",()=>expect(formatBytes(2048)).toBe("2.0 KB"));it("formata números",()=>expect(formatNumber(1000)).toMatch(/1.*000/))});
