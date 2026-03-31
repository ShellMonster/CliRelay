import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Plus,
  Copy,
  Pencil,
  Trash2,
  KeyRound,
  ShieldCheck,
  RefreshCw,
  Infinity,
  BarChart3,
  Power,
} from "lucide-react";
import {
  apiKeyEntriesApi,
  apiKeysApi,
  type ApiKeyAccessChannelOption,
  type ApiKeyAccessProviderOption,
  type ApiKeyEntry,
  type ApiKeyProviderAccessEntry,
} from "@/lib/http/apis/api-keys";
import { usageApi } from "@/lib/http/apis";
import type { UsageLogItem } from "@/lib/http/apis/usage";
import { Card } from "@/modules/ui/Card";
import { Button } from "@/modules/ui/Button";
import { EmptyState } from "@/modules/ui/EmptyState";
import { useToast } from "@/modules/ui/ToastProvider";
import { Modal } from "@/modules/ui/Modal";
import { HoverTooltip, OverflowTooltip } from "@/modules/ui/Tooltip";
import { MultiSelect, type MultiSelectOption } from "@/modules/ui/MultiSelect";
import { ToggleSwitch } from "@/modules/ui/ToggleSwitch";
import { VirtualTable, type VirtualTableColumn } from "@/modules/ui/VirtualTable";

/* ─── helpers ─── */

const generateKey = () => {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let result = "sk-";
  for (let i = 0; i < 32; i++) {
    result += chars[Math.floor(Math.random() * chars.length)];
  }
  return result;
};

const maskKey = (key: string) => {
  if (key.length <= 8) return key;
  return key.slice(0, 5) + "•".repeat(Math.min(key.length - 8, 20)) + key.slice(-3);
};

const formatDate = (iso: string | undefined) => {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleDateString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
};

const formatLimit = (limit: number | undefined) => {
  if (!limit || limit <= 0) return "无限制";
  return limit.toLocaleString();
};

const formatTimestamp = (value: string): string => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || "--";
  return date.toLocaleString();
};

const formatLatencyMs = (value: number): string => {
  if (!Number.isFinite(value) || value < 0) return "--";
  if (value < 1) return "<1ms";
  if (value < 1000) return `${Math.round(value)}ms`;
  const seconds = value / 1000;
  const fixed = seconds.toFixed(seconds < 10 ? 2 : 1);
  const trimmed = fixed.endsWith(".0") ? fixed.slice(0, -2) : fixed;
  return `${trimmed}s`;
};

const API_KEY_PROVIDER_LABELS: Record<string, string> = {
  gemini: "Gemini",
  "gemini-cli": "Gemini CLI",
  claude: "Claude",
  codex: "Codex",
  "codex-compat": "Codex Compat",
  "copilot-compat": "GitHub Copilot",
  vertex: "Vertex",
  "openai-compatibility": "OpenAI Compatible",
  ampcode: "Ampcode",
  antigravity: "Antigravity",
  qwen: "Qwen",
  kimi: "Kimi",
  iflow: "iFlow",
  aistudio: "AI Studio",
};

const API_KEY_PROVIDER_ORDER = [
  "gemini",
  "gemini-cli",
  "claude",
  "codex",
  "codex-compat",
  "copilot-compat",
  "vertex",
  "openai-compatibility",
  "ampcode",
  "antigravity",
  "qwen",
  "kimi",
  "iflow",
  "aistudio",
];

const getProviderLabel = (provider: string) => API_KEY_PROVIDER_LABELS[provider] || provider;

const compareProviderOption = (
  left: Pick<ApiKeyAccessProviderOption, "provider" | "label">,
  right: Pick<ApiKeyAccessProviderOption, "provider" | "label">,
) => {
  const leftIndex = API_KEY_PROVIDER_ORDER.indexOf(left.provider);
  const rightIndex = API_KEY_PROVIDER_ORDER.indexOf(right.provider);
  if (leftIndex >= 0 || rightIndex >= 0) {
    if (leftIndex < 0) return 1;
    if (rightIndex < 0) return -1;
    if (leftIndex !== rightIndex) return leftIndex - rightIndex;
  }
  return left.label.localeCompare(right.label, "zh-CN");
};

