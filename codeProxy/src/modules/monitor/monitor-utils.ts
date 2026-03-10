export const formatNumber = (value: number): string => {
  return new Intl.NumberFormat("zh-CN").format(Math.round(value));
};

export const formatRate = (value: number): string => {
  return `${value.toFixed(2)}%`;
};
