import type { LucideIcon } from "lucide-react";
import { Plus, RefreshCw } from "lucide-react";
import type { ProviderSimpleConfig } from "@/lib/http/types";
import { Button } from "@/modules/ui/Button";
import { Card } from "@/modules/ui/Card";
import { EmptyState } from "@/modules/ui/EmptyState";
import { ProviderCardActionBar } from "@/modules/providers/ProviderCardActionBar";
import { ProviderStatusBar } from "@/modules/providers/ProviderStatusBar";
import type { KeyStatBucket, StatusBarData } from "@/modules/providers/provider-usage";
import {
  hasDisableAllModelsRule,
  maskApiKey,
  stripDisableAllModelsRule,
} from "@/modules/providers/providers-helpers";

export function ProviderKeyListCard({
  icon: Icon,
  title,
  description,
  loading = false,
  items,
  onAdd,
  onEdit,
  onDelete,
  onToggleEnabled,
  getStats,
  getStatusBar,
  getAutoSyncState,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  loading?: boolean;
  items: ProviderSimpleConfig[];
  onAdd: () => void;
  onEdit: (index: number) => void;
  onDelete: (index: number) => void;
  onToggleEnabled?: (index: number, enabled: boolean) => void;
  onCopy?: (index: number) => void;
  getStats: (item: ProviderSimpleConfig) => KeyStatBucket;
  getStatusBar: (item: ProviderSimpleConfig) => StatusBarData;
  getAutoSyncState?: (
    item: ProviderSimpleConfig,
  ) => { enabled: boolean; supported: boolean; label: string; title: string };
}) {
  return (
    <Card
      title={title}
      description={description}
      loading={loading}
      actions={
        <Button variant="primary" size="sm" onClick={onAdd} disabled={loading}>
          <Plus size={14} />
          新增
        </Button>
      }
    >
      {items.length === 0 ? (
        <EmptyState title="暂无配置" description="点击“新增”创建第一条配置。" />
      ) : (
        <div className="space-y-3">
          {items.map((item, idx) => {
            const disabled = hasDisableAllModelsRule(item.excludedModels);
            const excludedModels = stripDisableAllModelsRule(item.excludedModels);
            const models = Array.isArray(item.models)
              ? item.models.filter((model): model is NonNullable<(typeof item.models)[number]> =>
                  Boolean(model && typeof model === "object"),
                )
              : [];
            const stats = getStats(item);
            const statusData = getStatusBar(item);
            const participateInDefaultRouting = item.participateInDefaultRouting !== false;
            const autoSyncState = getAutoSyncState?.(item);

            return (
              <div
                key={`${item.apiKey}:${idx}`}
                className="relative rounded-2xl border border-slate-200 bg-white/70 px-4 py-3 shadow-sm dark:border-neutral-800 dark:bg-neutral-950/60"
              >
                <div className="min-w-0">
                  <div className={onToggleEnabled ? "md:pr-[320px]" : "md:pr-[220px]"}>
                    <p className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-white">
                      <Icon size={16} className="text-slate-900 dark:text-white" />
                      <span className="truncate">{item.name || maskApiKey(item.apiKey)}</span>
                      <span
                        className={[
                          "rounded-full px-2 py-0.5 text-[11px] font-medium",
                          participateInDefaultRouting
                            ? "bg-emerald-600/10 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-200"
                            : "bg-amber-500/10 text-amber-700 dark:bg-amber-500/15 dark:text-amber-200",
                        ].join(" ")}
                      >
                        {participateInDefaultRouting ? "参与默认路由" : "仅显式路由"}
                      </span>
                      {autoSyncState?.enabled ? (
                        <span
                          title={autoSyncState.title}
                          className={[
                            "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium",
                            autoSyncState.supported
                              ? "bg-sky-600/10 text-sky-700 dark:bg-sky-500/15 dark:text-sky-200"
                              : "bg-amber-500/10 text-amber-700 dark:bg-amber-500/15 dark:text-amber-200",
                          ].join(" ")}
                        >
                          <RefreshCw size={12} />
                          <span>{autoSyncState.label}</span>
                        </span>
                      ) : null}
                    </p>

                    <div className="mt-1 space-y-1 text-xs text-slate-600 dark:text-white/65">
                      <p className="truncate font-mono">apiKey：{maskApiKey(item.apiKey)}</p>
                      <p className="truncate font-mono">baseUrl：{item.baseUrl || "--"}</p>
                      {item.proxyUrl ? (
                        <p className="truncate font-mono">proxyUrl：{item.proxyUrl}</p>
                      ) : null}
                      <p className="tabular-nums">
                        models：{models.length} · excluded：{excludedModels.length} · 成功：
                        {stats.success} · 失败：{stats.failure}
                      </p>
                    </div>
                  </div>

                  {models.length ? (
                    <div className="mt-2 flex flex-wrap gap-1">
                      {models.map((model) => (
                        <span
                          key={model.name}
                          className="rounded-full bg-slate-900 px-2 py-0.5 text-[11px] text-white dark:bg-white dark:text-neutral-950"
                          title={
                            model.alias && model.alias !== model.name
                              ? `${model.name} => ${model.alias}`
                              : model.name
                          }
                        >
                          {model.alias && model.alias !== model.name
                            ? `${model.name} → ${model.alias}`
                            : model.name}
                        </span>
                      ))}
                    </div>
                  ) : null}

                  {excludedModels.length ? (
                    <div className="mt-2 flex flex-wrap gap-1">
                      {excludedModels.map((model) => (
                        <span
                          key={model}
                          className="rounded-full bg-rose-600/10 px-2 py-0.5 text-[11px] text-rose-700 dark:bg-rose-500/15 dark:text-rose-200"
                        >
                          {model}
                        </span>
                      ))}
                    </div>
                  ) : null}

                  <ProviderStatusBar data={statusData} />
                </div>
                <ProviderCardActionBar
                  enabled={!disabled}
                  showToggle={Boolean(onToggleEnabled)}
                  onToggle={
                    onToggleEnabled ? (enabled) => onToggleEnabled(idx, enabled) : undefined
                  }
                  onEdit={() => onEdit(idx)}
                  onDelete={() => onDelete(idx)}
                />
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
