import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LogViewer } from "./LogViewer";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("LogViewer Follow", () => {
  it("desliga no scroll e so volta a seguir com um novo clique", () => {
    const onAutoScrollChange = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: vi.fn(),
    });
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });

    const view = render(
      <LogViewer
        entries={[
          {
            ui_id: "log-1",
            timestamp: "2026-07-30T16:00:00Z",
            severity: "INFO",
            message: "teste",
          },
        ]}
        density="compact"
        streamState="connected"
        liveCount={1}
        autoScroll
        onAutoScrollChange={onAutoScrollChange}
      />,
    );

    fireEvent.wheel(screen.getByRole("log", { name: "Logs" }));
    expect(onAutoScrollChange).toHaveBeenLastCalledWith(false);

    view.rerender(
      <LogViewer
        entries={[
          {
            ui_id: "log-1",
            timestamp: "2026-07-30T16:00:00Z",
            severity: "INFO",
            message: "teste",
          },
        ]}
        density="compact"
        streamState="connected"
        liveCount={1}
        autoScroll={false}
        onAutoScrollChange={onAutoScrollChange}
      />,
    );

    fireEvent.scroll(screen.getByRole("log", { name: "Logs" }), {
      target: { scrollTop: 0 },
    });
    expect(onAutoScrollChange).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Follow" }));
    expect(onAutoScrollChange).toHaveBeenLastCalledWith(true);
  });

  it("insere novos logs sem acionar o reposicionamento do Follow", () => {
    const scrollTo = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: scrollTo,
    });
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });

    const oldEntry = {
      ui_id: "log-1",
      timestamp: "2026-07-30T16:00:00Z",
      severity: "INFO" as const,
      message: "log anterior",
    };
    const view = render(
      <LogViewer
        entries={[oldEntry]}
        density="compact"
        streamState="connected"
        liveCount={1}
        autoScroll={false}
        onAutoScrollChange={vi.fn()}
      />,
    );
    scrollTo.mockClear();

    view.rerender(
      <LogViewer
        entries={[
          {
            ui_id: "log-2",
            timestamp: "2026-07-30T16:00:01Z",
            severity: "INFO",
            message: "log novo",
          },
          oldEntry,
        ]}
        density="compact"
        streamState="connected"
        liveCount={2}
        autoScroll={false}
        onAutoScrollChange={vi.fn()}
      />,
    );

    expect(screen.getByText("log novo")).toBeInTheDocument();
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("preserva exatamente o log visivel quando outro entra no topo", () => {
    const offsetTop = vi
      .spyOn(HTMLElement.prototype, "offsetTop", "get")
      .mockImplementation(function (this: HTMLElement) {
        if (!this.dataset.logKey || !this.parentElement) return 0;
        return Array.from(this.parentElement.children).indexOf(this) * 29;
      });
    const offsetHeight = vi
      .spyOn(HTMLElement.prototype, "offsetHeight", "get")
      .mockReturnValue(29);

    const olderEntry = {
      ui_id: "log-older",
      timestamp: "2026-07-30T15:59:59Z",
      severity: "INFO" as const,
      message: "log que deve permanecer visivel",
    };
    const currentEntry = {
      ui_id: "log-current",
      timestamp: "2026-07-30T16:00:00Z",
      severity: "INFO" as const,
      message: "log atual",
    };
    const view = render(
      <LogViewer
        entries={[currentEntry, olderEntry]}
        density="compact"
        streamState="connected"
        liveCount={2}
        autoScroll={false}
        onAutoScrollChange={vi.fn()}
      />,
    );
    const logWindow = screen.getByRole("log", { name: "Logs" });
    logWindow.scrollTop = 29;
    fireEvent.scroll(logWindow);

    view.rerender(
      <LogViewer
        entries={[
          {
            ui_id: "log-new",
            timestamp: "2026-07-30T16:00:01Z",
            severity: "INFO",
            message: "novo log no topo",
          },
          currentEntry,
          olderEntry,
        ]}
        density="compact"
        streamState="connected"
        liveCount={3}
        autoScroll={false}
        onAutoScrollChange={vi.fn()}
      />,
    );

    expect(logWindow.scrollTop).toBe(58);
    offsetTop.mockRestore();
    offsetHeight.mockRestore();
  });
});
