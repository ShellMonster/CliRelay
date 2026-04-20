import { expect, test, type Page, type Route } from "@playwright/test";

const setAuthedWithFailingSystemMonitorWebSocket = async (page: Page) => {
  await page.addInitScript(() => {
    localStorage.setItem(
      "code-proxy-admin-auth",
      JSON.stringify({
        apiBase: "http://127.0.0.1:8317",
        managementKey: "test-management-key",
        rememberPassword: true,
      }),
    );

    const NativeWebSocket = window.WebSocket;

    class FailingSystemMonitorWebSocket extends NativeWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        if (String(url).includes("/system-stats/ws")) {
          throw new Error("Simulated system monitor websocket bootstrap failure");
        }
        super(url, protocols);
      }
    }

    Object.defineProperty(window, "WebSocket", {
      configurable: true,
      writable: true,
      value: FailingSystemMonitorWebSocket,
    });
  });
};

const fulfillJson = async (route: Route, body: unknown) => {
  await route.fulfill({
    status: 200,
    contentType: "application/json; charset=utf-8",
    body: JSON.stringify(body),
  });
};

test("Dashboard: system monitor should leave loading and surface explicit failure after websocket and HTTP fallback both fail", async ({
  page,
}) => {
  await setAuthedWithFailingSystemMonitorWebSocket(page);

  let systemStatsFallbackCalls = 0;

  await page.route("**/v0/management/config", async (route) => {
    await fulfillJson(route, {});
  });

  await page.route("**/v0/management/dashboard-summary**", async (route) => {
    await fulfillJson(route, {
      kpi: {
        total_requests: 1876,
        success_rate: 92.8,
        success_requests: 1741,
        failed_requests: 135,
        processed_tokens: 231500,
        total_tokens: 244100,
        cached_tokens: 12600,
      },
    });
  });

  await page.route("**/v0/management/system-stats", async (route) => {
    systemStatsFallbackCalls += 1;
    await route.fulfill({
      status: 500,
      contentType: "application/json; charset=utf-8",
      body: JSON.stringify({ error: "simulated HTTP fallback failure" }),
    });
  });

  await page.goto("/#/dashboard");

  const monitorSection = page
    .locator("section")
    .filter({ has: page.getByText("运维监控", { exact: true }) })
    .first();

  await expect(monitorSection).toBeVisible();

  await expect
    .poll(() => systemStatsFallbackCalls, {
      message: "expected the HTTP fallback path to be attempted after websocket bootstrap failure",
    })
    .toBeGreaterThan(0);

  await expect(monitorSection.getByText("连接中…", { exact: true })).not.toBeVisible();
  await expect(monitorSection.getByText("系统监控加载失败，请稍后重试", { exact: true })).toBeVisible();
});
