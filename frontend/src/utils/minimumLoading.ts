export const MINIMUM_SKELETON_MS = 500;

export async function waitForMinimumLoading(
  startedAt: number,
  minimum = MINIMUM_SKELETON_MS,
) {
  const remaining = minimum - (performance.now() - startedAt);
  if (remaining <= 0) return;
  await new Promise<void>((resolve) => window.setTimeout(resolve, remaining));
}
