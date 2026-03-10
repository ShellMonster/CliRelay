import type {
  MonitorChannelStatsResponse,
  MonitorFailureStatsResponse,
  MonitorDailyTrendResponse,
  MonitorHourlyResponse,
  MonitorModelDistributionResponse,
  MonitorSummaryResponse,
} from "@/lib/http/apis/usage";

export interface MonitorUsageData {
  summary?: MonitorSummaryResponse["summary"];
  monitor?: {
    modelDistribution: MonitorModelDistributionResponse["items"];
    dailyTrend: MonitorDailyTrendResponse["items"];
    hourly: MonitorHourlyResponse["items"];
    channelStats?: MonitorChannelStatsResponse;
    failureStats?: MonitorFailureStatsResponse;
  };
}
