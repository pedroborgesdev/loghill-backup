export const tooltipOpenEvent = "logmate:tooltip-open";
export const tooltipDismissDetail = "__dismiss_tooltips__";

export function restoreFocusWithoutTooltip(element?: HTMLElement | null) {
  if (!element?.isConnected) return;
  window.dispatchEvent(new CustomEvent(tooltipOpenEvent, { detail: tooltipDismissDetail }));
  element.focus();
}
