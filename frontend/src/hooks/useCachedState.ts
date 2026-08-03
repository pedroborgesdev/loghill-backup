import { useCallback, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { queryClient } from "../api/queryClient";

export function useCachedState<T>(
  queryKey: readonly unknown[],
  initialValue: T,
): [T, Dispatch<SetStateAction<T>>];
export function useCachedState<T>(
  queryKey: readonly unknown[],
): [T | undefined, Dispatch<SetStateAction<T | undefined>>];
export function useCachedState<T>(
  queryKey: readonly unknown[],
  initialValue?: T,
): [T | undefined, Dispatch<SetStateAction<T | undefined>>] {
  const stableKey = useRef(queryKey).current;
  const [value, setLocalValue] = useState<T | undefined>(
    () => queryClient.getQueryData<T>(stableKey) ?? initialValue,
  );

  const setValue = useCallback<Dispatch<SetStateAction<T | undefined>>>(
    (nextValue) => {
      setLocalValue((current) => {
        const next = typeof nextValue === "function"
          ? (nextValue as (previous: T | undefined) => T | undefined)(current)
          : nextValue;
        queryClient.setQueryData(stableKey, next);
        return next;
      });
    },
    [stableKey],
  );

  return [value, setValue];
}
