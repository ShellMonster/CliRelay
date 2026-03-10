import { useCallback, useRef, useState } from "react";
import { useInterval } from "@/hooks/useInterval";
import { usageApi } from "@/lib/http/apis/usage";
import {
  buildCandidateUsageSourceIds,
  normalizeUsageSourceId,
  type KeyStats,
  type StatusBarData,
  type StatusBlockState,
} from "@/utils/usage";
import type { GeminiKeyConfig, OpenAIProviderConfig, ProviderKeyConfig } from "@/types";

const EMPTY_STATS: KeyStats = { bySource: {}, byAuthIndex: {} };
const BLOCK_COUNT = 20;
const BLOCK_DURATION_MS = 10 * 60 * 1000;
const WINDOW_MS = BLOCK_COUNT * BLOCK_DURATION_MS;

type ProviderCollections = {
  geminiKeys: GeminiKeyConfig[];
  codexConfigs: ProviderKeyConfig[];
  claudeConfigs: ProviderKeyConfig[];
  vertexConfigs: ProviderKeyConfig[];
  openaiProviders: OpenAIProviderConfig[];
};

type ProviderStatusCache = {
  gemini: Map<string, StatusBarData>;
  codex: Map<string, StatusBarData>;
  claude: Map<string, StatusBarData>;
  vertex: Map<string, StatusBarData>;
  openai: Map<string, StatusBarData>;
};

const createEmptyStatusBarData = (nowMs: number): StatusBarData => {
  const windowStart = nowMs - WINDOW_MS;
  const blocks: StatusBlockState[] = Array.from({ length: BLOCK_COUNT }, () => "idle");
  const blockDetails = Array.from({ length: BLOCK_COUNT }, (_, idx) => {
    const startTime = windowStart + idx * BLOCK_DURATION_MS;
    return {
      success: 0,
      failure: 0,
      rate: -1,
      startTime,
      endTime: startTime + BLOCK_DURATION_MS,
    };
  });

  return {
    blocks,
    blockDetails,
    successRate: 100,
    totalSuccess: 0,
    totalFailure: 0,
  };
};

const createEmptyProviderStatusCache = (): ProviderStatusCache => ({
  gemini: new Map(),
  codex: new Map(),
  claude: new Map(),
  vertex: new Map(),
  openai: new Map(),
});

const applyHealthPoint = (
  cache: Map<string, StatusBarData>,
  key: string,
  bucket: string,
  successCount: number,
  failureCount: number,
  nowMs: number,
) => {
  if (!key) return;
  const bucketTime = Date.parse(bucket);
  if (Number.isNaN(bucketTime)) return;
  const windowStart = nowMs - WINDOW_MS;
  if (bucketTime < windowStart || bucketTime > nowMs) return;

  const ageMs = nowMs - bucketTime;
  const blockIndex = BLOCK_COUNT - 1 - Math.floor(ageMs / BLOCK_DURATION_MS);
  if (blockIndex < 0 || blockIndex >= BLOCK_COUNT) return;

  const statusData = cache.get(key) ?? createEmptyStatusBarData(nowMs);
  const detail = statusData.blockDetails[blockIndex];
  detail.success += successCount;
  detail.failure += failureCount;
  const total = detail.success + detail.failure;
  detail.rate = total > 0 ? detail.success / total : -1;

  statusData.totalSuccess += successCount;
  statusData.totalFailure += failureCount;
  statusData.blocks[blockIndex] =
    total === 0
      ? "idle"
      : detail.failure === 0
        ? "success"
        : detail.success === 0
          ? "failure"
          : "mixed";

  const statusTotal = statusData.totalSuccess + statusData.totalFailure;
  statusData.successRate = statusTotal > 0 ? (statusData.totalSuccess / statusTotal) * 100 : 100;
  cache.set(key, statusData);
};

