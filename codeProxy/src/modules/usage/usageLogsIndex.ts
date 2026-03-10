import { usageApi } from "@/lib/http/apis";

export type FlatUsageEntry = {
  timestamp: string;
  failed: boolean;
  source: string;
  apiKey: string;
  model: string;
  authIndex: string;
  inputTokens: number;
  outputTokens: number;
  reasoningTokens: number;
  cachedTokens: number;
  totalTokens: number;
};

const mapEntry = (item: Awaited<ReturnType<typeof usageApi.getUsageLogs>>["items"][number]): FlatUsageEntry => ({
  timestamp: item.timestamp,
  failed: item.failed,
  source: item.source,
  apiKey: item.api_key,
  model: item.model,
  authIndex: item.auth_index,
  inputTokens: item.input_tokens,
  outputTokens: item.output_tokens,
  reasoningTokens: item.reasoning_tokens,
  cachedTokens: item.cached_tokens,
  totalTokens: item.total_tokens,
});

export async function fetchFlatUsageEntries(days = 30, size = 200): Promise<FlatUsageEntry[]> {
  const entries: FlatUsageEntry[] = [];
  let page = 1;
  let total = 0;

  do {
    const response = await usageApi.getUsageLogs({ page, size, days });
    total = response.total;
    entries.push(...response.items.map(mapEntry));
    if (response.items.length < size) {
      break;
    }
    page += 1;
  } while (entries.length < total);

  return entries;
}

export async function fetchAllUsageLogs(
  days = 30,
  size = 200,
  filters?: {
    api_key?: string;
    model?: string;
    status?: string;
  },
) {
  const items: Awaited<ReturnType<typeof usageApi.getUsageLogs>>["items"] = [];
  let page = 1;
  let total = 0;

  do {
    const response = await usageApi.getUsageLogs({
      page,
      size,
      days,
      api_key: filters?.api_key,
      model: filters?.model,
      status: filters?.status,
    });
    total = response.total;
    items.push(...response.items);
    if (response.items.length < size) {
      return {
        ...response,
        items,
        total: Math.max(total, items.length),
      };
    }
    page += 1;
  } while (items.length < total);

  const lastPage =
    items.length === 0
      ? await usageApi.getUsageLogs({ page: 1, size, days })
      : await usageApi.getUsageLogs({ page: 1, size, days, api_key: filters?.api_key, model: filters?.model, status: filters?.status });

  return {
    ...lastPage,
    items,
    total: Math.max(total, items.length),
    page: 1,
    size: items.length || size,
  };
}
