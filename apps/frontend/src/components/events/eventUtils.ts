export const eventKeyPattern = /^[a-z0-9][a-z0-9_-]{2,79}$/;

export function isValidEventKey(value: string) {
  return eventKeyPattern.test(value);
}
