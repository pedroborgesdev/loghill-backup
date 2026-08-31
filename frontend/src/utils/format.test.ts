import { describe,expect,it } from "vitest";
import { formatBytes,formatNumber } from "./format";
describe("formatters",()=>{it("formats bytes",()=>expect(formatBytes(2048)).toBe("2.0 KB"));it("formats numbers",()=>expect(formatNumber(1000)).toMatch(/1.*000/))});
