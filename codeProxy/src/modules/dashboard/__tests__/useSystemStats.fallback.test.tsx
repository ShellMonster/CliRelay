import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { SystemMonitorSection } from "@/modules/dashboard/SystemMonitorSection";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  useAuth: vi.fn(),
}));

vi.mock("@/lib/http/client", () => ({
  apiClient: {
    get: mocks.apiGet,
  },
}));

vi.mock("@/modules/auth/AuthProvider", () => ({
  useAuth: mocks.useAuth,
}));

class FailingWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  constructor(_url: string | URL) {
    throw new Error("websocket bootstrap failed");
  }
}

const recoveredStats = {
  db_size_bytes: 2048,
  log_size_bytes: 4096,
  process_mem_bytes: 1024 * 1024 * 128,
  process_mem_pct: 32.1,
  process_cpu_pct: 12.3,
  go_routines: 17,
  go_heap_bytes: 1024 * 1024 * 32,
  system_cpu_pct: 26.4,
  system_mem_total: 1024 * 1024 * 1024,
  system_mem_used: 1024 * 1024 * 512,
  system_mem_pct: 50.0,
  net_bytes_sent: 2048,
  net_bytes_recv: 4096,
  net_send_rate: 128,
  net_recv_rate: 256,
  uptime_seconds: 65,
  start_time: "2026-04-10T03:20:00.000Z",
  channel_latency: [],
};

describe("useSystemStats fallback failure", () => {
  const originalWebSocket = globalThis.WebSocket;

  beforeEach(() => {
    mocks.useAuth.mockReturnValue({
      state: {
        apiBase: "http://localhost:8317",
        managementKey: "test-management-key",
      },
    });
    globalThis.WebSocket = FailingWebSocket as unknown as typeof WebSocket;
  });

  afterEach(() => {
    vi.useRealTimers();
    mocks.apiGet.mockReset();
    mocks.useAuth.mockReset();
    globalThis.WebSocket = originalWebSocket;
  });

  test("should leave loading and surface a terminal failure when websocket bootstrap and HTTP fallback both fail", async () => {
    mocks.apiGet.mockRejectedValue(new Error("http fallback failed"));

    render(<SystemMonitorSection />);

    expect(screen.getByText("连接中…")).toBeInTheDocument();

    await waitFor(() => {
      expect(mocks.apiGet).toHaveBeenCalledWith("/system-stats");
    });

    expect(screen.queryByText("连接中…")).not.toBeInTheDocument();
  });

  test("should clear the error and render latest stats after a later HTTP fallback recovery", async () => {
    let scheduledPoll: (() => void) | undefined;
    // Fragile: depends on setInterval — will break if implementation switches to setTimeout recursion
    const intervalSpy = vi.spyOn(globalThis, "setInterval" as any).mockImplementationOnce(((fn: () => void) => {
      scheduledPoll = fn;
      return 1;
    }) as any);

    mocks.apiGet.mockRejectedValueOnce(new Error("http fallback failed")).mockResolvedValueOnce(recoveredStats);

    render(<SystemMonitorSection />);

    await waitFor(() => {
      expect(screen.getByText("系统监控加载失败，请稍后重试")).toBeInTheDocument();
    });

    await act(async () => {
      scheduledPoll?.();
    });

    expect(scheduledPoll).toBeTypeOf("function");

    await waitFor(() => {
      expect(screen.getByText("系统 CPU")).toBeInTheDocument();
      expect(screen.getByText("26.4%")).toBeInTheDocument();
    });

    expect(mocks.apiGet).toHaveBeenCalledTimes(2);
    expect(screen.queryByText("系统监控加载失败，请稍后重试")).not.toBeInTheDocument();
    expect(screen.queryByText("加载失败")).not.toBeInTheDocument();

    intervalSpy.mockRestore();
  });
});
