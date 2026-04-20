import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { MonitorPage } from "@/modules/monitor/MonitorPage";

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
  getMonitorFilters: vi.fn(),
  getMonitorSummary: vi.fn(),
  getMonitorModelDistribution: vi.fn(),
  getMonitorDailyTrend: vi.fn(),
  getMonitorHourly: vi.fn(),
  notify: vi.fn(),
}));

vi.mock("@/lib/http/apis/usage", () => ({
  usageApi: {
    getMonitorFilters: mocks.getMonitorFilters,
    getMonitorSummary: mocks.getMonitorSummary,
    getMonitorModelDistribution: mocks.getMonitorModelDistribution,
    getMonitorDailyTrend: mocks.getMonitorDailyTrend,
    getMonitorHourly: mocks.getMonitorHourly,
  },
}));

vi.mock("@/modules/ui/ToastProvider", () => ({
  useToast: () => ({ notify: mocks.notify }),
}));

vi.mock("@/modules/ui/ThemeProvider", () => ({
  useTheme: () => ({ state: { mode: "light" } }),
}));

vi.mock("@/modules/ui/AnimatedNumber", () => ({
  AnimatedNumber: ({ value }: { value: number }) => <span>{String(value)}</span>,
}));

vi.mock("@/modules/ui/Reveal", () => ({
  Reveal: ({ children }: { children: import("react").ReactNode }) => <>{children}</>,
}));

vi.mock("@/modules/ui/SearchableSelect", () => ({
  SearchableSelect: ({ "aria-label": ariaLabel }: { "aria-label": string }) => (
    <div aria-label={ariaLabel} />
  ),
}));

vi.mock("@/modules/ui/MultiSelect", () => ({
  MultiSelect: () => <div aria-label="按渠道过滤" />,
}));

vi.mock("@/modules/ui/charts/EChart", () => ({
  EChart: () => <div data-testid="echart" />,
}));

vi.mock("@/modules/ui/charts/ChartLegend", () => ({
  ChartLegend: ({ items }: { items: Array<{ label: string }> }) => (
    <div>{items.map((item) => item.label).join("|")}</div>
  ),
}));

vi.mock("@/modules/monitor/monitor-chart-options", () => ({
  createDailyTrendOption: () => ({}),
  createHourlyModelOption: () => ({}),
  createHourlyTokenOption: () => ({}),
  createModelDistributionOption: () => ({}),
}));

vi.mock("@/modules/monitor/MonitorPagePieces", () => ({
  TimeRangeSelector: ({ onChange }: { onChange: (next: 7 | 30) => void }) => (
    <div>
      <button type="button" onClick={() => onChange(7)}>
        7 天
      </button>
      <button type="button" onClick={() => onChange(30)}>
        30 天
      </button>
    </div>
  ),
  HourWindowSelector: () => <div />,
  KpiCard: ({ title, value }: { title: string; value: import("react").ReactNode }) => (
    <article>
      <span>{title}</span>
      <div>{value}</div>
    </article>
  ),
  MonitorCard: ({ title, children, loading = false }: { title: string; children: import("react").ReactNode; loading?: boolean }) => (
    <section aria-busy={loading}>
      <h3>{title}</h3>
      {children}
    </section>
  ),
}));

const filtersResponse = {
  days: 7,
  filters: {
    api_keys: [],
    api_key_names: {},
    models: [],
    channels: [],
    channel_options: [],
  },
};

const makeSummary = (days: number) => ({
  days,
  summary: {
    TotalRequests: days * 10,
    SuccessRequests: days * 10,
    FailedRequests: 0,
    SuccessRate: 100,
    ProcessedTokens: days * 100,
    TotalTokens: days * 100,
    CachedTokens: 0,
    InputTokens: days * 40,
    OutputTokens: days * 60,
  },
});

const makeDistribution = (model: string) => ({
  days: 7,
  items: [
    {
      model,
      requests: 100,
      tokens: 1000,
      total_tokens: 1000,
      cached_tokens: 0,
      processed_tokens: 1000,
    },
  ],
});

const makeDaily = (label: string) => ({
  days: 7,
  items: [
    {
      day: label,
      requests: 10,
      input_tokens: 100,
      output_tokens: 50,
      reasoning_tokens: 0,
      cached_tokens: 0,
      total_tokens: 150,
      processed_tokens: 150,
    },
  ],
});

