import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, test, vi } from "vitest";
import { DashboardPage } from "@/modules/dashboard/DashboardPage";

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

const createDeferred = <T,>(): Deferred<T> => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;

  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });

  return { promise, resolve, reject };
};

const mocks = vi.hoisted(() => ({
  getDashboardSummary: vi.fn(),
  notify: vi.fn(),
}));

vi.mock("@/lib/http/apis/usage", () => ({
  usageApi: {
    getDashboardSummary: mocks.getDashboardSummary,
  },
}));

vi.mock("@/modules/dashboard/SystemMonitorSection", () => ({
  SystemMonitorSection: () => <div data-testid="system-monitor-section" />,
}));

vi.mock("@/modules/ui/ToastProvider", () => ({
  useToast: () => ({ notify: mocks.notify }),
}));

const makeSummary = (days: number, totalRequests: number) => ({
  days,
  kpi: {
    total_requests: totalRequests,
    success_requests: totalRequests - 1,
    failed_requests: 1,
    success_rate: 99.5,
    input_tokens: 10,
    output_tokens: 20,
    reasoning_tokens: 0,
    cached_tokens: 0,
    total_tokens: 30,
    processed_tokens: 30,
  },
  counts: {
    api_keys: 1,
    providers_total: 1,
    gemini_keys: 0,
    claude_keys: 0,
    codex_keys: 0,
    vertex_keys: 0,
    openai_providers: 0,
    auth_files: 0,
  },
});

describe("DashboardPage stale response race", () => {
  test("keeps the newest days selection when an older request resolves later", async () => {
    const user = userEvent.setup();
    const pending = new Map<number, Deferred<ReturnType<typeof makeSummary>>>();

    mocks.getDashboardSummary.mockImplementation((days: number) => {
      const deferred = createDeferred<ReturnType<typeof makeSummary>>();
      pending.set(days, deferred);
      return deferred.promise;
    });

    render(<DashboardPage />);

    expect(mocks.getDashboardSummary).toHaveBeenCalledWith(7);

    await user.click(screen.getByRole("button", { name: "近 30 天" }));

    expect(mocks.getDashboardSummary).toHaveBeenCalledWith(30);
    expect(mocks.getDashboardSummary).toHaveBeenCalledTimes(2);

    await act(async () => {
      pending.get(30)?.resolve(makeSummary(30, 303));
      await Promise.resolve();
    });

    const requestsCard = screen.getByText("请求数").closest("article");
    expect(requestsCard).not.toBeNull();
    expect(within(requestsCard as HTMLElement).getByText("303")).toBeInTheDocument();
    expect(requestsCard).toHaveTextContent("最近 30 天的总请求数");

    await act(async () => {
      pending.get(7)?.resolve(makeSummary(7, 707));
      await Promise.resolve();
    });

    expect(requestsCard).toHaveTextContent("最近 30 天的总请求数");
    expect(requestsCard).toHaveTextContent("303");
  });
});
