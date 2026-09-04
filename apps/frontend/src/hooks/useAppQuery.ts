import { useQuery, type QueryKey, type UseQueryOptions } from "@tanstack/react-query";
import { useEffect } from "react";
import { useCachedState } from "./useCachedState";

/**
 * Prefer this over manual useEffect + useCachedState for read models.
 * Keeps TanStack Query as the source of truth while preserving the existing
 * ["view", ...] query keys used by prefetch and cache hydration.
 */
export function useAppQuery<T>(
  queryKey: QueryKey,
  queryFn: () => Promise<T>,
  options?: Omit<UseQueryOptions<T, Error, T, QueryKey>, "queryKey" | "queryFn">,
) {
  const [cached, setCached] = useCachedState<T>(queryKey);
  const query = useQuery({
    queryKey,
    queryFn,
    initialData: cached,
    ...options,
  });

  useEffect(() => {
    if (query.data !== undefined) {
      setCached(query.data);
    }
  }, [query.data, setCached]);

  return query;
}
