import { useMemo, useState, useCallback, Fragment } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { useDisableModel } from "@/hooks";
import { TimeRangeSelector, formatTimeRangeCaption, type TimeRange } from "./TimeRangeSelector";
import { DisableModelModal } from "./DisableModelModal";
import { formatTimestamp, getRateClassName, getProviderDisplayParts, type DateRange } from "@/utils/monitor";
import type { MonitorUsageData } from "@/modules/monitor/types";
import styles from "@/pages/MonitorPage.module.scss";

interface ChannelStatsProps {
  data: MonitorUsageData | null;
  loading: boolean;
  providerMap: Record<string, string>;
  providerModels: Record<string, Set<string>>;
}

interface ModelStat {
  requests: number;
  success: number;
  failed: number;
  successRate: number;
  recentRequests: { failed: boolean; timestamp: number }[];
  lastTimestamp: number;
}

interface ChannelStat {
  source: string;
  displayName: string;
  providerName: string | null;
  maskedKey: string;
  totalRequests: number;
  successRequests: number;
  failedRequests: number;
  successRate: number;
  lastRequestTime: number;
  recentRequests: { failed: boolean; timestamp: number }[];
  models: Record<string, ModelStat>;
}

export function ChannelStats({ data, loading, providerMap, providerModels }: ChannelStatsProps) {
  const { t } = useTranslation();
  const [expandedChannel, setExpandedChannel] = useState<string | null>(null);
  const [filterChannel, setFilterChannel] = useState("");
  const [filterModel, setFilterModel] = useState("");
  const [filterStatus, setFilterStatus] = useState<"" | "success" | "failed">("");

  // 时间范围状态
  const [timeRange, setTimeRange] = useState<TimeRange>(7);
  const [customRange, setCustomRange] = useState<DateRange | undefined>();

  // 使用禁用模型 Hook
  const {
    disableState,
    disabling,
    isModelDisabled,
    handleDisableClick: onDisableClick,
    handleConfirmDisable,
    handleCancelDisable,
  } = useDisableModel({ providerMap, providerModels });

  // 处理时间范围变化
  const handleTimeRangeChange = useCallback((range: TimeRange, custom?: DateRange) => {
    setTimeRange(range);
    if (custom) {
      setCustomRange(custom);
    }
  }, []);

  // 计算渠道统计数据
  const channelStats = useMemo(() => {
    const monitorStats = data?.monitor?.channelStats;
    if (!monitorStats) return [];

    const stats: Record<string, ChannelStat> = {};

    monitorStats.channels.forEach((channel) => {
      const { provider, masked } = getProviderDisplayParts(channel.source, providerMap);
      if (!provider) return;
      const displayName = `${provider} (${masked})`;
      stats[displayName] = {
        source: channel.source,
        displayName,
        providerName: provider,
        maskedKey: masked,
        totalRequests: channel.requests,
        successRequests: channel.success_requests,
        failedRequests: channel.failed_requests,
        successRate: channel.success_rate,
        lastRequestTime: channel.last_request_at ? new Date(channel.last_request_at).getTime() : 0,
        recentRequests: [],
        models: {},
      };
    });

    monitorStats.models.forEach((modelPoint) => {
      const { provider, masked } = getProviderDisplayParts(modelPoint.source, providerMap);
      if (!provider) return;
      const displayName = `${provider} (${masked})`;
      const channel = stats[displayName];
      if (!channel) return;
      channel.models[modelPoint.model] = {
        requests: modelPoint.requests,
        success: modelPoint.success_requests,
        failed: modelPoint.failed_requests,
        successRate: modelPoint.success_rate,
        recentRequests: [],
        lastTimestamp: modelPoint.last_request_at ? new Date(modelPoint.last_request_at).getTime() : 0,
      };
    });

    return Object.values(stats)
      .filter((stat) => stat.totalRequests > 0)
      .sort((a, b) => b.totalRequests - a.totalRequests)
      .slice(0, 10);
  }, [data?.monitor?.channelStats, providerMap]);

  // 获取所有渠道和模型列表
  const { channels, models } = useMemo(() => {
    const channelSet = new Set<string>();
    const modelSet = new Set<string>();

    channelStats.forEach((stat) => {
      channelSet.add(stat.displayName);
      Object.keys(stat.models).forEach((model) => modelSet.add(model));
    });

    return {
      channels: Array.from(channelSet).sort(),
      models: Array.from(modelSet).sort(),
    };
  }, [channelStats]);

  // 过滤后的数据
  const filteredStats = useMemo(() => {
    return channelStats.filter((stat) => {
      if (filterChannel && stat.displayName !== filterChannel) return false;
      if (filterModel && !stat.models[filterModel]) return false;
      if (filterStatus === "success" && stat.failedRequests > 0) return false;
      if (filterStatus === "failed" && stat.failedRequests === 0) return false;
      return true;
    });
  }, [channelStats, filterChannel, filterModel, filterStatus]);

  // 切换展开状态
  const toggleExpand = (displayName: string) => {
    setExpandedChannel(expandedChannel === displayName ? null : displayName);
  };

  // 开始禁用流程（阻止事件冒泡）
  const handleDisableClick = (source: string, model: string, e: React.MouseEvent) => {
    e.stopPropagation();
    onDisableClick(source, model);
  };

  return (
    <>
      <Card
        title={t("monitor.channel.title")}
        subtitle={
          <span>
            {formatTimeRangeCaption(timeRange, customRange, t)} · {t("monitor.channel.subtitle")}
            <span style={{ color: "var(--text-tertiary)" }}>
              {" "}
              · {t("monitor.channel.click_hint")}
            </span>
          </span>
        }
        extra={
          <TimeRangeSelector
            value={timeRange}
            onChange={handleTimeRangeChange}
            customRange={customRange}
          />
        }
      >
        {/* 筛选器 */}
        <div className={styles.logFilters}>
          <select
            className={styles.logSelect}
            value={filterChannel}
            onChange={(e) => setFilterChannel(e.target.value)}
          >
            <option value="">{t("monitor.channel.all_channels")}</option>
            {channels.map((channel) => (
              <option key={channel} value={channel}>
                {channel}
              </option>
            ))}
          </select>
          <select
            className={styles.logSelect}
            value={filterModel}
            onChange={(e) => setFilterModel(e.target.value)}
          >
            <option value="">{t("monitor.channel.all_models")}</option>
            {models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
          <select
            className={styles.logSelect}
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value as "" | "success" | "failed")}
          >
            <option value="">{t("monitor.channel.all_status")}</option>
            <option value="success">{t("monitor.channel.only_success")}</option>
            <option value="failed">{t("monitor.channel.only_failed")}</option>
          </select>
        </div>

        {/* 表格 */}
        <div className={styles.tableWrapper}>
          {loading ? (
            <div className={styles.emptyState}>{t("common.loading")}</div>
          ) : filteredStats.length === 0 ? (
            <div className={styles.emptyState}>{t("monitor.no_data")}</div>
          ) : (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>{t("monitor.channel.header_name")}</th>
                  <th>{t("monitor.channel.header_count")}</th>
                  <th>{t("monitor.channel.header_rate")}</th>
                  <th>{t("monitor.channel.header_recent")}</th>
                  <th>{t("monitor.channel.header_time")}</th>
                </tr>
              </thead>
              <tbody>
                {filteredStats.map((stat) => (
                  <Fragment key={stat.displayName}>
                    <tr
                      className={styles.expandable}
                      onClick={() => toggleExpand(stat.displayName)}
                    >
                      <td>
                        {stat.providerName ? (
                          <>
                            <span className={styles.channelName}>{stat.providerName}</span>
                            <span className={styles.channelSecret}> ({stat.maskedKey})</span>
                          </>
                        ) : (
                          stat.maskedKey
                        )}
                      </td>
                      <td>{stat.totalRequests.toLocaleString()}</td>
                      <td className={getRateClassName(stat.successRate, styles)}>
                        {stat.successRate.toFixed(1)}%
                      </td>
                      <td>
                        <div className={styles.statusBars}>
                          {stat.recentRequests.map((req, i) => (
                            <div
                              key={i}
                              className={`${styles.statusBar} ${req.failed ? styles.failure : styles.success}`}
                            />
                          ))}
                        </div>
                      </td>
                      <td>{formatTimestamp(stat.lastRequestTime)}</td>
                    </tr>
                    {expandedChannel === stat.displayName && (
                      <tr key={`${stat.displayName}-detail`}>
                        <td colSpan={5} className={styles.expandDetail}>
                          <div className={styles.expandTableWrapper}>
                            <table className={styles.table}>
                              <thead>
                                <tr>
                                  <th>{t("monitor.channel.model")}</th>
                                  <th>{t("monitor.channel.header_count")}</th>
                                  <th>{t("monitor.channel.header_rate")}</th>
                                  <th>
                                    {t("monitor.channel.success")}/{t("monitor.channel.failed")}
                                  </th>
                                  <th>{t("monitor.channel.header_recent")}</th>
                                  <th>{t("monitor.channel.header_time")}</th>
                                  <th>{t("monitor.logs.header_actions")}</th>
                                </tr>
                              </thead>
                              <tbody>
                                {Object.entries(stat.models)
                                  .sort((a, b) => {
                                    const aDisabled = isModelDisabled(stat.source, a[0]);
                                    const bDisabled = isModelDisabled(stat.source, b[0]);
                                    // 已禁用的排在后面
                                    if (aDisabled !== bDisabled) {
                                      return aDisabled ? 1 : -1;
                                    }
                                    // 然后按请求数降序
                                    return b[1].requests - a[1].requests;
                                  })
                                  .map(([modelName, modelStat]) => {
                                    const disabled = isModelDisabled(stat.source, modelName);
                                    return (
                                      <tr
                                        key={modelName}
                                        className={disabled ? styles.modelDisabled : ""}
                                      >
                                        <td>{modelName}</td>
                                        <td>{modelStat.requests.toLocaleString()}</td>
                                        <td
                                          className={getRateClassName(
                                            modelStat.successRate,
                                            styles,
                                          )}
                                        >
                                          {modelStat.successRate.toFixed(1)}%
                                        </td>
                                        <td>
                                          <span className={styles.kpiSuccess}>
                                            {modelStat.success}
                                          </span>
                                          {" / "}
                                          <span className={styles.kpiFailure}>
                                            {modelStat.failed}
                                          </span>
                                        </td>
                                        <td>
                                          <div className={styles.statusBars}>
                                            {modelStat.recentRequests.map((req, i) => (
                                              <div
                                                key={i}
                                                className={`${styles.statusBar} ${req.failed ? styles.failure : styles.success}`}
                                              />
                                            ))}
                                          </div>
                                        </td>
                                        <td>{formatTimestamp(modelStat.lastTimestamp)}</td>
                                        <td>
                                          {disabled ? (
                                            <span className={styles.disabledLabel}>
                                              {t("monitor.logs.removed")}
                                            </span>
                                          ) : stat.source &&
                                            stat.source !== "-" &&
                                            stat.source !== "unknown" ? (
                                            <button
                                              className={styles.disableBtn}
                                              onClick={(e) =>
                                                handleDisableClick(stat.source, modelName, e)
                                              }
                                            >
                                              {t("monitor.logs.disable")}
                                            </button>
                                          ) : (
                                            "-"
                                          )}
                                        </td>
                                      </tr>
                                    );
                                  })}
                              </tbody>
                            </table>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </Card>

      {/* 禁用确认弹窗 */}
      <DisableModelModal
        disableState={disableState}
        disabling={disabling}
        onConfirm={handleConfirmDisable}
        onCancel={handleCancelDisable}
      />
    </>
  );
}