const normalizeProviderAccess = (
  entries: ApiKeyProviderAccessEntry[] | undefined,
): ApiKeyProviderAccessEntry[] => {
  if (!Array.isArray(entries) || entries.length === 0) return [];

  const order: string[] = [];
  const rules = new Map<
    string,
    {
      channels: string[] | null;
      models: string[] | null;
    }
  >();
  const seenChannelsByProvider = new Map<string, Set<string>>();
  const seenModelsByProvider = new Map<string, Set<string>>();

  entries.forEach((entry) => {
    const provider = String(entry.provider || "")
      .trim()
      .toLowerCase();
    if (!provider) return;
    if (!rules.has(provider)) {
      rules.set(provider, { channels: [], models: [] });
      seenChannelsByProvider.set(provider, new Set());
      seenModelsByProvider.set(provider, new Set());
      order.push(provider);
    }

    const channels = Array.isArray(entry.channels)
      ? entry.channels.map((channel) => String(channel || "").trim()).filter(Boolean)
      : [];
    const models = Array.isArray(entry.models)
      ? entry.models.map((model) => String(model || "").trim()).filter(Boolean)
      : [];

    const current = rules.get(provider) ?? { channels: [], models: [] };

    if (channels.length === 0) {
      current.channels = null;
      seenChannelsByProvider.delete(provider);
    } else if (current.channels !== null) {
      const nextChannels = current.channels ?? [];
      const seenChannels = seenChannelsByProvider.get(provider) ?? new Set<string>();
      channels.forEach((channel) => {
        if (seenChannels.has(channel)) return;
        seenChannels.add(channel);
        nextChannels.push(channel);
      });
      current.channels = nextChannels;
      seenChannelsByProvider.set(provider, seenChannels);
    }

    if (models.length === 0) {
      current.models = null;
      seenModelsByProvider.delete(provider);
    } else if (current.models !== null) {
      const nextModels = current.models ?? [];
      const seenModels = seenModelsByProvider.get(provider) ?? new Set<string>();
      models.forEach((model) => {
        if (seenModels.has(model)) return;
        seenModels.add(model);
        nextModels.push(model);
      });
      current.models = nextModels;
      seenModelsByProvider.set(provider, seenModels);
    }

    rules.set(provider, current);
  });

  return order.map((provider) => {
    const current = rules.get(provider);
    if (!current) {
      return { provider };
    }
    const nextEntry: ApiKeyProviderAccessEntry = { provider };
    if (Array.isArray(current.channels) && current.channels.length > 0) {
      nextEntry.channels = current.channels;
    }
    if (Array.isArray(current.models) && current.models.length > 0) {
      nextEntry.models = current.models;
    }
    return nextEntry;
  });
};

const describeProviderAccess = (
  entries: ApiKeyProviderAccessEntry[] | undefined,
  providerOptions: ApiKeyAccessProviderOption[],
): string[] => {
  const normalized = normalizeProviderAccess(entries);
  const providerMap = new Map<string, ApiKeyAccessProviderOption>();
  providerOptions.forEach((option) => {
    providerMap.set(
      String(option.provider || "")
        .trim()
        .toLowerCase(),
      option,
    );
  });

  return normalized.map((entry) => {
    const provider = providerMap.get(entry.provider);
    const label = provider?.label || getProviderLabel(entry.provider);
    const selectedChannels = Array.isArray(entry.channels) ? entry.channels.filter(Boolean) : [];
    if (selectedChannels.length === 0) {
      return `${label}：全部供应商`;
    }

    const channelLabelMap = new Map<string, string>();
    provider?.channels.forEach((channel) => {
      channelLabelMap.set(channel.id, channel.label || channel.id);
    });
    const channelLabels = selectedChannels.map(
      (channelID) => channelLabelMap.get(channelID) || channelID,
    );
    return `${label}：${channelLabels.join(", ")}`;
  });
};

/* ─── usage detail row type ─── */

interface UsageLogRow {
  id: string;
  timestamp: string;
  model: string;
  failed: boolean;
  latencyText: string;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
}

const mapUsageLogRow = (item: UsageLogItem): UsageLogRow => ({
  id: String(item.id),
  timestamp: item.timestamp,
  model: item.model,
  failed: item.failed,
  latencyText: formatLatencyMs(item.latency_ms),
  inputTokens: item.input_tokens,
  outputTokens: item.output_tokens,
  totalTokens: item.total_tokens,
});

/* ─── types ─── */

interface FormValues {
  name: string;
  key: string;
  dailyLimit: string;
  totalQuota: string;
  allowedModels: string[];
  providerAccess: ApiKeyProviderAccessEntry[];
}

/* ─── component ─── */

