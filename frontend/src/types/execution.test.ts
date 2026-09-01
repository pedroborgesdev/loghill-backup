import { describe, expect, it } from "vitest";
import { isRecentExecution } from "./execution";

describe("isRecentExecution",()=>{const now=new Date("2026-08-02T15:00:00.000Z");it("accepts only valid timestamps within the last hour",()=>{expect(isRecentExecution("2026-08-02T14:30:00.000Z",now)).toBe(true);expect(isRecentExecution("2026-08-02T14:00:00.000Z",now)).toBe(false);expect(isRecentExecution("2026-08-02T15:01:00.000Z",now)).toBe(false);expect(isRecentExecution("invalid",now)).toBe(false)})});
