import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 20_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

export async function cachedQuery<T>(
  queryKey: readonly unknown[],
  queryFn: () => Promise<T>,
  force = false,
) {
  if (force) {
    await queryClient.invalidateQueries({ queryKey, exact: true });
  }
  return queryClient.fetchQuery({ queryKey, queryFn });
}
