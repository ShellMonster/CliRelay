import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, test, vi } from "vitest";
import { ProvidersPage } from "@/modules/providers/ProvidersPage";
import { ThemeProvider } from "@/modules/ui/ThemeProvider";
import { ToastProvider } from "@/modules/ui/ToastProvider";

const mocks = vi.hoisted(() => ({
  getGeminiKeys: vi.fn(async () => []),
  getClaudeConfigs: vi.fn(async () => []),
  getCodexConfigs: vi.fn(async () => []),
  getCodexCompatConfigs: vi.fn(async () => []),
  getCopilotCompatConfigs: vi.fn(async () => [
    {
      apiKey: "gho_existing",
      name: "Compat Existing",
      models: [null, { name: "gpt-5.4" }],
    },
  ]),
  getVertexConfigs: vi.fn(async () => []),
  getOpenAIProviders: vi.fn(async () => []),
  getAmpcode: vi.fn(async () => ({})),
  getModelMappings: vi.fn(async () => []),
  getUsageSourceStats: vi.fn(async () => ({ items: [] })),
  buildModelDiscoveryEndpoints: vi.fn(() => []),
}));

vi.mock("@/lib/http/apis", () => ({
  providersApi: {
    getGeminiKeys: mocks.getGeminiKeys,
    getClaudeConfigs: mocks.getClaudeConfigs,
    getCodexConfigs: mocks.getCodexConfigs,
    getCodexCompatConfigs: mocks.getCodexCompatConfigs,
    getCopilotCompatConfigs: mocks.getCopilotCompatConfigs,
    getVertexConfigs: mocks.getVertexConfigs,
    getOpenAIProviders: mocks.getOpenAIProviders,
    saveGeminiKeys: vi.fn(),
    saveClaudeConfigs: vi.fn(),
    saveCodexConfigs: vi.fn(),
    saveCodexCompatConfigs: vi.fn(),
    saveCopilotCompatConfigs: vi.fn(),
    saveVertexConfigs: vi.fn(),
    saveOpenAIProviders: vi.fn(),
    deleteGeminiKey: vi.fn(),
    deleteClaudeConfig: vi.fn(),
    deleteCodexConfig: vi.fn(),
    deleteCodexCompatConfig: vi.fn(),
    deleteCopilotCompatConfig: vi.fn(),
    deleteVertexConfig: vi.fn(),
  },
  ampcodeApi: {
    getAmpcode: mocks.getAmpcode,
    getModelMappings: mocks.getModelMappings,
    updateUpstreamUrl: vi.fn(),
    updateUpstreamApiKey: vi.fn(),
    updateForceModelMappings: vi.fn(),
    patchModelMappings: vi.fn(),
  },
  usageApi: {
    getUsageSourceStats: mocks.getUsageSourceStats,
  },
  modelsApi: {
    buildModelDiscoveryEndpoints: mocks.buildModelDiscoveryEndpoints,
    fetchProviderModelsViaApiCall: vi.fn(async () => []),
    getStaticModelDefinitions: vi.fn(async () => []),
  },
}));

describe("ProvidersPage GitHub Copilot editor", () => {
  test("malformed model entries do not crash the page and api key remains editable", async () => {
    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={["/ai-providers"]}>
        <ThemeProvider>
          <ToastProvider>
            <Routes>
              <Route path="/ai-providers" element={<ProvidersPage />} />
            </Routes>
          </ToastProvider>
        </ThemeProvider>
      </MemoryRouter>,
    );

    await screen.findByText("配置总览");
    await user.click(screen.getByText("GitHub Copilot", { exact: true }));

    await waitFor(() => {
      expect(mocks.getCopilotCompatConfigs).toHaveBeenCalledTimes(1);
    });

    expect(await screen.findByText("Compat Existing")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "编辑" }));

    const apiKeyInput = await screen.findByPlaceholderText("粘贴 API Key");
    await user.clear(apiKeyInput);
    await user.type(apiKeyInput, "new-copilot-key");

    expect(apiKeyInput).toHaveValue("new-copilot-key");
    expect(screen.getByText("展示：new-co***-key")).toBeInTheDocument();
  });
});
