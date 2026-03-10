import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { MonitorUsageData } from "@/modules/monitor/types";
import styles from "@/pages/MonitorPage.module.scss";

interface KpiCardsProps {
  data: MonitorUsageData | null;
  loading: boolean;
  timeRange: number;
}

// 格式化数字
function formatNumber(num: number): string {
  if (num >= 1000000000) {
    return (num / 1000000000).toFixed(2) + "B";
  }
  if (num >= 1000000) {
    return (num / 1000000).toFixed(2) + "M";
  }
  if (num >= 1000) {
    return (num / 1000).toFixed(2) + "K";
  }
  return num.toLocaleString();
}

export function KpiCards({ data, loading, timeRange }: KpiCardsProps) {
  const { t } = useTranslation();

  // 计算统计数据
  const stats = useMemo(() => {
    if (data?.summary) {
      const summary = data.summary;
      const avgRpd = timeRange > 0 ? Math.round(summary.TotalRequests / timeRange) : 0;
      const avgRpm = timeRange > 0 ? Number((summary.TotalRequests / (timeRange * 24 * 60)).toFixed(1)) : 0;
      const avgTpm = timeRange > 0 ? Math.round(summary.TotalTokens / (timeRange * 24 * 60)) : 0;
      return {
        totalRequests: summary.TotalRequests,
        successRequests: summary.SuccessRequests,
        failedRequests: summary.FailedRequests,
        successRate: summary.SuccessRate,
        totalTokens: summary.TotalTokens,
        inputTokens: summary.InputTokens,
        outputTokens: summary.OutputTokens,
        reasoningTokens: summary.ReasoningTokens,
        cachedTokens: summary.CachedTokens,
        avgTpm,
        avgRpm,
        avgRpd,
      };
    }

    return {
      totalRequests: 0,
      successRequests: 0,
      failedRequests: 0,
      successRate: 0,
      totalTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      reasoningTokens: 0,
      cachedTokens: 0,
      avgTpm: 0,
      avgRpm: 0,
      avgRpd: 0,
    };
  }, [data, timeRange]);

  const timeRangeLabel =
    timeRange === 1 ? t("monitor.today") : t("monitor.last_n_days", { n: timeRange });

  return (
    <div className={styles.kpiGrid}>
      {/* 请求数 */}
      <div className={styles.kpiCard}>
        <div className={styles.kpiTitle}>
          <span className={styles.kpiLabel}>{t("monitor.kpi.requests")}</span>
          <span className={styles.kpiTag}>{timeRangeLabel}</span>
        </div>
        <div className={styles.kpiValue}>{loading ? "--" : formatNumber(stats.totalRequests)}</div>
        <div className={styles.kpiMeta}>
          <span className={styles.kpiSuccess}>
            {t("monitor.kpi.success")}: {loading ? "--" : stats.successRequests.toLocaleString()}
          </span>
          <span className={styles.kpiFailure}>
            {t("monitor.kpi.failed")}: {loading ? "--" : stats.failedRequests.toLocaleString()}
          </span>
          <span>
            {t("monitor.kpi.rate")}: {loading ? "--" : stats.successRate.toFixed(1)}%
          </span>
        </div>
      </div>

      {/* Tokens */}
      <div className={`${styles.kpiCard} ${styles.green}`}>
        <div className={styles.kpiTitle}>
          <span className={styles.kpiLabel}>{t("monitor.kpi.tokens")}</span>
          <span className={styles.kpiTag}>{timeRangeLabel}</span>
        </div>
        <div className={styles.kpiValue}>{loading ? "--" : formatNumber(stats.totalTokens)}</div>
        <div className={styles.kpiMeta}>
          <span>
            {t("monitor.kpi.input")}: {loading ? "--" : formatNumber(stats.inputTokens)}
          </span>
          <span>
            {t("monitor.kpi.output")}: {loading ? "--" : formatNumber(stats.outputTokens)}
          </span>
        </div>
      </div>

      {/* 平均 TPM */}
      <div className={`${styles.kpiCard} ${styles.purple}`}>
        <div className={styles.kpiTitle}>
          <span className={styles.kpiLabel}>{t("monitor.kpi.avg_tpm")}</span>
          <span className={styles.kpiTag}>{timeRangeLabel}</span>
        </div>
        <div className={styles.kpiValue}>{loading ? "--" : formatNumber(stats.avgTpm)}</div>
        <div className={styles.kpiMeta}>
          <span>{t("monitor.kpi.tokens_per_minute")}</span>
        </div>
      </div>

      {/* 平均 RPM */}
      <div className={`${styles.kpiCard} ${styles.orange}`}>
        <div className={styles.kpiTitle}>
          <span className={styles.kpiLabel}>{t("monitor.kpi.avg_rpm")}</span>
          <span className={styles.kpiTag}>{timeRangeLabel}</span>
        </div>
        <div className={styles.kpiValue}>{loading ? "--" : stats.avgRpm.toFixed(1)}</div>
        <div className={styles.kpiMeta}>
          <span>{t("monitor.kpi.requests_per_minute")}</span>
        </div>
      </div>

      {/* 日均 RPD */}
      <div className={`${styles.kpiCard} ${styles.cyan}`}>
        <div className={styles.kpiTitle}>
          <span className={styles.kpiLabel}>{t("monitor.kpi.avg_rpd")}</span>
          <span className={styles.kpiTag}>{timeRangeLabel}</span>
        </div>
        <div className={styles.kpiValue}>{loading ? "--" : formatNumber(stats.avgRpd)}</div>
        <div className={styles.kpiMeta}>
          <span>{t("monitor.kpi.requests_per_day")}</span>
        </div>
      </div>
    </div>
  );
}
