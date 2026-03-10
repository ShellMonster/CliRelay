import { useCallback, useRef, useState } from "react";
import type { KeyStats, StatusBarData, StatusBlockState } from "@/utils/usage";
import { usageApi } from "@/lib/http/apis/usage";
import { normalizeUsageSourceId } from "@/utils/usage";

export type UseAuthFilesStatsResult = {
  keyStats: KeyStats;
  statusBarCache: Map<string, StatusBarData>;
  loadKeyStats: () => Promise<void>;
};

const BLOCK_COUNT = 20;
const BLOCK_DURATION_MS = 10 * 60 * 1000;
const WINDOW_MS = BLOCK_COUNT * BLOCK_DURATION_MS;

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

export function useAuthFilesStats(): UseAuthFilesStatsResult {
  const [keyStats, setKeyStats] = useState<KeyStats>({ bySource: {}, byAuthIndex: {} });
  const [statusBarCache, setStatusBarCache] = useState<Map<string, StatusBarData>>(new Map());
  const loadingKeyStatsRef = useRef(false);

  const loadKeyStats = useCallback(async () => {
    if (loadingKeyStatsRef.current) return;
    loadingKeyStatsRef.current = true;
    try {
      const [overview, credentialHealth] = await Promise.all([
        usageApi.getUsageOverview(30),
        usageApi.getUsageCredentialHealth(30),
      ]);
      const stats: KeyStats = { bySource: {}, byAuthIndex: {} };
      overview.credentials.forEach((item) => {
        const source = normalizeUsageSourceId(item.source);
        if (source) {
          stats.bySource[source] = {
            success: item.success_count,
            failure: item.failure_count,
          };
        }
        const authIndex = item.auth_index || "";
        if (authIndex) {
          stats.byAuthIndex[authIndex] = {
            success: item.success_count,
            failure: item.failure_count,
          };
        }
      });

      const nowMs = Date.now();
      const windowStart = nowMs - WINDOW_MS;
      const nextStatusBarCache = new Map<string, StatusBarData>();
      credentialHealth.items.forEach((item) => {
        const authIndex = item.auth_index.trim();
        if (!authIndex) return;

        const bucketTime = Date.parse(item.bucket);
        if (Number.isNaN(bucketTime) || bucketTime < windowStart || bucketTime > nowMs) return;

        const ageMs = nowMs - bucketTime;
        const blockIndex = BLOCK_COUNT - 1 - Math.floor(ageMs / BLOCK_DURATION_MS);
        if (blockIndex < 0 || blockIndex >= BLOCK_COUNT) return;

        const statusData = nextStatusBarCache.get(authIndex) ?? createEmptyStatusBarData(nowMs);
        const detail = statusData.blockDetails[blockIndex];
        detail.success += item.success_count;
        detail.failure += item.failure_count;
        const total = detail.success + detail.failure;
        detail.rate = total > 0 ? detail.success / total : -1;
        statusData.totalSuccess += item.success_count;
        statusData.totalFailure += item.failure_count;
        statusData.blocks[blockIndex] =
          total === 0
            ? "idle"
            : detail.failure === 0
              ? "success"
              : detail.success === 0
                ? "failure"
                : "mixed";
        const statusTotal = statusData.totalSuccess + statusData.totalFailure;
        statusData.successRate =
          statusTotal > 0 ? (statusData.totalSuccess / statusTotal) * 100 : 100;
        nextStatusBarCache.set(authIndex, statusData);
      });

      setKeyStats(stats);
      setStatusBarCache(nextStatusBarCache);
    } catch {
      // 静默失败
    } finally {
      loadingKeyStatsRef.current = false;
    }
  }, []);

  return { keyStats, statusBarCache, loadKeyStats };
}
