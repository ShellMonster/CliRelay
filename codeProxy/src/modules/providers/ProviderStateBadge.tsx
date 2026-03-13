export function ProviderStateBadge({
  enabled,
  enabledText = "已启用",
  disabledText = "已禁用",
  minWidthClassName = "min-w-[64px]",
}: {
  enabled: boolean;
  enabledText?: string;
  disabledText?: string;
  minWidthClassName?: string;
}) {
  return (
    <span
      className={[
        "inline-flex items-center justify-center rounded-full px-2.5 py-1 text-xs font-semibold",
        minWidthClassName,
        enabled
          ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-200"
          : "bg-amber-500/15 text-amber-700 dark:text-amber-200",
      ].join(" ")}
    >
      {enabled ? enabledText : disabledText}
    </span>
  );
}