export function ApiKeysPage() {
  const { notify } = useToast();

  const [entries, setEntries] = useState<ApiKeyEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [editIndex, setEditIndex] = useState<number | null>(null);
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null);
  const [usageViewKey, setUsageViewKey] = useState<string | null>(null);
  const [usageViewName, setUsageViewName] = useState<string>("");
  const [saving, setSaving] = useState(false);
  const [usageRows, setUsageRows] = useState<UsageLogRow[]>([]);
  const [usageLoading, setUsageLoading] = useState(false);
  const [usageTotal, setUsageTotal] = useState(0);
  const [usagePage, setUsagePage] = useState(1);
  const [accessOptions, setAccessOptions] = useState<ApiKeyAccessProviderOption[]>([]);
  const [accessOptionsLoading, setAccessOptionsLoading] = useState(false);
  const [form, setForm] = useState<FormValues>({
    name: "",
    key: "",
    dailyLimit: "",
    totalQuota: "",
    allowedModels: [],
    providerAccess: [],
  });

  const providerOptions = useMemo(() => {
    const optionMap = new Map<string, ApiKeyAccessProviderOption>();
    const sortChannels = (channels: ApiKeyAccessChannelOption[]) =>
      [...channels].sort((left, right) => left.label.localeCompare(right.label, "zh-CN"));

    accessOptions.forEach((option) => {
      const provider = String(option.provider || "")
        .trim()
        .toLowerCase();
      if (!provider) return;

      const channelMap = new Map<string, ApiKeyAccessChannelOption>();
      (Array.isArray(option.channels) ? option.channels : []).forEach((channel) => {
        const id = String(channel.id || "").trim();
        if (!id) return;
        channelMap.set(id, {
          id,
          label: String(channel.label || channel.id || "").trim() || id,
          models: Array.isArray(channel.models)
            ? channel.models
                .map((model) => {
                  const modelID = String(model.id || "").trim();
                  if (!modelID) return null;
                  return {
                    id: modelID,
                    label: String(model.label || model.id || "").trim() || modelID,
                  };
                })
                .filter((model): model is { id: string; label: string } => model !== null)
            : [],
        });
      });

      optionMap.set(provider, {
        provider,
        label: option.label || getProviderLabel(provider),
        channels: sortChannels(Array.from(channelMap.values())),
      });
    });

    normalizeProviderAccess(form.providerAccess).forEach((entry) => {
      const provider = entry.provider;
      const existing = optionMap.get(provider);
      const channelMap = new Map<string, ApiKeyAccessChannelOption>();

      (existing?.channels || []).forEach((channel) => {
        channelMap.set(channel.id, channel);
      });

      entry.channels?.forEach((channel) => {
        const normalized = String(channel || "").trim();
        if (!normalized || channelMap.has(normalized)) return;
        channelMap.set(normalized, {
          id: normalized,
          label: normalized,
          models: [],
        });
      });

      optionMap.set(provider, {
        provider,
        label: existing?.label || getProviderLabel(provider),
        channels: sortChannels(Array.from(channelMap.values())),
      });
    });

    return Array.from(optionMap.values()).sort(compareProviderOption);
  }, [accessOptions, form.providerAccess]);

  const providerOptionMap = useMemo(() => {
    const optionMap = new Map<string, ApiKeyAccessProviderOption>();
    providerOptions.forEach((option) => {
      optionMap.set(option.provider, option);
    });
    return optionMap;
  }, [providerOptions]);

  const providerAccessMap = useMemo(() => {
    const accessMap = new Map<string, ApiKeyProviderAccessEntry>();
    normalizeProviderAccess(form.providerAccess).forEach((entry) => {
      accessMap.set(entry.provider, entry);
    });
    return accessMap;
  }, [form.providerAccess]);

  const modelOptions = useMemo(() => {
    const optionMap = new Map<string, MultiSelectOption>();
    const addModel = (id: string, label: string) => {
      const normalized = String(id || "").trim();
      if (!normalized) return;
      optionMap.set(normalized, {
        value: normalized,
        label: String(label || id || "").trim() || normalized,
      });
    };

    const normalizedAccess = normalizeProviderAccess(form.providerAccess);
    if (normalizedAccess.length === 0) {
      providerOptions.forEach((provider) => {
        provider.channels.forEach((channel) => {
          channel.models.forEach((model) => addModel(model.id, model.label));
        });
      });
    } else {
      normalizedAccess.forEach((entry) => {
        const provider = providerOptionMap.get(entry.provider);
        if (!provider) {
          entry.models?.forEach((model) => addModel(model, model));
          return;
        }

        const selectedChannelSet =
          Array.isArray(entry.channels) && entry.channels.length > 0
            ? new Set(entry.channels)
            : null;
        provider.channels.forEach((channel) => {
          if (selectedChannelSet && !selectedChannelSet.has(channel.id)) return;
          channel.models.forEach((model) => addModel(model.id, model.label));
        });

        entry.models?.forEach((model) => addModel(model, model));
      });
    }

    form.allowedModels.forEach((model) => {
      addModel(model, model);
    });

    return Array.from(optionMap.values()).sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
  }, [providerOptionMap, providerOptions, form.allowedModels, form.providerAccess]);

  /* ─── load ─── */

  const loadPage = useCallback(async () => {
    setLoading(true);
    setAccessOptionsLoading(true);
    try {
      const [entriesData, legacyKeys, nextAccessOptions] = await Promise.all([
        apiKeyEntriesApi.list(),
        apiKeysApi.list().catch(() => [] as string[]),
        apiKeyEntriesApi.getAccessOptions().catch((err: unknown) => {
          notify({
            type: "error",
            message: err instanceof Error ? err.message : "加载供应商限制选项失败",
          });
          return [] as ApiKeyAccessProviderOption[];
        }),
      ]);

      // Canonicalize top-level legacy api-keys into api-key-entries so later disable/delete
      // operations are authoritative and don't leave an active legacy duplicate behind.
      const entryKeySet = new Set(
        entriesData.map((entry) => String(entry.key || "").trim()).filter(Boolean),
      );
      const legacyEntryKeys = Array.from(
        new Set(legacyKeys.map((key) => String(key || "").trim()).filter(Boolean)),
      );
      const missingEntries = legacyEntryKeys
        .filter((key) => !entryKeySet.has(key))
        .map(
          (key): ApiKeyEntry => ({
            key,
            "created-at": new Date().toISOString(),
          }),
        );

      let finalEntries = entriesData;
      if (legacyEntryKeys.length > 0) {
        const merged =
          missingEntries.length > 0 ? [...entriesData, ...missingEntries] : entriesData;
        try {
          await apiKeyEntriesApi.replace(merged);
          if (missingEntries.length > 0) {
            notify({
              type: "success",
              message: `已自动导入 ${missingEntries.length} 个旧 API Key`,
            });
          }
          finalEntries = merged;
        } catch (err: unknown) {
          notify({
            type: "error",
            message: err instanceof Error ? err.message : "同步旧 API Key 失败",
          });
        }
      }
      setEntries(finalEntries);
      setAccessOptions(nextAccessOptions);
    } catch (err: unknown) {
      notify({ type: "error", message: err instanceof Error ? err.message : "加载 API Keys 失败" });
    } finally {
      setLoading(false);
      setAccessOptionsLoading(false);
    }
  }, [notify]);

  const loadUsage = useCallback(
    async (apiKey: string, page = 1) => {
      setUsageLoading(true);
      try {
        const response = await usageApi.getUsageLogs({
          page,
          size: 100,
          days: 30,
          api_key: apiKey,
        });
        const rows = (response.items ?? []).map(mapUsageLogRow);
        setUsageRows((prev) => (page === 1 ? rows : [...prev, ...rows]));
        setUsageTotal(response.total ?? 0);
        setUsagePage(page);
      } catch (err: unknown) {
        notify({ type: "error", message: err instanceof Error ? err.message : "加载调用记录失败" });
      } finally {
        setUsageLoading(false);
      }
    },
    [notify],
  );

  useEffect(() => {
    void loadPage();
  }, [loadPage]);

  /* ─── toggle disable ─── */

  const handleToggleDisable = async (index: number) => {
    const entry = entries[index];
    const updated = { ...entry, disabled: !entry.disabled };
    const newEntries = [...entries];
    newEntries[index] = updated;

    try {
      await apiKeyEntriesApi.replace(newEntries);
      setEntries(newEntries);
      notify({
        type: "success",
        message: updated.disabled
          ? `已禁用「${entry.name || "未命名"}」`
          : `已启用「${entry.name || "未命名"}」`,
      });
    } catch (err: unknown) {
      notify({ type: "error", message: err instanceof Error ? err.message : "操作失败" });
    }
  };

  /* ─── create ─── */

  const handleOpenCreate = () => {
    setForm({
      name: "",
      key: generateKey(),
      dailyLimit: "",
      totalQuota: "",
      allowedModels: [],
      providerAccess: [],
    });
    setShowCreate(true);
  };

  const handleProviderToggle = (provider: string, enabled: boolean) => {
    const normalizedProvider = String(provider || "")
      .trim()
      .toLowerCase();
    if (!normalizedProvider) return;
    setForm((prev) => {
      const next = normalizeProviderAccess(prev.providerAccess);
      const existing = next.find((entry) => entry.provider === normalizedProvider);
      const filtered = next.filter((entry) => entry.provider !== normalizedProvider);
      if (enabled) {
        filtered.push(existing || { provider: normalizedProvider });
      }
      return { ...prev, providerAccess: normalizeProviderAccess(filtered) };
    });
  };

  const handleProviderChannelsChange = (provider: string, channels: string[]) => {
    const normalizedProvider = String(provider || "")
      .trim()
      .toLowerCase();
    if (!normalizedProvider) return;
    setForm((prev) => {
      const normalizedAccess = normalizeProviderAccess(prev.providerAccess);
      const existing = normalizedAccess.find((entry) => entry.provider === normalizedProvider);
      const next = normalizedAccess.filter((entry) => entry.provider !== normalizedProvider);
      next.push({
        provider: normalizedProvider,
        channels: channels.length > 0 ? channels : undefined,
        models: existing?.models,
      });
      return { ...prev, providerAccess: normalizeProviderAccess(next) };
    });
  };

  const handleAllowedModelsChange = (models: string[]) => {
    setForm((prev) => ({
      ...prev,
      allowedModels: models,
    }));
  };

  const handleCreate = async () => {
    if (!form.name.trim()) {
      notify({ type: "error", message: "请填写 API Key 名称" });
      return;
    }
    if (!form.key.trim()) {
      notify({ type: "error", message: "Key 不能为空" });
      return;
    }
    setSaving(true);
    try {
      const newEntry: ApiKeyEntry = {
        key: form.key.trim(),
        name: form.name.trim(),
        "daily-limit": form.dailyLimit ? parseInt(form.dailyLimit, 10) || 0 : undefined,
        "total-quota": form.totalQuota ? parseInt(form.totalQuota, 10) || 0 : undefined,
        "allowed-models": form.allowedModels.length > 0 ? form.allowedModels : undefined,
        "provider-access": form.providerAccess.length > 0 ? form.providerAccess : undefined,
        "created-at": new Date().toISOString(),
      };
      await apiKeyEntriesApi.replace([...entries, newEntry]);
      notify({ type: "success", message: "创建成功" });
      setShowCreate(false);
      await loadPage();
    } catch (err: unknown) {
      notify({ type: "error", message: err instanceof Error ? err.message : "创建失败" });
    } finally {
      setSaving(false);
    }
  };

  /* ─── edit ─── */

  const handleOpenEdit = (index: number) => {
    const entry = entries[index];
    setForm({
      name: entry.name || "",
      key: entry.key,
      dailyLimit: entry["daily-limit"]?.toString() || "",
      totalQuota: entry["total-quota"]?.toString() || "",
      allowedModels: entry["allowed-models"] || [],
      providerAccess: normalizeProviderAccess(entry["provider-access"]),
    });
    setEditIndex(index);
  };

  const handleEdit = async () => {
    if (editIndex === null) return;
    if (!form.name.trim()) {
      notify({ type: "error", message: "请填写 API Key 名称" });
      return;
    }
    setSaving(true);
    try {
      await apiKeyEntriesApi.update({
        index: editIndex,
        value: {
          name: form.name.trim(),
          "daily-limit": form.dailyLimit ? parseInt(form.dailyLimit, 10) || 0 : 0,
          "total-quota": form.totalQuota ? parseInt(form.totalQuota, 10) || 0 : 0,
          "allowed-models": form.allowedModels.length > 0 ? form.allowedModels : [],
          "provider-access": form.providerAccess,
        },
      });
      notify({ type: "success", message: "更新成功" });
      setEditIndex(null);
      await loadPage();
    } catch (err: unknown) {
      notify({ type: "error", message: err instanceof Error ? err.message : "更新失败" });
    } finally {
      setSaving(false);
    }
  };

  /* ─── delete ─── */

  const handleDelete = async () => {
    if (deleteIndex === null) return;
    setSaving(true);
    try {
      await apiKeyEntriesApi.delete({ index: deleteIndex });
      notify({ type: "success", message: "删除成功" });
      setDeleteIndex(null);
      await loadPage();
    } catch (err: unknown) {
      notify({ type: "error", message: err instanceof Error ? err.message : "删除失败" });
    } finally {
      setSaving(false);
    }
  };

  /* ─── copy ─── */

  const handleCopy = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key);
      notify({ type: "success", message: "已复制到剪贴板" });
    } catch {
      notify({ type: "error", message: "复制失败" });
    }
  };

  /* ─── usage view ─── */

  const handleViewUsage = (entry: ApiKeyEntry) => {
    setUsageViewKey(entry.key);
    setUsageViewName(entry.name || "未命名");
    setUsageRows([]);
    setUsageTotal(0);
    setUsagePage(1);
    void loadUsage(entry.key, 1);
  };

  /* ─── column definitions ─── */

  const apiKeyColumns = useMemo<VirtualTableColumn<ApiKeyEntry>[]>(
    () => [
      {
        key: "status",
        label: "状态",
        width: "w-[52px]",
        headerClassName: "text-center",
        cellClassName: "text-center",
        render: (row, idx) => (
          <button
            onClick={() => void handleToggleDisable(idx)}
            title={row.disabled ? "点击启用" : "点击禁用"}
            className={`inline-flex h-7 w-7 items-center justify-center rounded-lg transition-colors ${
              row.disabled
                ? "text-slate-400 hover:bg-red-50 hover:text-red-500 dark:text-white/30 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                : "text-emerald-500 hover:bg-emerald-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
            }`}
          >
            <Power size={15} />
          </button>
        ),
      },
      {
        key: "name",
        label: "名称",
        width: "w-[100px]",
        cellClassName: "font-medium",
        render: (row) => (
          <OverflowTooltip content={row.name || "未命名"} className="block min-w-0">
            <span className="block min-w-0 truncate">
              {row.name || <span className="text-slate-400 dark:text-white/40">未命名</span>}
            </span>
          </OverflowTooltip>
        ),
      },
      {
        key: "key",
        label: "Key",
        cellClassName: "whitespace-nowrap",
        render: (row) => (
          <code className="rounded-md bg-slate-100 px-2 py-0.5 font-mono text-xs text-slate-700 dark:bg-neutral-800 dark:text-white/70">
            {maskKey(row.key)}
          </code>
        ),
      },
      {
        key: "dailyLimit",
        label: "每日限制",
        width: "w-[90px]",
        cellClassName: "whitespace-nowrap text-slate-700 dark:text-white/70",
        render: (row) => (
          <span className="inline-flex items-center gap-1">
            {!row["daily-limit"] ? (
              <>
                <Infinity size={14} className="text-green-500" /> 无限制
              </>
            ) : (
              formatLimit(row["daily-limit"])
            )}
          </span>
        ),
      },
      {
        key: "totalQuota",
        label: "总配额",
        width: "w-[90px]",
        cellClassName: "whitespace-nowrap text-slate-700 dark:text-white/70",
        render: (row) => (
          <span className="inline-flex items-center gap-1">
            {!row["total-quota"] ? (
              <>
                <Infinity size={14} className="text-green-500" /> 无限制
              </>
            ) : (
              formatLimit(row["total-quota"])
            )}
          </span>
        ),
      },
      {
        key: "providerAccess",
        label: "可用供应商",
        width: "w-[190px]",
        cellClassName: "text-slate-700 dark:text-white/70",
        render: (row) => {
          const summaries = describeProviderAccess(row["provider-access"], providerOptions);
          return summaries.length > 0 ? (
            <HoverTooltip content={summaries.join("\n")} className="block min-w-0">
              <span className="inline-flex items-center gap-1.5 text-xs">
                <span className="inline-flex h-5 min-w-[20px] items-center justify-center rounded-md bg-sky-50 px-1.5 font-semibold tabular-nums text-sky-600 dark:bg-sky-900/30 dark:text-sky-300">
                  {summaries.length}
                </span>
                <span className="max-w-[120px] truncate text-slate-500 dark:text-white/50">
                  {summaries[0]}
                  {summaries.length > 1 ? " 等" : ""}
                </span>
              </span>
            </HoverTooltip>
          ) : (
            <span className="inline-flex items-center gap-1 whitespace-nowrap text-green-600 dark:text-green-400">
              <ShieldCheck size={14} /> 全部供应商
            </span>
          );
        },
      },
      {
        key: "allowedModels",
        label: "全局模型",
        width: "w-[160px]",
        cellClassName: "text-slate-700 dark:text-white/70",
        render: (row) =>
          row["allowed-models"]?.length ? (
            <HoverTooltip content={row["allowed-models"].join(", ")} className="block min-w-0">
              <span className="inline-flex items-center gap-1.5 text-xs">
                <span className="inline-flex h-5 min-w-[20px] items-center justify-center rounded-md bg-indigo-50 px-1.5 font-semibold tabular-nums text-indigo-600 dark:bg-indigo-900/30 dark:text-indigo-300">
                  {row["allowed-models"].length}
                </span>
                <span className="max-w-[100px] truncate text-slate-500 dark:text-white/50">
                  {row["allowed-models"][0]}
                  {row["allowed-models"].length > 1 ? " 等" : ""}
                </span>
              </span>
            </HoverTooltip>
          ) : (
            <span className="inline-flex items-center gap-1 whitespace-nowrap text-green-600 dark:text-green-400">
              <ShieldCheck size={14} /> 不限制
            </span>
          ),
      },
      {
        key: "createdAt",
        label: "创建时间",
        width: "w-[140px]",
        cellClassName: "whitespace-nowrap text-slate-500 dark:text-white/50",
        render: (row) => <>{formatDate(row["created-at"])}</>,
      },
      {
        key: "actions",
        label: "操作",
        width: "w-[130px]",
        render: (row, idx) => (
          <div className="flex gap-1">
            <button
              onClick={() => handleViewUsage(row)}
              className="rounded-lg p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-blue-600 dark:text-white/50 dark:hover:bg-neutral-800 dark:hover:text-blue-400"
              title="查看调用情况"
            >
              <BarChart3 size={15} />
            </button>
            <button
              onClick={() => void handleCopy(row.key)}
              className="rounded-lg p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-indigo-600 dark:text-white/50 dark:hover:bg-neutral-800 dark:hover:text-indigo-400"
              title="复制 Key"
            >
              <Copy size={15} />
            </button>
            <button
              onClick={() => handleOpenEdit(idx)}
              className="rounded-lg p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-amber-600 dark:text-white/50 dark:hover:bg-neutral-800 dark:hover:text-amber-400"
              title="编辑"
            >
              <Pencil size={15} />
            </button>
            <button
              onClick={() => setDeleteIndex(idx)}
              className="rounded-lg p-1.5 text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:text-white/50 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              title="删除"
            >
              <Trash2 size={15} />
            </button>
          </div>
        ),
      },
    ],
    [handleToggleDisable, handleViewUsage, handleCopy, handleOpenEdit, providerOptions],
  );

  const usageLogColumns = useMemo<VirtualTableColumn<UsageLogRow>[]>(
    () => [
      {
        key: "timestamp",
        label: "时间",
        width: "w-48",
        cellClassName: "font-mono text-xs tabular-nums text-slate-700 dark:text-slate-200",
        render: (row) => (
          <span className="block min-w-0 truncate">{formatTimestamp(row.timestamp)}</span>
        ),
      },
      {
        key: "model",
        label: "模型",
        width: "w-48",
        render: (row) => (
          <OverflowTooltip content={row.model} className="block min-w-0">
            <span className="block min-w-0 truncate">{row.model}</span>
          </OverflowTooltip>
        ),
      },
      {
        key: "status",
        label: "状态",
        width: "w-16",
        render: (row) =>
          row.failed ? (
            <span className="inline-flex min-w-[44px] justify-center rounded-lg bg-rose-50 px-2 py-1 text-xs font-semibold text-rose-700 dark:bg-rose-500/15 dark:text-rose-200">
              失败
            </span>
          ) : (
            <span className="inline-flex min-w-[44px] justify-center rounded-lg bg-emerald-50 px-2 py-1 text-xs font-semibold text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-200">
              成功
            </span>
          ),
      },
      {
        key: "latency",
        label: "用时",
        width: "w-20",
        headerClassName: "text-right",
        cellClassName:
          "text-right font-mono text-xs tabular-nums text-slate-700 dark:text-slate-200",
        render: (row) => <>{row.latencyText}</>,
      },
      {
        key: "inputTokens",
        label: "输入",
        width: "w-20",
        headerClassName: "text-right",
        cellClassName:
          "text-right font-mono text-xs tabular-nums text-slate-700 dark:text-slate-200",
        render: (row) => <>{row.inputTokens.toLocaleString()}</>,
      },
      {
        key: "outputTokens",
        label: "输出",
        width: "w-20",
        headerClassName: "text-right",
        cellClassName:
          "text-right font-mono text-xs tabular-nums text-slate-700 dark:text-slate-200",
        render: (row) => <>{row.outputTokens.toLocaleString()}</>,
      },
      {
        key: "totalTokens",
        label: "总 Token",
        width: "w-24",
        headerClassName: "text-right",
        cellClassName: "text-right font-mono text-xs tabular-nums text-slate-900 dark:text-white",
        render: (row) => <>{row.totalTokens.toLocaleString()}</>,
      },
    ],
    [],
  );

  const usageHasMore = usageRows.length < usageTotal;

  /* ─── render form ─── */

  const renderForm = () => (
    <div className="space-y-4">
      <div>
        <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-white/80">
          名称 <span className="text-rose-500">*</span>
        </label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
          placeholder="例如：团队A（必填）"
          className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition-all focus:border-indigo-400 focus:ring-2 focus:ring-indigo-400/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white dark:focus:border-indigo-500"
        />
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-white/80">
          API Key
        </label>
        <div className="flex gap-2">
          <input
            type="text"
            value={form.key}
            onChange={(e) => setForm((p) => ({ ...p, key: e.target.value }))}
            placeholder="sk-..."
            className="flex-1 rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none transition-all focus:border-indigo-400 focus:ring-2 focus:ring-indigo-400/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white dark:focus:border-indigo-500"
            readOnly={editIndex !== null}
          />
          {editIndex === null && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setForm((p) => ({ ...p, key: generateKey() }))}
            >
              <RefreshCw size={14} />
              重新生成
            </Button>
          )}
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-white/80">
            每日请求限制
          </label>
          <input
            type="number"
            value={form.dailyLimit}
            onChange={(e) => setForm((p) => ({ ...p, dailyLimit: e.target.value }))}
            placeholder="0 = 无限制"
            min={0}
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition-all focus:border-indigo-400 focus:ring-2 focus:ring-indigo-400/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white dark:focus:border-indigo-500"
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-white/80">
            总请求额度
          </label>
          <input
            type="number"
            value={form.totalQuota}
            onChange={(e) => setForm((p) => ({ ...p, totalQuota: e.target.value }))}
            placeholder="0 = 无限制"
            min={0}
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition-all focus:border-indigo-400 focus:ring-2 focus:ring-indigo-400/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white dark:focus:border-indigo-500"
          />
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-white/80">
          供应商与渠道限制（不选 = 不限制供应商）
        </label>
        <p className="mb-2 text-xs text-slate-500 dark:text-white/50">
          先开启允许访问的供应商类型，再选择该类型下可用的供应商实例；某个类型不选实例时，表示该类型下全部供应商都可用。
        </p>
        <div className="grid gap-3 md:grid-cols-2">
          {providerOptions.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-200 bg-slate-50 px-4 py-5 text-sm text-slate-500 dark:border-neutral-700 dark:bg-neutral-900/40 dark:text-white/45">
              {accessOptionsLoading ? "正在加载供应商选项..." : "当前暂无可配置的供应商选项"}
            </div>
          ) : (
            providerOptions.map((provider) => {
              const selected = providerAccessMap.has(provider.provider);
              const selectedChannels = providerAccessMap.get(provider.provider)?.channels || [];
              return (
                <div
                  key={provider.provider}
                  className={`rounded-2xl border px-4 py-3 transition-colors ${
                    selected
                      ? "border-indigo-200 bg-indigo-50/60 dark:border-indigo-500/30 dark:bg-indigo-500/10"
                      : "border-slate-200 bg-white dark:border-neutral-700 dark:bg-neutral-900"
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="text-sm font-semibold text-slate-900 dark:text-white">
                        {provider.label}
                      </div>
                      <div className="mt-1 text-xs text-slate-500 dark:text-white/45">
                        {provider.provider}
                      </div>
                    </div>
                    <ToggleSwitch
                      checked={selected}
                      onCheckedChange={(checked) =>
                        handleProviderToggle(provider.provider, checked)
                      }
                      ariaLabel={`切换 ${provider.label}`}
                    />
                  </div>

                  {selected ? (
                    provider.channels.length > 0 ? (
                      <div className="mt-3">
                        <div className="mb-1 text-xs text-slate-500 dark:text-white/50">
                          可访问的供应商实例
                        </div>
                        <MultiSelect
                          options={provider.channels.map((channel) => ({
                            value: channel.id,
                            label: channel.label,
                          }))}
                          value={selectedChannels}
                          onChange={(selectedItems) =>
                            handleProviderChannelsChange(provider.provider, selectedItems)
                          }
                          placeholder="选择供应商实例..."
                          emptyLabel="该类型下全部供应商"
                          selectAllLabel="该类型下全部供应商"
                          searchPlaceholder="搜索供应商实例..."
                          emptyResultLabel="无匹配供应商实例"
                        />
                      </div>
                    ) : (
                      <div className="mt-3 rounded-xl bg-white/70 px-3 py-2 text-xs text-slate-500 dark:bg-neutral-950/50 dark:text-white/45">
                        当前未发现可选供应商实例；保持空选择将允许该类型下全部供应商。
                      </div>
                    )
                  ) : (
                    <div className="mt-3 text-xs text-slate-400 dark:text-white/35">
                      关闭后，此 Key 无法访问该供应商类型。
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-white/80">
          全局模型白名单（可选）
        </label>
        <p className="mb-2 text-xs text-slate-500 dark:text-white/50">
          不选表示不做全局模型限制；如果上方选择了具体供应商实例，这里只展示这些实例当前可用的模型并集。
        </p>
        <MultiSelect
          options={modelOptions}
          value={form.allowedModels}
          onChange={handleAllowedModelsChange}
          placeholder="选择模型..."
          emptyLabel="全部模型"
        />
      </div>
    </div>
  );

  /* ─── main render ─── */

  return (
    <div className="space-y-6">
      <Card
        title="API Keys 管理"
        description="创建和管理 API Keys，支持设置请求限额、可访问供应商实例以及全局模型白名单。"
        actions={
          <div className="flex gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void loadPage()}
              disabled={loading}
            >
              <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
              刷新
            </Button>
            <Button variant="primary" size="sm" onClick={handleOpenCreate}>
              <Plus size={14} />
              创建 Key
            </Button>
          </div>
        }
        loading={loading}
      >
        {entries.length === 0 ? (
          <EmptyState
            title="暂无 API Key"
            description="点击「创建 Key」按钮来添加第一个 API Key。"
            icon={<KeyRound size={32} className="text-slate-400" />}
          />
        ) : (
          <VirtualTable<ApiKeyEntry>
            rows={entries}
            columns={apiKeyColumns}
            rowKey={(row) => row.key}
            rowHeight={44}
            height="h-auto max-h-[70vh]"
            minWidth="min-w-[1080px]"
            caption="API Keys 列表"
            emptyText="暂无 API Key"
            rowClassName={(row) => (row.disabled ? "opacity-50" : "")}
          />
        )}
      </Card>

      {/* Create Modal */}
      <Modal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        title="创建 API Key"
        description="填写信息并生成新的 API Key（名称为必填项）"
        footer={
          <>
            <Button variant="secondary" onClick={() => setShowCreate(false)}>
              取消
            </Button>
            <Button variant="primary" onClick={() => void handleCreate()} disabled={saving}>
              {saving ? "创建中..." : "创建"}
            </Button>
          </>
        }
      >
        {renderForm()}
      </Modal>

      {/* Edit Modal */}
      <Modal
        open={editIndex !== null}
        onClose={() => setEditIndex(null)}
        title="编辑 API Key"
        description="修改名称、供应商实例限制和模型白名单"
        footer={
          <>
            <Button variant="secondary" onClick={() => setEditIndex(null)}>
              取消
            </Button>
            <Button variant="primary" onClick={() => void handleEdit()} disabled={saving}>
              {saving ? "保存中..." : "保存"}
            </Button>
          </>
        }
      >
        {renderForm()}
      </Modal>

      {/* Delete Confirm */}
      <Modal
        open={deleteIndex !== null}
        onClose={() => setDeleteIndex(null)}
        title="确认删除"
        description="删除后将无法恢复，使用此 Key 的所有客户端将无法访问。"
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleteIndex(null)}>
              取消
            </Button>
            <Button variant="danger" onClick={() => void handleDelete()} disabled={saving}>
              {saving ? "删除中..." : "确认删除"}
            </Button>
          </>
        }
      >
        {deleteIndex !== null && entries[deleteIndex] && (
          <div className="rounded-xl bg-red-50 p-3 dark:bg-red-900/20">
            <div className="text-sm font-medium text-red-800 dark:text-red-300">
              {entries[deleteIndex].name || "未命名"}
            </div>
            <code className="text-xs text-red-600 dark:text-red-400">
              {maskKey(entries[deleteIndex].key)}
            </code>
          </div>
        )}
      </Modal>

      {/* Usage View — detailed call log table */}
      <Modal
        open={usageViewKey !== null}
        onClose={() => setUsageViewKey(null)}
        title={`调用情况 — ${usageViewName}`}
        description={
          usageViewKey
            ? `Key: ${maskKey(usageViewKey)}  ·  已加载 ${usageRows.length} / ${usageTotal} 条记录`
            : ""
        }
        footer={
          usageViewKey && usageHasMore ? (
            <Button
              variant="secondary"
              onClick={() => void loadUsage(usageViewKey, usagePage + 1)}
              disabled={usageLoading}
            >
              {usageLoading ? "加载中..." : "加载更多"}
            </Button>
          ) : undefined
        }
      >
        {usageLoading ? (
          <div className="py-8 text-center text-sm text-slate-500 dark:text-white/50">
            加载中...
          </div>
        ) : usageRows.length === 0 ? (
          <div className="py-8 text-center text-sm text-slate-500 dark:text-white/50">
            暂无调用记录
          </div>
        ) : (
          <VirtualTable<UsageLogRow>
            rows={usageRows}
            columns={usageLogColumns}
            rowKey={(row) => row.id}
            rowHeight={40}
            height="h-auto max-h-[60vh]"
            minWidth="min-w-[700px]"
            caption="调用记录"
            emptyText="暂无调用记录"
          />
        )}
      </Modal>
    </div>
  );
}
