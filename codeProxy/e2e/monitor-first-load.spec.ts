import { expect, test, type Page, type Route } from "@playwright/test";

const EVIDENCE_PATH = new URL(
  "../../.sisyphus/evidence/task-1-monitor-baseline.json",
  import.meta.url,
);

const KPI_VISIBLE_THRESHOLD_MS = Number(process.env.KPI_THRESHOLD_MS ?? 2000);
const CHARTS_INTERACTIVE_THRESHOLD_MS = Number(process.env.CHARTS_THRESHOLD_MS ?? 4000);

const writeEvidence = async (fileUrl: URL, content: string) => {
  // @ts-expect-error Playwright test runs on Node, but this workspace does not expose Node types to e2e files.
  const { writeFile } = await import("node:fs/promises");
  await writeFile(fileUrl, content, "utf8");
};

type MonitorRequestKey = "filters" | "summary" | "distribution" | "daily" | "hourly";

const setAuthed = async (page: Page) => {
  await page.addInitScript(() => {
    localStorage.setItem(
      "code-proxy-admin-auth",
      JSON.stringify({
        apiBase: "http://127.0.0.1:8317",
        managementKey: "test-management-key",
        rememberPassword: true,
      }),
    );
  });
};

const fulfillJson = async (route: Route, body: unknown) => {
  await route.fulfill({
    status: 200,
    contentType: "application/json; charset=utf-8",
    body: JSON.stringify(body),
  });
};

const formatDay = (date: Date) => date.toISOString().slice(0, 10);

const formatHour = (date: Date) => {
  const rounded = new Date(date);
  rounded.setMinutes(0, 0, 0);
  return rounded.toISOString();
};

const buildFixtures = () => {
  const now = new Date();
  const dailyItems = Array.from({ length: 7 }, (_, index) => {
    const date = new Date(now);
    date.setDate(date.getDate() - (6 - index));
    return {
      day: formatDay(date),
      requests: 120 + index * 9,
      input_tokens: 10_000 + index * 400,
      output_tokens: 4_500 + index * 220,
      reasoning_tokens: 1_200 + index * 50,
      cached_tokens: 850 + index * 40,
      total_tokens: 16_550 + index * 710,
      processed_tokens: 15_700 + index * 670,
    };
  });

  const hourlyItems = Array.from({ length: 24 }, (_, index) => {
    const date = new Date(now);
    date.setHours(date.getHours() - (23 - index), 0, 0, 0);
    return [
      {
        hour: formatHour(date),
        model: "claude-3-7-sonnet",
        requests: 18 + (index % 4),
        input_tokens: 1_600 + index * 15,
        output_tokens: 620 + index * 9,
        reasoning_tokens: 140 + index * 4,
        cached_tokens: 95 + index,
        total_tokens: 2_455 + index * 29,
        processed_tokens: 2_360 + index * 28,
      },
      {
        hour: formatHour(date),
        model: "gpt-4.1-mini",
        requests: 11 + (index % 3),
        input_tokens: 980 + index * 10,
        output_tokens: 410 + index * 6,
        reasoning_tokens: 70 + index * 2,
        cached_tokens: 48 + index,
        total_tokens: 1_508 + index * 19,
        processed_tokens: 1_460 + index * 18,
      },
    ];
  }).flat();

  return {
    filters: {
      days: 7,
      filters: {
        api_keys: ["key-alpha", "key-beta"],
        api_key_names: {
          "key-alpha": "Alpha Key",
          "key-beta": "Beta Key",
        },
        models: ["claude-3-7-sonnet", "gpt-4.1-mini"],
        channels: ["channel-a", "channel-b"],
        channel_options: [
          { value: "channel-a", label: "主渠道 A" },
          { value: "channel-b", label: "备用渠道 B" },
        ],
      },
    },
    summary: {
      days: 7,
      summary: {
        TotalRequests: 1_876,
        SuccessRequests: 1_741,
        FailedRequests: 135,
        SuccessRate: 92.8,
        InputTokens: 151_200,
        OutputTokens: 61_400,
        ReasoningTokens: 18_900,
        CachedTokens: 12_600,
        TotalTokens: 244_100,
        ProcessedTokens: 231_500,
      },
    },
    distribution: {
      days: 7,
      items: [
        {
          model: "claude-3-7-sonnet",
          requests: 990,
          tokens: 125_000,
          total_tokens: 125_000,
          cached_tokens: 6_300,
          processed_tokens: 118_700,
        },
        {
          model: "gpt-4.1-mini",
          requests: 610,
          tokens: 84_000,
          total_tokens: 84_000,
          cached_tokens: 4_100,
          processed_tokens: 79_900,
        },
        {
          model: "gemini-2.5-flash",
          requests: 276,
          tokens: 35_100,
          total_tokens: 35_100,
          cached_tokens: 2_200,
          processed_tokens: 32_900,
        },
      ],
    },
    daily: {
      days: 7,
      items: dailyItems,
    },
    hourly: {
      hours: 24,
      items: hourlyItems,
    },
  };
};

