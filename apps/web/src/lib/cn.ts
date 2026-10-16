/**
 * Minimal className combiner. Falsy values are dropped.
 *
 * Deliberately dependency-free: we control the component styles, so a full
 * tailwind-merge is unnecessary and would add bundle weight.
 */
export function cn(
  ...values: Array<string | number | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}
