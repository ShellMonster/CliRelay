import { Settings2, Trash2 } from "lucide-react";
import { Button } from "@/modules/ui/Button";
import { ToggleSwitch } from "@/modules/ui/ToggleSwitch";
import { ProviderStateBadge } from "@/modules/providers/ProviderStateBadge";

export function ProviderCardActionBar({
  enabled,
  showToggle = true,
  onToggle,
  onEdit,
  onDelete,
  className,
}: {
  enabled: boolean;
  showToggle?: boolean;
  onToggle?: (enabled: boolean) => void;
  onEdit?: () => void;
  onDelete?: () => void;
  className?: string;
}) {
  return (
    <div
      className={[
        "mt-3 flex flex-wrap items-center gap-2 md:absolute md:right-4 md:top-3 md:mt-0 md:flex-nowrap md:justify-end",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <ProviderStateBadge enabled={enabled} />
      {showToggle ? (
        <div className="inline-flex shrink-0 items-center gap-2">
          <span className="text-sm font-semibold leading-none text-slate-900 dark:text-white">
            启用
          </span>
          <ToggleSwitch
            checked={enabled}
            ariaLabel="启用"
            onCheckedChange={(next) => onToggle?.(next)}
          />
        </div>
      ) : null}
      {onEdit ? (
        <Button variant="secondary" size="sm" onClick={onEdit}>
          <Settings2 size={14} />
          编辑
        </Button>
      ) : null}
      {onDelete ? (
        <Button variant="danger" size="sm" onClick={onDelete}>
          <Trash2 size={14} />
          删除
        </Button>
      ) : null}
    </div>
  );
}
