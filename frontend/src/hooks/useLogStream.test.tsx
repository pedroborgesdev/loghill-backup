import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLogStream } from "./useLogStream";

class EventSourceMock {
  static instances: EventSourceMock[] = [];
  static readonly CLOSED = 2;
  readonly CLOSED = 2;
  readyState = 1;
  listeners = new Map<string, (event: MessageEvent<string>) => void>();
  onerror: (() => void) | null = null;
  constructor(public url: string) { EventSourceMock.instances.push(this); }
  addEventListener(name: string, listener: EventListenerOrEventListenerObject) {
    this.listeners.set(name, listener as (event: MessageEvent<string>) => void);
  }
  emit(name: string, data: object = {}) {
    this.listeners.get(name)?.({ data: JSON.stringify(data) } as MessageEvent<string>);
  }
  close() { this.readyState = EventSourceMock.CLOSED; }
}

describe("useLogStream", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    EventSourceMock.instances = [];
    vi.stubGlobal("EventSource", EventSourceMock);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("agrupa eventos antes de renderizar e preserva fila pausada", () => {
    const { result, rerender } = renderHook(
      ({ paused }) => useLogStream("worker-12345678", [], paused),
      { initialProps: { paused: false } },
    );
    const source = EventSourceMock.instances[0];
    act(() => {
      source.emit("status");
      source.emit("log", {
        timestamp: "2026-07-30T10:00:00Z",
        severity: "INFO",
        message: "teste 1",
      });
    });
    expect(result.current.entries).toHaveLength(0);
    act(() => vi.advanceTimersByTime(150));
    expect(result.current.entries).toHaveLength(1);

    rerender({ paused: true });
    act(() => {
      source.emit("log", {
        timestamp: "2026-07-30T10:00:01Z",
        severity: "ERROR",
        message: "teste 2",
      });
      vi.advanceTimersByTime(300);
    });
    expect(result.current.entries).toHaveLength(1);
    expect(result.current.pendingCount).toBe(1);

    rerender({ paused: false });
    act(() => vi.advanceTimersByTime(150));
    expect(result.current.entries).toHaveLength(2);
  });
});
