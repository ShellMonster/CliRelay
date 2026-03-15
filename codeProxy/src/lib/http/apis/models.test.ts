import { beforeEach, describe, expect, it, vi } from "vitest";

const { requestMock, getMock } = vi.hoisted(() => ({
  requestMock: vi.fn(),
  getMock: vi.fn(),
}));

vi.mock("@/lib/http/apis/api-call", () => ({
  apiCallApi: {
    request: requestMock,
  },
  getApiCallErrorMessage: (result: {
    statusCode?: number;
    body?: unknown;
    bodyText?: string;
  }) => {
    const status = Number(result?.statusCode ?? 0);
    const body =
      typeof result?.body === "string"
        ? result.body
        : (result?.body as { error?: { message?: string } | string; message?: string } | null);
    const message =
      typeof body === "string"
        ? body
        : typeof body?.error === "string"
          ? body.error
          : body?.error && typeof body.error === "object" && "message" in body.error
            ? String(body.error.message ?? "")
            : body?.message || result?.bodyText || "";
    return status && message ? `${status} ${message}`.trim() : `HTTP ${status || 0}`;
  },
}));

vi.mock("@/lib/http/client", () => ({
  apiClient: {
    get: getMock,
  },
}));

import { modelsApi } from "./models";

describe("modelsApi model discovery fallback", () => {
  beforeEach(() => {
    requestMock.mockReset();
    getMock.mockReset();
  });

  it("builds distinct openai-compatible fallback endpoints", () => {
    expect(modelsApi.buildModelDiscoveryEndpoints("openai", "https://api.githubcopilot.com")).toEqual([
      "https://api.githubcopilot.com/models",
      "https://api.githubcopilot.com/v1/models",
    ]);

    expect(
      modelsApi.buildModelDiscoveryEndpoints("openai", "https://api.githubcopilot.com/v1"),
    ).toEqual([
      "https://api.githubcopilot.com/models",
      "https://api.githubcopilot.com/v1/models",
    ]);
  });

  it("falls back from /models to /v1/models for codex-style providers", async () => {
    requestMock
      .mockResolvedValueOnce({
        statusCode: 404,
        header: {},
        bodyText: "not found",
        body: "not found",
      })
      .mockResolvedValueOnce({
        statusCode: 200,
        header: {},
        bodyText: JSON.stringify({ data: [{ id: "gpt-5.4" }] }),
        body: { data: [{ id: "gpt-5.4" }] },
      });

    const models = await modelsApi.fetchProviderModelsViaApiCall(
      "codex",
      "https://api.githubcopilot.com",
      "sk-test",
    );

    expect(models).toEqual([{ name: "gpt-5.4" }]);
    expect(requestMock).toHaveBeenCalledTimes(2);
    expect(requestMock.mock.calls[0]?.[0]).toMatchObject({
      method: "GET",
      url: "https://api.githubcopilot.com/models",
      header: { Authorization: "Bearer sk-test" },
    });
    expect(requestMock.mock.calls[1]?.[0]).toMatchObject({
      method: "GET",
      url: "https://api.githubcopilot.com/v1/models",
      header: { Authorization: "Bearer sk-test" },
    });
  });

  it("falls back from anthropic /v1/models to bearer /models for claude providers", async () => {
    requestMock
      .mockResolvedValueOnce({
        statusCode: 404,
        header: {},
        bodyText: "not found",
        body: "not found",
      })
      .mockResolvedValueOnce({
        statusCode: 200,
        header: {},
        bodyText: JSON.stringify({ data: [{ id: "claude-3.7-sonnet" }] }),
        body: { data: [{ id: "claude-3.7-sonnet" }] },
      });

    const models = await modelsApi.fetchProviderModelsViaApiCall(
      "claude",
      "https://api.githubcopilot.com",
      "sk-claude",
    );

    expect(models).toEqual([{ name: "claude-3.7-sonnet" }]);
    expect(requestMock).toHaveBeenCalledTimes(2);
    expect(requestMock.mock.calls[0]?.[0]).toMatchObject({
      method: "GET",
      url: "https://api.githubcopilot.com/v1/models",
      header: {
        "x-api-key": "sk-claude",
        "anthropic-version": "2023-06-01",
      },
    });
    expect(requestMock.mock.calls[1]?.[0]).toMatchObject({
      method: "GET",
      url: "https://api.githubcopilot.com/models",
      header: {
        Authorization: "Bearer sk-claude",
      },
    });
  });
});
