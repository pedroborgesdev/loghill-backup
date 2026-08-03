import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { queryClient } from "../api/queryClient";
import { useCachedState } from "./useCachedState";

describe("useCachedState", () => {
  afterEach(() => queryClient.clear());

  it("restores data synchronously after a view is remounted", () => {
    const key = ["view", "test"] as const;
    const first = renderHook(() => useCachedState<{ value: string }>(key));

    act(() => first.result.current[1]({ value: "cached" }));
    first.unmount();

    const second = renderHook(() => useCachedState<{ value: string }>(key));
    expect(second.result.current[0]).toEqual({ value: "cached" });
  });
});
