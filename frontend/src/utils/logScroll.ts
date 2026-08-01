export function shouldFreezeLogViewport({
  autoScroll,
  pinnedToLatest,
  userInteracting,
}: {
  autoScroll: boolean;
  pinnedToLatest: boolean;
  userInteracting: boolean;
}) {
  return userInteracting || !autoScroll || !pinnedToLatest;
}

const logScrollKeys = new Set([
  "ArrowDown",
  "ArrowUp",
  "End",
  "Home",
  "PageDown",
  "PageUp",
  " ",
]);

export function isLogScrollKey(key: string) {
  return logScrollKeys.has(key);
}