const makeHourly = (model: string) => ({
  hours: 24,
  items: [
    {
      hour: "2026-04-10T10:00:00.000Z",
      model,
      requests: 10,
      input_tokens: 100,
      output_tokens: 50,
      reasoning_tokens: 0,
      cached_tokens: 0,
      total_tokens: 150,
      processed_tokens: 150,
    },
  ],
});

describe("MonitorPage stale chart response race", () => {
  afterEach(() => {
    vi.useRealTimers();
    mocks.getMonitorFilters.mockReset();
    mocks.getMonitorSummary.mockReset();
    mocks.getMonitorModelDistribution.mockReset();
    mocks.getMonitorDailyTrend.mockReset();
    mocks.getMonitorHourly.mockReset();
    mocks.notify.mockReset();
  });

  test("ignores older chart responses after a newer time range refresh starts", async () => {
    vi.useFakeTimers();

    const summaryPending = new Map<number, Deferred<ReturnType<typeof makeSummary>>>();
    const distributionPending = new Map<number, Deferred<ReturnType<typeof makeDistribution>>>();
    const dailyPending = new Map<number, Deferred<ReturnType<typeof makeDaily>>>();
    const hourlyPending = new Map<number, Deferred<ReturnType<typeof makeHourly>>>();

    mocks.getMonitorFilters.mockResolvedValue(filtersResponse);
    mocks.getMonitorSummary.mockImplementation((days: number) => {
      const deferred = createDeferred<ReturnType<typeof makeSummary>>();
      summaryPending.set(days, deferred);
      return deferred.promise;
    });
    mocks.getMonitorModelDistribution.mockImplementation((days: number) => {
      const deferred = createDeferred<ReturnType<typeof makeDistribution>>();
      distributionPending.set(days, deferred);
      return deferred.promise;
    });
    mocks.getMonitorDailyTrend.mockImplementation((days: number) => {
      const deferred = createDeferred<ReturnType<typeof makeDaily>>();
      dailyPending.set(days, deferred);
      return deferred.promise;
    });
    mocks.getMonitorHourly.mockImplementation(() => {
      const deferred = createDeferred<ReturnType<typeof makeHourly>>();
      hourlyPending.set(summaryPending.size === 1 ? 7 : 30, deferred);
      return deferred.promise;
    });

    render(<MonitorPage />);

    expect(mocks.getMonitorSummary).toHaveBeenCalledWith(7, "", "", []);

    await act(async () => {
      summaryPending.get(7)?.resolve(makeSummary(7));
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(mocks.getMonitorModelDistribution).toHaveBeenCalledWith(7, 10, "", "", []);
    expect(mocks.getMonitorDailyTrend).toHaveBeenCalledWith(7, "", "", []);
    expect(mocks.getMonitorHourly).toHaveBeenCalledWith(24, "", "", []);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "30 天" }));
      await Promise.resolve();
    });

    expect(mocks.getMonitorSummary).toHaveBeenCalledWith(30, "", "", []);

    await act(async () => {
      summaryPending.get(30)?.resolve(makeSummary(30));
      await Promise.resolve();
    });

    await act(async () => {
      distributionPending.get(7)?.resolve(makeDistribution("stale-model"));
      dailyPending.get(7)?.resolve(makeDaily("2026-04-09"));
      hourlyPending.get(7)?.resolve(makeHourly("stale-model"));
      await Promise.resolve();
    });

    const distributionCard = screen.getByRole("heading", { name: "模型用量分布" }).closest("section");
    expect(distributionCard).not.toBeNull();
    expect(within(distributionCard as HTMLElement).queryByText("stale-model")).not.toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    await act(async () => {
      distributionPending.get(30)?.resolve(makeDistribution("fresh-model"));
      dailyPending.get(30)?.resolve(makeDaily("2026-04-10"));
      hourlyPending.get(30)?.resolve(makeHourly("fresh-model"));
      await Promise.resolve();
    });

    expect(within(distributionCard as HTMLElement).getByText("fresh-model")).toBeInTheDocument();
    expect(within(distributionCard as HTMLElement).queryByText("stale-model")).not.toBeInTheDocument();
  });
});