test("Monitor: first load uses staged loading with deferred charts", async ({
  page,
}) => {
  await setAuthed(page);

  const fixtures = buildFixtures();
  const chartResponseDelayMs = 500;
  const requestCounts: Record<MonitorRequestKey, number> = {
    filters: 0,
    summary: 0,
    distribution: 0,
    daily: 0,
    hourly: 0,
  };
  const requestUrls: Record<MonitorRequestKey, string[]> = {
    filters: [],
    summary: [],
    distribution: [],
    daily: [],
    hourly: [],
  };

  const recordRequest = (key: MonitorRequestKey, route: Route) => {
    requestCounts[key] += 1;
    const url = new URL(route.request().url());
    requestUrls[key].push(`${url.pathname}${url.search}`);
    return url;
  };

  await page.route("**/v0/management/config", async (route) => {
    await fulfillJson(route, {});
  });

  await page.route("**/v0/management/dashboard-summary**", async (route) => {
    await fulfillJson(route, {
      kpi: {
        total_requests: 320,
        success_requests: 300,
        failed_requests: 20,
        success_rate: 93.75,
        total_tokens: 80_000,
        processed_tokens: 76_000,
        cached_tokens: 4_000,
      },
    });
  });

  await page.route("**/v0/management/system-stats**", async (route) => {
    await fulfillJson(route, {
      db_size_bytes: 1024,
      log_size_bytes: 2048,
      process_mem_bytes: 1024 * 1024,
      process_mem_pct: 12,
      process_cpu_pct: 8,
      go_routines: 12,
      go_heap_bytes: 1024 * 1024,
      system_cpu_pct: 18,
      system_mem_total: 16 * 1024 * 1024 * 1024,
      system_mem_used: 8 * 1024 * 1024 * 1024,
      system_mem_pct: 50,
      net_bytes_sent: 512,
      net_bytes_recv: 1024,
      net_send_rate: 32,
      net_recv_rate: 64,
      uptime_seconds: 3600,
      start_time: new Date().toISOString(),
      channel_latency: [],
    });
  });

  await page.route("**/v0/management/monitor/filters**", async (route) => {
    const url = recordRequest("filters", route);
    expect(url.searchParams.get("days")).toBe("7");
    await fulfillJson(route, fixtures.filters);
  });

  await page.route("**/v0/management/monitor/summary**", async (route) => {
    const url = recordRequest("summary", route);
    expect(url.searchParams.get("days")).toBe("7");
    await fulfillJson(route, fixtures.summary);
  });

  await page.route("**/v0/management/monitor/model-distribution**", async (route) => {
    const url = recordRequest("distribution", route);
    expect(url.searchParams.get("days")).toBe("7");
    expect(url.searchParams.get("limit")).toBe("10");
    await new Promise((resolve) => setTimeout(resolve, chartResponseDelayMs));
    await fulfillJson(route, fixtures.distribution);
  });

  await page.route("**/v0/management/monitor/daily-trend**", async (route) => {
    const url = recordRequest("daily", route);
    expect(url.searchParams.get("days")).toBe("7");
    await new Promise((resolve) => setTimeout(resolve, chartResponseDelayMs));
    await fulfillJson(route, fixtures.daily);
  });

  await page.route("**/v0/management/monitor/hourly**", async (route) => {
    const url = recordRequest("hourly", route);
    expect(url.searchParams.get("hours")).toBe("24");
    await new Promise((resolve) => setTimeout(resolve, chartResponseDelayMs));
    await fulfillJson(route, fixtures.hourly);
  });

  await page.goto("/#/dashboard");
  await expect(page.getByRole("link", { name: "监控中心" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "仪表盘" }).first()).toBeVisible();

  const monitorNavLink = page.getByRole("link", { name: "监控中心" });
  const monitorNavigationStartedMs = await page.evaluate(() => Math.round(performance.now()));
  await Promise.all([page.waitForURL(/#\/monitor$/), monitorNavLink.click()]);

  const monitorToolbar = page
    .locator("section")
    .filter({ has: page.getByRole("button", { name: "应用过滤" }) })
    .first();
  await expect(monitorToolbar.getByText("监控中心", { exact: true })).toBeVisible();
  await expect(page.getByLabel("按 Key 名称过滤")).toBeVisible();
  await expect(page.locator("article").filter({ hasText: "总请求" }).first()).toBeVisible();
  const kpiAndFilterVisibleMs = await page.evaluate(
    (navigationStartedMs) => Math.round(performance.now()) - navigationStartedMs,
    monitorNavigationStartedMs,
  );

  expect(requestCounts.filters).toBe(1);
  expect(requestCounts.summary).toBe(1);
  expect(requestCounts.distribution).toBeLessThanOrEqual(1);
  expect(requestCounts.daily).toBeLessThanOrEqual(1);
  expect(requestCounts.hourly).toBeLessThanOrEqual(1);

  const busyChartSections = page.locator('section[aria-busy="true"]');
  await expect(busyChartSections).toHaveCount(4);

  for (const title of [
    "模型用量分布",
    "每日用量趋势",
    "每小时模型请求分布",
    "每小时 Token 用量",
  ]) {
    await expect(page.getByRole("heading", { name: title })).toBeVisible();
  }

  await expect
    .poll(async () => {
      return await page.locator('section[aria-busy="true"]').count();
    })
    .toBe(0);
  await expect
    .poll(async () => {
      return await page.locator("canvas").count();
    })
    .toBeGreaterThanOrEqual(4);
  const allChartsInteractiveMs = await page.evaluate(
    (navigationStartedMs) => Math.round(performance.now()) - navigationStartedMs,
    monitorNavigationStartedMs,
  );

  expect(requestCounts).toEqual({
    filters: 1,
    summary: 1,
    distribution: 1,
    daily: 1,
    hourly: 1,
  });

  const totalMonitorRequests = Object.values(requestCounts).reduce((sum, count) => sum + count, 0);
  expect(totalMonitorRequests).toBe(5);
  expect(allChartsInteractiveMs).toBeGreaterThan(kpiAndFilterVisibleMs);
  expect(kpiAndFilterVisibleMs).toBeLessThanOrEqual(KPI_VISIBLE_THRESHOLD_MS);
  expect(allChartsInteractiveMs).toBeLessThanOrEqual(CHARTS_INTERACTIVE_THRESHOLD_MS);

  const evidence = {
    scenario: "monitor-first-load-optimized",
    route: "/#/monitor",
    measurement: {
      method: "navigate-to-monitor-from-already-loaded-admin-shell",
      shell_route: "/#/dashboard",
      start_event: "sidebar click on 监控中心",
      thresholds_ms: {
        kpi_and_filter_visible: KPI_VISIBLE_THRESHOLD_MS,
        all_charts_interactive: CHARTS_INTERACTIVE_THRESHOLD_MS,
      },
    },
    request_count: {
      total: totalMonitorRequests,
      by_endpoint: requestCounts,
    },
    request_urls: requestUrls,
    response_delay_ms: {
      deferred_charts: chartResponseDelayMs,
    },
    milestones_ms: {
      kpi_and_filter_visible: kpiAndFilterVisibleMs,
      all_charts_interactive: allChartsInteractiveMs,
    },
  };

  await writeEvidence(EVIDENCE_PATH, `${JSON.stringify(evidence, null, 2)}\n`);
});
