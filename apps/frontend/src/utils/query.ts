export function syncSearchParams(
  current: URLSearchParams,
  search: string,
): URLSearchParams {
  const next = new URLSearchParams(current);
  const previousSearch = current.get("search") ?? "";
  if (search) next.set("search", search);
  else next.delete("search");
  if (previousSearch !== search) next.set("page", "1");
  return next;
}