export const useProviderStats = (collections: ProviderCollections) => {
  const [keyStats, setKeyStats] = useState<KeyStats>(EMPTY_STATS);
  const [statusBarCache, setStatusBarCache] = useState<ProviderStatusCache>(
    createEmptyProviderStatusCache(),
  );
  const [isLoading, setIsLoading] = useState(false);
  const loadingRef = useRef(false);

  const loadKeyStats = useCallback(async () => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setIsLoading(true);
    try {
      const [overview, credentialHealth] = await Promise.all([
        usageApi.getUsageOverview(30),
        usageApi.getUsageCredentialHealth(30),
      ]);
      const stats: KeyStats = { bySource: {}, byAuthIndex: {} };
      overview.credentials.forEach((entry) => {
        const source = normalizeUsageSourceId(entry.source);
        if (source) {
          const bucket = (stats.bySource[source] ??= { success: 0, failure: 0 });
          bucket.success += entry.success_count;
          bucket.failure += entry.failure_count;
        }
        const authIndex = entry.auth_index || "";
        if (authIndex) {
          const bucket = (stats.byAuthIndex[authIndex] ??= { success: 0, failure: 0 });
          bucket.success += entry.success_count;
          bucket.failure += entry.failure_count;
        }
      });

      const nextStatusBarCache = createEmptyProviderStatusCache();
      const nowMs = Date.now();

      const providerSourceMaps = {
        gemini: new Map<string, string>(),
        codex: new Map<string, string>(),
        claude: new Map<string, string>(),
        vertex: new Map<string, string>(),
        openai: new Map<string, string>(),
      };

      collections.geminiKeys.forEach((config) => {
        if (!config.apiKey) return;
        buildCandidateUsageSourceIds({ apiKey: config.apiKey, prefix: config.prefix }).forEach((id) =>
          providerSourceMaps.gemini.set(id, config.apiKey),
        );
      });
      collections.codexConfigs.forEach((config) => {
        if (!config.apiKey) return;
        buildCandidateUsageSourceIds({ apiKey: config.apiKey, prefix: config.prefix }).forEach((id) =>
          providerSourceMaps.codex.set(id, config.apiKey),
        );
      });
      collections.claudeConfigs.forEach((config) => {
        if (!config.apiKey) return;
        buildCandidateUsageSourceIds({ apiKey: config.apiKey, prefix: config.prefix }).forEach((id) =>
          providerSourceMaps.claude.set(id, config.apiKey),
        );
      });
      collections.vertexConfigs.forEach((config) => {
        if (!config.apiKey) return;
        buildCandidateUsageSourceIds({ apiKey: config.apiKey, prefix: config.prefix }).forEach((id) =>
          providerSourceMaps.vertex.set(id, config.apiKey),
        );
      });
      collections.openaiProviders.forEach((provider) => {
        const providerKey = provider.name;
        buildCandidateUsageSourceIds({ prefix: provider.prefix }).forEach((id) =>
          providerSourceMaps.openai.set(id, providerKey),
        );
        (provider.apiKeyEntries || []).forEach((entry) => {
          buildCandidateUsageSourceIds({ apiKey: entry.apiKey }).forEach((id) =>
            providerSourceMaps.openai.set(id, providerKey),
          );
        });
      });

      credentialHealth.items.forEach((item) => {
        const source = normalizeUsageSourceId(item.source);
        if (!source) return;
        const successCount = item.success_count;
        const failureCount = item.failure_count;
        const bucket = item.bucket;

        const geminiKey = providerSourceMaps.gemini.get(source);
        if (geminiKey) {
          applyHealthPoint(nextStatusBarCache.gemini, geminiKey, bucket, successCount, failureCount, nowMs);
        }
        const codexKey = providerSourceMaps.codex.get(source);
        if (codexKey) {
          applyHealthPoint(nextStatusBarCache.codex, codexKey, bucket, successCount, failureCount, nowMs);
        }
        const claudeKey = providerSourceMaps.claude.get(source);
        if (claudeKey) {
          applyHealthPoint(nextStatusBarCache.claude, claudeKey, bucket, successCount, failureCount, nowMs);
        }
        const vertexKey = providerSourceMaps.vertex.get(source);
        if (vertexKey) {
          applyHealthPoint(nextStatusBarCache.vertex, vertexKey, bucket, successCount, failureCount, nowMs);
        }
        const openaiKey = providerSourceMaps.openai.get(source);
        if (openaiKey) {
          applyHealthPoint(nextStatusBarCache.openai, openaiKey, bucket, successCount, failureCount, nowMs);
        }
      });

      setKeyStats(stats);
      setStatusBarCache(nextStatusBarCache);
    } catch {
      // 静默失败
    } finally {
      loadingRef.current = false;
      setIsLoading(false);
    }
  }, []);

  useInterval(loadKeyStats, 240_000);

  return { keyStats, statusBarCache, loadKeyStats, isLoading };
}
