export function limitLiveEntries<T>(entries: T[], pageSize: number): T[] {
  return entries.slice(0, Math.max(1, pageSize));
}

export function calculateLivePagination({
  baseTotal,
  receivedCount,
  receivedAtLastLoad,
  pageSize,
}: {
  baseTotal: number;
  receivedCount: number;
  receivedAtLastLoad: number;
  pageSize: number;
}) {
  const liveDelta = Math.max(0, receivedCount - receivedAtLastLoad);
  const total = Math.max(0, baseTotal) + liveDelta;
  return {
    total,
    totalPages: Math.max(1, Math.ceil(total / Math.max(1, pageSize))),
  };
}
