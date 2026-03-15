import { useCallback, useEffect, useMemo, useState } from "react";
import { Check, ChevronDown, Copy, Plus, Trash2 } from "lucide-react";
import { configApi, type UserAgentRoutingProviderOption } from "@/lib/http/apis/config";
import type {
  PayloadFilterRule,
  PayloadModelEntry,
  PayloadParamEntry,
  PayloadParamValueType,
  PayloadProtocol,
  PayloadRule,
  RoutingStrategy,
  UserAgentRoutingMatchMode,
  UserAgentRoutingRuleEntry,
  VisualConfigValues,
} from "@/modules/config/visual/types";
import { makeClientId } from "@/modules/config/visual/types";
import {
  VISUAL_CONFIG_PAYLOAD_VALUE_TYPE_OPTIONS,
  VISUAL_CONFIG_PROTOCOL_OPTIONS,
} from "@/modules/config/visual/useVisualConfig";
import { Button } from "@/modules/ui/Button";
import { Card } from "@/modules/ui/Card";
import { ConfirmModal } from "@/modules/ui/ConfirmModal";
import { TextInput } from "@/modules/ui/Input";
import { Modal } from "@/modules/ui/Modal";
import { MultiSelect, type MultiSelectOption } from "@/modules/ui/MultiSelect";
import { ToggleSwitch } from "@/modules/ui/ToggleSwitch";
import { useToast } from "@/modules/ui/ToastProvider";

const isValidApiKeyCharset = (key: string): boolean => /^[\x21-\x7E]+$/.test(key);

const maskApiKey = (value: string): string => {
  const trimmed = value.trim();
  if (!trimmed) return "--";
  if (trimmed.length <= 10) return `${trimmed.slice(0, 2)}***${trimmed.slice(-2)}`;
  return `${trimmed.slice(0, 6)}***${trimmed.slice(-4)}`;
};

const USER_AGENT_ROUTING_MATCH_MODE_OPTIONS = [
  { value: "contains", label: "contains（包含）" },
  { value: "regex", label: "regex（正则）" },
] satisfies ReadonlyArray<{ value: UserAgentRoutingMatchMode; label: string }>;

const createEmptyUserAgentRoutingRule = (): UserAgentRoutingRuleEntry => ({
  id: makeClientId(),
  name: "",
  enabled: true,
  matchMode: "contains",
  pattern: "",
  models: [],
  forceProviders: [],
  preferProviders: [],
  forceChannels: [],
  preferChannels: [],
});

const normalizeUserAgentRoutingModels = (values: string[]): string[] =>
  values
    .map((value) => value.trim())
    .filter(Boolean)
    .filter(
      (value, index, arr) =>
        arr.findIndex((item) => item.toLowerCase() === value.toLowerCase()) === index,
    );

const normalizeUserAgentRoutingProviders = (values: string[]): string[] =>
  values
    .map((value) => value.trim().toLowerCase())
    .filter(Boolean)
    .filter((value, index, arr) => arr.indexOf(value) === index);

const normalizeUserAgentRoutingExactValues = (values: string[]): string[] =>
  values
    .map((value) => value.trim())
    .filter(Boolean)
    .filter((value, index, arr) => arr.indexOf(value) === index);

const summarizeUserAgentRoutingValues = (values: string[], emptyLabel: string): string => {
  if (!values.length) return emptyLabel;
  if (values.length <= 2) return values.join(", ");
  return `${values.slice(0, 2).join(", ")} +${values.length - 2}`;
};

const mergeUserAgentRoutingOptions = (
  baseOptions: MultiSelectOption[],
  selectedValues: string[],
): MultiSelectOption[] => {
  const optionMap = new Map<string, MultiSelectOption>();
  baseOptions.forEach((option) => {
    optionMap.set(option.value.toLowerCase(), option);
  });
  selectedValues.forEach((value) => {
    const trimmed = value.trim();
    if (!trimmed) return;
    const key = trimmed.toLowerCase();
    if (optionMap.has(key)) return;
    optionMap.set(key, { value: trimmed, label: trimmed });
  });
  return Array.from(optionMap.values()).sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
};

const mergeUserAgentRoutingExactOptions = (
  baseOptions: MultiSelectOption[],
  selectedValues: string[],
): MultiSelectOption[] => {
  const optionMap = new Map<string, MultiSelectOption>();
  baseOptions.forEach((option) => {
    optionMap.set(option.value, option);
  });
  selectedValues.forEach((value) => {
    const trimmed = value.trim();
    if (!trimmed || optionMap.has(trimmed)) return;
    optionMap.set(trimmed, { value: trimmed, label: trimmed });
  });
  return Array.from(optionMap.values()).sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
};

const collectUserAgentRoutingChannelIDs = (
  providerIDs: string[],
  providerOptionMap: Map<string, UserAgentRoutingProviderOption>,
): Set<string> => {
  const channelIDs = new Set<string>();
  providerIDs.forEach((providerID) => {
    providerOptionMap.get(providerID)?.channels.forEach((channel) => {
      const channelID = channel.id.trim();
      if (channelID) channelIDs.add(channelID);
    });
  });
  return channelIDs;
};

const filterUserAgentRoutingChannelsByProviders = (
  channels: string[],
  providerIDs: string[],
  providerOptionMap: Map<string, UserAgentRoutingProviderOption>,
  preserveWithoutProviders: boolean,
): string[] => {
  const normalizedChannels = normalizeUserAgentRoutingExactValues(channels);
  const normalizedProviders = normalizeUserAgentRoutingProviders(providerIDs);
  if (normalizedProviders.length === 0) {
    return preserveWithoutProviders ? normalizedChannels : [];
  }
  const allowedChannelIDs = collectUserAgentRoutingChannelIDs(
    normalizedProviders,
    providerOptionMap,
  );
  if (allowedChannelIDs.size === 0) {
    return [];
  }
  return normalizedChannels.filter((channelID) => allowedChannelIDs.has(channelID));
};

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <div className="text-sm font-semibold text-slate-900 dark:text-white">{label}</div>
      {hint ? <div className="text-xs text-slate-600 dark:text-white/65">{hint}</div> : null}
      <div className="pt-1">{children}</div>
    </div>
  );
}

import { Select } from "@/modules/ui/Select";

function SelectInput({
  value,
  onChange,
  options,
  disabled,
  ariaLabel,
}: {
  value: string;
  onChange: (value: string) => void;
  options: ReadonlyArray<{ value: string; label: string }>;
  disabled?: boolean;
  ariaLabel: string;
}) {
  return (
    <Select
      value={value}
      onChange={onChange}
      options={options.map((opt) => ({ value: opt.value, label: opt.label }))}
      aria-label={ariaLabel}
      className={disabled ? "pointer-events-none opacity-60" : undefined}
    />
  );
}

function TextArea({
  value,
  onChange,
  disabled,
  placeholder,
  ariaLabel,
  rows = 4,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  ariaLabel: string;
  rows?: number;
}) {
  return (
    <textarea
      value={value}
      onChange={(e) => onChange(e.currentTarget.value)}
      disabled={disabled}
      placeholder={placeholder}
      aria-label={ariaLabel}
      rows={rows}
      spellCheck={false}
      className={[
        "w-full resize-y rounded-xl border border-slate-200 bg-white px-3 py-2.5 font-mono text-xs text-slate-900 outline-none transition",
        "focus-visible:ring-2 focus-visible:ring-slate-400/35 dark:border-neutral-800 dark:bg-neutral-900 dark:text-slate-100 dark:focus-visible:ring-white/15",
        disabled ? "opacity-60" : null,
      ]
        .filter(Boolean)
        .join(" ")}
    />
  );
}

function ApiKeysEditor({
  value,
  disabled,
  onChange,
}: {
  value: string;
  disabled?: boolean;
  onChange: (nextValue: string) => void;
}) {
  const { notify } = useToast();
  const apiKeys = useMemo(
    () =>
      value
        .split("\n")
        .map((key) => key.trim())
        .filter(Boolean),
    [value],
  );

  const [modalOpen, setModalOpen] = useState(false);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [inputValue, setInputValue] = useState("");
  const [formError, setFormError] = useState("");
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null);

  const openAddModal = () => {
    setEditingIndex(null);
    setInputValue("");
    setFormError("");
    setModalOpen(true);
  };

  const openEditModal = (index: number) => {
    setEditingIndex(index);
    setInputValue(apiKeys[index] ?? "");
    setFormError("");
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setInputValue("");
    setEditingIndex(null);
    setFormError("");
  };

  const updateApiKeys = (nextKeys: string[]) => {
    onChange(nextKeys.join("\n"));
  };

  const handleDelete = (index: number) => {
    updateApiKeys(apiKeys.filter((_, i) => i !== index));
    setDeleteIndex(null);
  };

  const handleSave = () => {
    const trimmed = inputValue.trim();
    if (!trimmed) {
      setFormError("API Key 不能为空");
      return;
    }
    if (!isValidApiKeyCharset(trimmed)) {
      setFormError("API Key 仅允许可见 ASCII 字符（不含空格/中文）");
      return;
    }

    const nextKeys =
      editingIndex === null
        ? [...apiKeys, trimmed]
        : apiKeys.map((key, idx) => (idx === editingIndex ? trimmed : key));
    updateApiKeys(nextKeys);
    closeModal();
  };

  const handleCopy = async (apiKey: string) => {
    try {
      if (!navigator.clipboard?.writeText) {
        notify({ type: "error", message: "当前浏览器不支持一键复制" });
        return;
      }
      await navigator.clipboard.writeText(apiKey);
      notify({ type: "success", message: "已复制" });
    } catch {
      notify({ type: "error", message: "复制失败" });
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm font-semibold text-slate-900 dark:text-white">API Keys</div>
        <Button size="sm" onClick={openAddModal} disabled={disabled}>
          <Plus size={14} />
          新增
        </Button>
      </div>

      {apiKeys.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-4 text-center text-sm text-slate-600 dark:border-neutral-800 dark:bg-neutral-950/40 dark:text-white/65">
          暂无 API Key
        </div>
      ) : (
        <div className="space-y-2">
          {apiKeys.map((key, index) => (
            <div
              key={`${key}-${index}`}
              className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-slate-200 bg-white/60 px-4 py-3 dark:border-neutral-800 dark:bg-neutral-950/40"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="inline-flex rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700 dark:bg-white/10 dark:text-white/80">
                    #{index + 1}
                  </span>
                  <span className="text-sm font-semibold text-slate-900 dark:text-white">
                    API Key
                  </span>
                </div>
                <div className="mt-1 truncate font-mono text-xs text-slate-600 dark:text-white/65">
                  {maskApiKey(key)}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => void handleCopy(key)}
                  disabled={disabled}
                >
                  <Copy size={14} />
                  复制
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => openEditModal(index)}
                  disabled={disabled}
                >
                  编辑
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => setDeleteIndex(index)}
                  disabled={disabled}
                >
                  <Trash2 size={14} />
                  删除
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <p className="text-xs text-slate-600 dark:text-white/65">
        对应 `config.yaml` 中 `api-keys` 配置（每行一个）。
      </p>

      <Modal
        open={modalOpen}
        onClose={closeModal}
        title={editingIndex !== null ? "编辑 API Key" : "新增 API Key"}
        footer={
          <>
            <Button variant="secondary" onClick={closeModal} disabled={disabled}>
              取消
            </Button>
            <Button onClick={handleSave} disabled={disabled}>
              <Check size={14} />
              {editingIndex !== null ? "更新" : "添加"}
            </Button>
          </>
        }
      >
        <Field label="API Key" hint="仅允许可见 ASCII 字符（不含空格）。">
          <TextInput
            value={inputValue}
            onChange={(e) => setInputValue(e.currentTarget.value)}
            placeholder="sk-..."
            disabled={disabled}
          />
          {formError ? (
            <p className="mt-2 text-sm text-rose-600 dark:text-rose-300">{formError}</p>
          ) : null}
        </Field>
      </Modal>

      <ConfirmModal
        open={deleteIndex !== null}
        title="删除 API Key"
        description="确定要删除该 API Key 吗？此操作不可恢复。"
        confirmText="删除"
        variant="danger"
        onClose={() => setDeleteIndex(null)}
        onConfirm={() => {
          if (deleteIndex === null) return;
          handleDelete(deleteIndex);
        }}
      />
    </div>
  );
}

function updateRuleModels(
  rules: PayloadRule[],
  ruleIndex: number,
  updater: (models: PayloadModelEntry[]) => PayloadModelEntry[],
): PayloadRule[] {
  return rules.map((rule, idx) =>
    idx === ruleIndex ? { ...rule, models: updater(rule.models) } : rule,
  );
}

function updateRuleParams(
  rules: PayloadRule[],
  ruleIndex: number,
  updater: (params: PayloadParamEntry[]) => PayloadParamEntry[],
): PayloadRule[] {
  return rules.map((rule, idx) =>
    idx === ruleIndex ? { ...rule, params: updater(rule.params) } : rule,
  );
}

function PayloadRulesEditor({
  title,
  description,
  rules,
  disabled,
  onChange,
}: {
  title: string;
  description?: string;
  rules: PayloadRule[];
  disabled?: boolean;
  onChange: (rules: PayloadRule[]) => void;
}) {
  const addRule = () => {
    const next: PayloadRule = {
      id: makeClientId(),
      models: [{ id: makeClientId(), name: "", protocol: undefined }],
      params: [],
    };
    onChange([...(rules || []), next]);
  };

  const removeRule = (index: number) => {
    onChange((rules || []).filter((_, i) => i !== index));
  };

  const addModel = (ruleIndex: number) => {
    onChange(
      updateRuleModels(rules, ruleIndex, (models) => [
        ...models,
        { id: makeClientId(), name: "", protocol: undefined },
      ]),
    );
  };

  const removeModel = (ruleIndex: number, modelIndex: number) => {
    onChange(
      updateRuleModels(rules, ruleIndex, (models) => models.filter((_, i) => i !== modelIndex)),
    );
  };

  const updateModel = (
    ruleIndex: number,
    modelIndex: number,
    patch: Partial<PayloadModelEntry>,
  ) => {
    onChange(
      updateRuleModels(rules, ruleIndex, (models) =>
        models.map((m, i) => (i === modelIndex ? { ...m, ...patch } : m)),
      ),
    );
  };

  const addParam = (ruleIndex: number) => {
    const next: PayloadParamEntry = {
      id: makeClientId(),
      path: "",
      valueType: "string",
      value: "",
    };
    onChange(updateRuleParams(rules, ruleIndex, (params) => [...params, next]));
  };

  const removeParam = (ruleIndex: number, paramIndex: number) => {
    onChange(
      updateRuleParams(rules, ruleIndex, (params) => params.filter((_, i) => i !== paramIndex)),
    );
  };

  const updateParam = (
    ruleIndex: number,
    paramIndex: number,
    patch: Partial<PayloadParamEntry>,
  ) => {
    onChange(
      updateRuleParams(rules, ruleIndex, (params) =>
        params.map((p, i) => (i === paramIndex ? { ...p, ...patch } : p)),
      ),
    );
  };

  const getValuePlaceholder = (valueType: PayloadParamValueType) => {
    if (valueType === "number") return "例如：1";
    if (valueType === "boolean") return "true / false";
    if (valueType === "json") return '例如：{"a":1}';
    return "例如：hello";
  };

  return (
    <Card
      title={title}
      description={description}
      actions={
        <Button size="sm" onClick={addRule} disabled={disabled}>
          <Plus size={14} />
          新增规则
        </Button>
      }
    >
      {rules.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-4 text-center text-sm text-slate-600 dark:border-neutral-800 dark:bg-neutral-950/40 dark:text-white/65">
          暂无规则
        </div>
      ) : (
        <div className="space-y-4">
          {rules.map((rule, ruleIndex) => (
            <div
              key={rule.id}
              className="space-y-3 rounded-2xl border border-slate-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-950/40"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-sm font-semibold text-slate-900 dark:text-white">
                  规则 {ruleIndex + 1}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeRule(ruleIndex)}
                  disabled={disabled}
                >
                  <Trash2 size={14} />
                  删除规则
                </Button>
              </div>

              <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="text-xs font-semibold text-slate-600 dark:text-white/65">
                    匹配模型
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => addModel(ruleIndex)}
                    disabled={disabled}
                  >
                    <Plus size={14} />
                    新增模型
                  </Button>
                </div>

                <div className="space-y-2">
                  {(rule.models || []).map((model, modelIndex) => (
                    <div key={model.id} className="grid gap-2 lg:grid-cols-[1fr_180px_auto]">
                      <TextInput
                        value={model.name}
                        onChange={(e) =>
                          updateModel(ruleIndex, modelIndex, {
                            name: e.currentTarget.value,
                          })
                        }
                        placeholder="model 名称"
                        disabled={disabled}
                      />
                      <SelectInput
                        value={(model.protocol ?? "") as string}
                        onChange={(value) =>
                          updateModel(ruleIndex, modelIndex, {
                            protocol: (value || undefined) as PayloadProtocol | undefined,
                          })
                        }
                        options={VISUAL_CONFIG_PROTOCOL_OPTIONS}
                        disabled={disabled}
                        ariaLabel="协议"
                      />
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => removeModel(ruleIndex, modelIndex)}
                        disabled={disabled || (rule.models || []).length <= 1}
                      >
                        <Trash2 size={14} />
                        删除
                      </Button>
                    </div>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="text-xs font-semibold text-slate-600 dark:text-white/65">
                    覆盖参数
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => addParam(ruleIndex)}
                    disabled={disabled}
                  >
                    <Plus size={14} />
                    新增参数
                  </Button>
                </div>

                {(rule.params || []).length === 0 ? (
                  <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-3 text-center text-xs text-slate-600 dark:border-neutral-800 dark:bg-neutral-950/40 dark:text-white/65">
                    暂无参数（仅按模型匹配）
                  </div>
                ) : (
                  <div className="space-y-2">
                    {(rule.params || []).map((param, paramIndex) => (
                      <div
                        key={param.id}
                        className="space-y-2 rounded-2xl border border-slate-200 bg-white/60 p-3 dark:border-neutral-800 dark:bg-neutral-950/40"
                      >
                        <div className="grid gap-2 lg:grid-cols-[1fr_180px_auto]">
                          <TextInput
                            value={param.path}
                            onChange={(e) =>
                              updateParam(ruleIndex, paramIndex, {
                                path: e.currentTarget.value,
                              })
                            }
                            placeholder="参数路径，例如：headers.Authorization"
                            disabled={disabled}
                          />
                          <SelectInput
                            value={param.valueType}
                            onChange={(value) =>
                              updateParam(ruleIndex, paramIndex, {
                                valueType: value as PayloadParamValueType,
                              })
                            }
                            options={VISUAL_CONFIG_PAYLOAD_VALUE_TYPE_OPTIONS}
                            disabled={disabled}
                            ariaLabel="值类型"
                          />
                          <Button
                            variant="danger"
                            size="sm"
                            onClick={() => removeParam(ruleIndex, paramIndex)}
                            disabled={disabled}
                          >
                            <Trash2 size={14} />
                            删除
                          </Button>
                        </div>

                        {param.valueType === "json" ? (
                          <TextArea
                            value={param.value}
                            onChange={(value) => updateParam(ruleIndex, paramIndex, { value })}
                            placeholder={getValuePlaceholder(param.valueType)}
                            disabled={disabled}
                            ariaLabel="JSON 值"
                            rows={6}
                          />
                        ) : (
                          <TextInput
                            value={param.value}
                            onChange={(e) =>
                              updateParam(ruleIndex, paramIndex, {
                                value: e.currentTarget.value,
                              })
                            }
                            placeholder={getValuePlaceholder(param.valueType)}
                            disabled={disabled}
                          />
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function PayloadFilterRulesEditor({
  rules,
  disabled,
  onChange,
}: {
  rules: PayloadFilterRule[];
  disabled?: boolean;
  onChange: (rules: PayloadFilterRule[]) => void;
}) {
  const addRule = () => {
    const next: PayloadFilterRule = {
      id: makeClientId(),
      models: [{ id: makeClientId(), name: "", protocol: undefined }],
      params: [],
    };
    onChange([...(rules || []), next]);
  };

  const removeRule = (index: number) => {
    onChange((rules || []).filter((_, i) => i !== index));
  };

  const updateRule = (index: number, patch: Partial<PayloadFilterRule>) => {
    onChange((rules || []).map((rule, i) => (i === index ? { ...rule, ...patch } : rule)));
  };

  const addModel = (ruleIndex: number) => {
    const rule = rules[ruleIndex];
    updateRule(ruleIndex, {
      models: [...rule.models, { id: makeClientId(), name: "", protocol: undefined }],
    });
  };

  const removeModel = (ruleIndex: number, modelIndex: number) => {
    const rule = rules[ruleIndex];
    updateRule(ruleIndex, {
      models: rule.models.filter((_, i) => i !== modelIndex),
    });
  };

  const updateModel = (
    ruleIndex: number,
    modelIndex: number,
    patch: Partial<PayloadModelEntry>,
  ) => {
    const rule = rules[ruleIndex];
    updateRule(ruleIndex, {
      models: rule.models.map((m, i) => (i === modelIndex ? { ...m, ...patch } : m)),
    });
  };

  const addParam = (ruleIndex: number) => {
    const rule = rules[ruleIndex];
    updateRule(ruleIndex, { params: [...(rule.params || []), ""] });
  };

  const removeParam = (ruleIndex: number, paramIndex: number) => {
    const rule = rules[ruleIndex];
    updateRule(ruleIndex, {
      params: (rule.params || []).filter((_, i) => i !== paramIndex),
    });
  };

  const updateParam = (ruleIndex: number, paramIndex: number, nextValue: string) => {
    const rule = rules[ruleIndex];
    updateRule(ruleIndex, {
      params: (rule.params || []).map((p, i) => (i === paramIndex ? nextValue : p)),
    });
  };

  return (
    <Card
      title="Payload 过滤规则"
      description="匹配模型后，从请求 payload 中移除指定参数路径列表（对应 `payload.filter`）。"
      actions={
        <Button size="sm" onClick={addRule} disabled={disabled}>
          <Plus size={14} />
          新增规则
        </Button>
      }
    >
      {rules.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-4 text-center text-sm text-slate-600 dark:border-neutral-800 dark:bg-neutral-950/40 dark:text-white/65">
          暂无规则
        </div>
      ) : (
        <div className="space-y-4">
          {rules.map((rule, ruleIndex) => (
            <div
              key={rule.id}
              className="space-y-3 rounded-2xl border border-slate-200 bg-white/60 p-4 dark:border-neutral-800 dark:bg-neutral-950/40"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-sm font-semibold text-slate-900 dark:text-white">
                  规则 {ruleIndex + 1}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeRule(ruleIndex)}
                  disabled={disabled}
                >
                  <Trash2 size={14} />
                  删除规则
                </Button>
              </div>

              <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="text-xs font-semibold text-slate-600 dark:text-white/65">
                    匹配模型
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => addModel(ruleIndex)}
                    disabled={disabled}
                  >
                    <Plus size={14} />
                    新增模型
                  </Button>
                </div>

                <div className="space-y-2">
                  {(rule.models || []).map((model, modelIndex) => (
                    <div key={model.id} className="grid gap-2 lg:grid-cols-[1fr_180px_auto]">
                      <TextInput
                        value={model.name}
                        onChange={(e) =>
                          updateModel(ruleIndex, modelIndex, {
                            name: e.currentTarget.value,
                          })
                        }
                        placeholder="model 名称"
                        disabled={disabled}
                      />
                      <SelectInput
                        value={(model.protocol ?? "") as string}
                        onChange={(value) =>
                          updateModel(ruleIndex, modelIndex, {
                            protocol: (value || undefined) as PayloadProtocol | undefined,
                          })
                        }
                        options={VISUAL_CONFIG_PROTOCOL_OPTIONS}
                        disabled={disabled}
                        ariaLabel="协议"
                      />
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => removeModel(ruleIndex, modelIndex)}
                        disabled={disabled || (rule.models || []).length <= 1}
                      >
                        <Trash2 size={14} />
                        删除
                      </Button>
                    </div>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="text-xs font-semibold text-slate-600 dark:text-white/65">
                    移除参数路径
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => addParam(ruleIndex)}
                    disabled={disabled}
                  >
                    <Plus size={14} />
                    新增路径
                  </Button>
                </div>

                {(rule.params || []).length === 0 ? (
                  <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-3 text-center text-xs text-slate-600 dark:border-neutral-800 dark:bg-neutral-950/40 dark:text-white/65">
                    暂无路径
                  </div>
                ) : (
                  <div className="space-y-2">
                    {(rule.params || []).map((param, paramIndex) => (
                      <div
                        key={`${rule.id}-p-${paramIndex}`}
                        className="grid gap-2 lg:grid-cols-[1fr_auto]"
                      >
                        <TextInput
                          value={param}
                          onChange={(e) =>
                            updateParam(ruleIndex, paramIndex, e.currentTarget.value)
                          }
                          placeholder="例如：messages.0.content"
                          disabled={disabled}
                        />
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => removeParam(ruleIndex, paramIndex)}
                          disabled={disabled}
                        >
                          <Trash2 size={14} />
                          删除
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function UserAgentRoutingRulesEditor({
  rules,
  disabled,
  onChange,
}: {
  rules: UserAgentRoutingRuleEntry[];
  disabled?: boolean;
  onChange: (rules: UserAgentRoutingRuleEntry[]) => void;
}) {
  const { notify } = useToast();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null);
  const [formError, setFormError] = useState("");
  const [draft, setDraft] = useState<UserAgentRoutingRuleEntry>(createEmptyUserAgentRoutingRule());
  const [optionsLoading, setOptionsLoading] = useState(false);
  const [providerOptions, setProviderOptions] = useState<UserAgentRoutingProviderOption[]>([]);
  const [modelOptions, setModelOptions] = useState<MultiSelectOption[]>([]);

  useEffect(() => {
    let active = true;
    setOptionsLoading(true);
    void configApi
      .getUserAgentRoutingOptions()
      .then((data) => {
        if (!active) return;
        setProviderOptions(data.providers);
        setModelOptions(
          data.models.map((option) => ({
            value: option.id,
            label: option.label,
          })),
        );
      })
      .catch((error: unknown) => {
        if (!active) return;
        notify({
          type: "error",
          message: error instanceof Error ? error.message : "加载 UA 路由规则选项失败",
        });
      })
      .finally(() => {
        if (!active) return;
        setOptionsLoading(false);
      });

    return () => {
      active = false;
    };
  }, [notify]);

  const providerOptionMap = useMemo(() => {
    const optionMap = new Map<string, UserAgentRoutingProviderOption>();
    providerOptions.forEach((option) => {
      optionMap.set(option.id, option);
    });
    return optionMap;
  }, [providerOptions]);

  const providerSelectOptions = useMemo(
    () =>
      mergeUserAgentRoutingOptions(
        providerOptions.map((option) => ({
          value: option.id,
          label: option.label,
        })),
        [...draft.forceProviders, ...draft.preferProviders],
      ),
    [draft.forceProviders, draft.preferProviders, providerOptions],
  );

  const updateForceProviders = useCallback(
    (forceProviders: string[]) => {
      const nextForceProviders = normalizeUserAgentRoutingProviders(forceProviders);
      setDraft((prev) => ({
        ...prev,
        forceProviders: nextForceProviders,
        forceChannels: filterUserAgentRoutingChannelsByProviders(
          prev.forceChannels,
          nextForceProviders,
          providerOptionMap,
          false,
        ),
      }));
    },
    [providerOptionMap],
  );

  const updatePreferProviders = useCallback(
    (preferProviders: string[]) => {
      const nextPreferProviders = normalizeUserAgentRoutingProviders(preferProviders);
      setDraft((prev) => ({
        ...prev,
        preferProviders: nextPreferProviders,
        preferChannels: filterUserAgentRoutingChannelsByProviders(
          prev.preferChannels,
          nextPreferProviders,
          providerOptionMap,
          false,
        ),
      }));
    },
    [providerOptionMap],
  );

  const forceChannelSelectOptions = useMemo(() => {
    const channelOptions: MultiSelectOption[] = [];
    draft.forceProviders.forEach((providerID) => {
      const provider = providerOptionMap.get(providerID);
      provider?.channels.forEach((channel) => {
        channelOptions.push({
          value: channel.id,
          label: channel.label,
        });
      });
    });
    return mergeUserAgentRoutingExactOptions(channelOptions, draft.forceChannels);
  }, [draft.forceChannels, draft.forceProviders, providerOptionMap]);

  const preferChannelSelectOptions = useMemo(() => {
    const channelOptions: MultiSelectOption[] = [];
    draft.preferProviders.forEach((providerID) => {
      const provider = providerOptionMap.get(providerID);
      provider?.channels.forEach((channel) => {
        channelOptions.push({
          value: channel.id,
          label: channel.label,
        });
      });
    });
    return mergeUserAgentRoutingExactOptions(channelOptions, draft.preferChannels);
  }, [draft.preferChannels, draft.preferProviders, providerOptionMap]);

  const forceChannelsDisabled =
    disabled ||
    optionsLoading ||
    (draft.forceProviders.length === 0 && draft.forceChannels.length === 0);
  const preferChannelsDisabled =
    disabled ||
    optionsLoading ||
    (draft.preferProviders.length === 0 && draft.preferChannels.length === 0);

  const modelSelectOptions = useMemo(
    () => mergeUserAgentRoutingOptions(modelOptions, draft.models),
    [draft.models, modelOptions],
  );

  const forceChannelsHint =
    draft.forceProviders.length > 0
      ? "命中后只保留这些 provider 类型下的指定供应商。"
      : "先选择 force-providers，再选择该类型下的实际供应商。";

  const preferChannelsHint =
    draft.preferProviders.length > 0
      ? "命中后优先尝试这些 provider 类型下的指定供应商；其他同类型供应商仍可回退。"
      : "先选择 prefer-providers，再选择该类型下的实际供应商。";

  const openAddModal = () => {
    setEditingIndex(null);
    setDraft(createEmptyUserAgentRoutingRule());
    setFormError("");
    setModalOpen(true);
  };

  const openEditModal = (index: number) => {
    const current = rules[index];
    if (!current) return;
    setEditingIndex(index);
    setDraft({
      ...current,
      models: [...(current.models || [])],
      forceProviders: [...(current.forceProviders || [])],
      preferProviders: [...(current.preferProviders || [])],
      forceChannels: [...(current.forceChannels || [])],
      preferChannels: [...(current.preferChannels || [])],
    });
    setFormError("");
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setEditingIndex(null);
    setDraft(createEmptyUserAgentRoutingRule());
    setFormError("");
  };

  const removeRule = (index: number) => {
    onChange((rules || []).filter((_, i) => i !== index));
    setDeleteIndex(null);
    notify({
      type: "info",
      message: "规则已从当前草稿移除，仍需点击页面顶部“保存”写入 config.yaml",
    });
  };

  const saveRule = () => {
    const pattern = draft.pattern.trim();
    if (!pattern) {
      setFormError("pattern 不能为空");
      return;
    }

    const forceProviders = normalizeUserAgentRoutingProviders(draft.forceProviders);
    const preferProviders = normalizeUserAgentRoutingProviders(draft.preferProviders);

    const nextRule: UserAgentRoutingRuleEntry = {
      ...draft,
      name: draft.name.trim(),
      pattern,
      models: normalizeUserAgentRoutingModels(draft.models),
      forceProviders,
      preferProviders,
      forceChannels: filterUserAgentRoutingChannelsByProviders(
        draft.forceChannels,
        forceProviders,
        providerOptionMap,
        true,
      ),
      preferChannels: filterUserAgentRoutingChannelsByProviders(
        draft.preferChannels,
        preferProviders,
        providerOptionMap,
        true,
      ),
    };

    const nextRules =
      editingIndex === null
        ? [...(rules || []), nextRule]
        : (rules || []).map((rule, index) => (index === editingIndex ? nextRule : rule));
    onChange(nextRules);
    closeModal();
    notify({
      type: "info",
      message:
        editingIndex === null
          ? "规则已加入当前草稿，仍需点击页面顶部“保存”写入 config.yaml"
          : "规则修改已加入当前草稿，仍需点击页面顶部“保存”写入 config.yaml",
    });
  };

  return (
    <Card
      title="UA 路由规则"
      description="首个命中的规则生效。可仅按 UA 匹配，也可同时限定模型；`force/prefer-providers` 控制 provider 类型，`force/prefer-channels` 控制该类型下的实际供应商。"
      actions={
        <Button size="sm" onClick={openAddModal} disabled={disabled}>
          <Plus size={14} />
          新增规则
        </Button>
      }
    >
      <p className="mb-3 text-xs font-medium text-amber-700 dark:text-amber-300">
        弹窗里的“添加到草稿 / 更新草稿”只会修改当前页面草稿；还需要点击页面顶部“保存”才能真正写入
        config.yaml。
      </p>
      {rules.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-4 text-center text-sm text-slate-600 dark:border-neutral-800 dark:bg-neutral-950/40 dark:text-white/65">
          暂无 UA 路由规则
        </div>
      ) : (
        <div className="space-y-3">
          {rules.map((rule, index) => (
            <div
              key={rule.id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-slate-200 bg-white/60 px-4 py-3 dark:border-neutral-800 dark:bg-neutral-950/40"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="inline-flex rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700 dark:bg-white/10 dark:text-white/80">
                    #{index + 1}
                  </span>
                  <span className="text-sm font-semibold text-slate-900 dark:text-white">
                    {rule.name.trim() || `规则 ${index + 1}`}
                  </span>
                  <span
                    className={[
                      "inline-flex rounded-full px-2 py-0.5 text-xs font-medium",
                      rule.enabled
                        ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300"
                        : "bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-white/45",
                    ].join(" ")}
                  >
                    {rule.enabled ? "已启用" : "已禁用"}
                  </span>
                </div>

                <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-600 dark:text-white/65">
                  <span>
                    UA：{rule.matchMode} / {rule.pattern || "--"}
                  </span>
                  <span>
                    模型：{summarizeUserAgentRoutingValues(rule.models || [], "全部模型")}
                  </span>
                  <span>
                    强制：{summarizeUserAgentRoutingValues(rule.forceProviders || [], "未设置")}
                  </span>
                  <span>
                    优先：{summarizeUserAgentRoutingValues(rule.preferProviders || [], "未设置")}
                  </span>
                  <span>
                    强制供应商：
                    {summarizeUserAgentRoutingValues(rule.forceChannels || [], "未设置")}
                  </span>
                  <span>
                    优先供应商：
                    {summarizeUserAgentRoutingValues(rule.preferChannels || [], "未设置")}
                  </span>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => openEditModal(index)}
                  disabled={disabled}
                >
                  编辑
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => setDeleteIndex(index)}
                  disabled={disabled}
                >
                  <Trash2 size={14} />
                  删除
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Modal
        open={modalOpen}
        onClose={closeModal}
        title={editingIndex === null ? "新增 UA 路由规则" : "编辑 UA 路由规则"}
        footer={
          <>
            <Button variant="secondary" onClick={closeModal} disabled={disabled}>
              取消
            </Button>
            <Button onClick={saveRule} disabled={disabled}>
              <Check size={14} />
              {editingIndex === null ? "添加到草稿" : "更新草稿"}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid gap-3 lg:grid-cols-2">
            <Field label="name" hint="仅用于识别规则来源。">
              <TextInput
                value={draft.name}
                onChange={(e) => {
                  const value = e.currentTarget.value;
                  setDraft((prev) => ({ ...prev, name: value }));
                }}
                placeholder="例如：OpenCode 定向"
                disabled={disabled}
              />
            </Field>
            <Field label="match-mode" hint="contains 大多数场景更稳。">
              <SelectInput
                value={draft.matchMode}
                onChange={(value) =>
                  setDraft((prev) => ({
                    ...prev,
                    matchMode: value as UserAgentRoutingMatchMode,
                  }))
                }
                options={USER_AGENT_ROUTING_MATCH_MODE_OPTIONS}
                disabled={disabled}
                ariaLabel="user-agent match mode"
              />
            </Field>
          </div>

          <Field label="enabled" hint="关闭后保留规则但不参与匹配。">
            <ToggleSwitch
              label={draft.enabled ? "已启用" : "已禁用"}
              checked={draft.enabled}
              onCheckedChange={(next) => setDraft((prev) => ({ ...prev, enabled: next }))}
              disabled={disabled}
            />
          </Field>

          <Field label="pattern" hint="匹配 User-Agent；contains 忽略大小写，regex 使用原始正则。">
            <TextInput
              value={draft.pattern}
              onChange={(e) => {
                const value = e.currentTarget.value;
                setDraft((prev) => ({ ...prev, pattern: value }));
              }}
              placeholder="例如：opencode 或 ^opencode/"
              disabled={disabled}
            />
            {formError ? (
              <p className="mt-2 text-sm text-rose-600 dark:text-rose-300">{formError}</p>
            ) : null}
          </Field>

          <Field label="models" hint="可选。为空表示只按 UA 匹配；选择后需要 UA 与模型同时命中。">
            <MultiSelect
              options={modelSelectOptions}
              value={draft.models}
              onChange={(models) => setDraft((prev) => ({ ...prev, models }))}
              placeholder="选择模型"
              emptyLabel="全部模型"
              searchPlaceholder="搜索模型..."
              selectAllLabel="全部模型"
              emptyResultLabel="无匹配模型"
              disabled={disabled || optionsLoading}
            />
            <p className="mt-2 text-xs text-slate-600 dark:text-white/65">
              {optionsLoading
                ? "正在加载模型选项..."
                : `共 ${modelSelectOptions.length} 个可选模型`}
            </p>
          </Field>

          <div className="grid gap-3 lg:grid-cols-2">
            <Field label="force-providers" hint="命中后只保留这些 provider。">
              <MultiSelect
                options={providerSelectOptions}
                value={draft.forceProviders}
                onChange={updateForceProviders}
                placeholder="选择 provider"
                emptyLabel="未设置"
                searchPlaceholder="搜索 provider..."
                selectAllLabel="清空选择"
                emptyResultLabel="无匹配 provider"
                disabled={disabled || optionsLoading}
              />
            </Field>
            <Field
              label="prefer-providers"
              hint="命中后优先尝试这些 provider；未命中的 provider 仍可回退。"
            >
              <MultiSelect
                options={providerSelectOptions}
                value={draft.preferProviders}
                onChange={updatePreferProviders}
                placeholder="选择 provider"
                emptyLabel="未设置"
                searchPlaceholder="搜索 provider..."
                selectAllLabel="清空选择"
                emptyResultLabel="无匹配 provider"
                disabled={disabled || optionsLoading}
              />
            </Field>
          </div>

          <div className="grid gap-3 lg:grid-cols-2">
            <Field label="force-channels" hint={forceChannelsHint}>
              <MultiSelect
                options={forceChannelSelectOptions}
                value={draft.forceChannels}
                onChange={(forceChannels) => setDraft((prev) => ({ ...prev, forceChannels }))}
                placeholder="选择实际供应商"
                emptyLabel="未设置"
                searchPlaceholder="搜索实际供应商..."
                selectAllLabel="清空选择"
                emptyResultLabel="无匹配供应商"
                disabled={forceChannelsDisabled}
              />
            </Field>
            <Field label="prefer-channels" hint={preferChannelsHint}>
              <MultiSelect
                options={preferChannelSelectOptions}
                value={draft.preferChannels}
                onChange={(preferChannels) => setDraft((prev) => ({ ...prev, preferChannels }))}
                placeholder="选择实际供应商"
                emptyLabel="未设置"
                searchPlaceholder="搜索实际供应商..."
                selectAllLabel="清空选择"
                emptyResultLabel="无匹配供应商"
                disabled={preferChannelsDisabled}
              />
            </Field>
          </div>

          <p className="text-xs text-slate-600 dark:text-white/65">
            Provider、实际供应商和模型选项基于当前已加载的认证与模型注册表生成。
          </p>
          <p className="text-xs font-medium text-amber-700 dark:text-amber-300">
            当前弹窗只会修改页面草稿；关闭弹窗后，还需要点击页面顶部“保存”才能真正写入 config.yaml。
          </p>
        </div>
      </Modal>

      <ConfirmModal
        open={deleteIndex !== null}
        title="删除 UA 路由规则"
        description="确定要删除这条 UA 路由规则吗？此操作不可恢复。"
        confirmText="删除"
        variant="danger"
        onClose={() => setDeleteIndex(null)}
        onConfirm={() => {
          if (deleteIndex === null) return;
          removeRule(deleteIndex);
        }}
      />
    </Card>
  );
}

export function VisualConfigEditor({
  values,
  disabled,
  onChange,
}: {
  values: VisualConfigValues;
  disabled?: boolean;
  onChange: (values: Partial<VisualConfigValues>) => void;
}) {
  const update = useCallback(
    (patch: Partial<VisualConfigValues>) => {
      onChange(patch);
    },
    [onChange],
  );

  const routingOptions = useMemo(
    () =>
      [
        { value: "round-robin", label: "round-robin（轮询）" },
        { value: "fill-first", label: "fill-first（优先填满）" },
      ] satisfies ReadonlyArray<{ value: RoutingStrategy; label: string }>,
    [],
  );

  return (
    <div className="space-y-6">
      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="基础" description="主机/端口、认证目录与 API Keys。">
          <div className="space-y-4">
            <div className="grid gap-3 lg:grid-cols-2">
              <Field label="host" hint="为空表示使用服务端默认。">
                <TextInput
                  value={values.host}
                  onChange={(e) => update({ host: e.currentTarget.value })}
                  placeholder="0.0.0.0"
                  disabled={disabled}
                />
              </Field>
              <Field label="port" hint="非负整数。">
                <TextInput
                  value={values.port}
                  onChange={(e) => update({ port: e.currentTarget.value })}
                  placeholder="8080"
                  inputMode="numeric"
                  disabled={disabled}
                />
              </Field>
            </div>

            <Field label="auth-dir" hint="认证文件目录路径。">
              <TextInput
                value={values.authDir}
                onChange={(e) => update({ authDir: e.currentTarget.value })}
                placeholder="./auth"
                disabled={disabled}
              />
            </Field>

            <div className="rounded-xl border border-indigo-200 bg-indigo-50/50 px-4 py-3 dark:border-indigo-800 dark:bg-indigo-950/30">
              <p className="text-sm text-indigo-800 dark:text-indigo-300">
                API Keys 已迁移至专属管理页面。
                <a
                  href="#/api-keys"
                  className="ml-1 font-semibold underline underline-offset-2 hover:text-indigo-600 dark:hover:text-indigo-200"
                >
                  前往 API Keys 管理 →
                </a>
              </p>
            </div>
          </div>
        </Card>

        <Card title="TLS" description="启用 TLS 并配置证书路径。">
          <div className="space-y-4">
            <ToggleSwitch
              label="启用 TLS"
              description="开启后使用 tls.cert / tls.key。"
              checked={values.tlsEnable}
              onCheckedChange={(next) => update({ tlsEnable: next })}
              disabled={disabled}
            />
            <div className="grid gap-3">
              <Field label="tls.cert" hint="证书文件路径。">
                <TextInput
                  value={values.tlsCert}
                  onChange={(e) => update({ tlsCert: e.currentTarget.value })}
                  placeholder="./cert.pem"
                  disabled={disabled}
                />
              </Field>
              <Field label="tls.key" hint="私钥文件路径。">
                <TextInput
                  value={values.tlsKey}
                  onChange={(e) => update({ tlsKey: e.currentTarget.value })}
                  placeholder="./key.pem"
                  disabled={disabled}
                />
              </Field>
            </div>
          </div>
        </Card>
      </div>

      <Card title="远程管理" description="对应 `remote-management` 配置段。">
        <div className="grid gap-5 lg:grid-cols-2">
          <div className="space-y-4">
            <ToggleSwitch
              label="允许远程访问"
              description="remote-management.allow-remote"
              checked={values.rmAllowRemote}
              onCheckedChange={(next) => update({ rmAllowRemote: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="禁用控制面板"
              description="remote-management.disable-control-panel"
              checked={values.rmDisableControlPanel}
              onCheckedChange={(next) => update({ rmDisableControlPanel: next })}
              disabled={disabled}
            />
          </div>
          <div className="space-y-4">
            <Field label="secret-key" hint="远程管理密钥（请妥善保管）。">
              <TextInput
                value={values.rmSecretKey}
                onChange={(e) => update({ rmSecretKey: e.currentTarget.value })}
                placeholder="******"
                disabled={disabled}
              />
            </Field>
            <Field label="panel-github-repository" hint="面板仓库地址（如需）。">
              <TextInput
                value={values.rmPanelRepo}
                onChange={(e) => update({ rmPanelRepo: e.currentTarget.value })}
                placeholder="owner/repo"
                disabled={disabled}
              />
            </Field>
          </div>
        </div>
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="开关" description="常用运行开关（写入 config.yaml）。">
          <div className="space-y-4">
            <ToggleSwitch
              label="Debug"
              description="debug"
              checked={values.debug}
              onCheckedChange={(next) => update({ debug: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="商业模式"
              description="commercial-mode（切换后通常需要重启服务）"
              checked={values.commercialMode}
              onCheckedChange={(next) => update({ commercialMode: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="写入日志文件"
              description="logging-to-file"
              checked={values.loggingToFile}
              onCheckedChange={(next) => update({ loggingToFile: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="使用统计"
              description="usage-statistics-enabled"
              checked={values.usageStatisticsEnabled}
              onCheckedChange={(next) => update({ usageStatisticsEnabled: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="存储请求/响应内容"
              description="usage-log-content-enabled"
              checked={values.usageLogContentEnabled}
              onCheckedChange={(next) => update({ usageLogContentEnabled: next })}
              disabled={disabled}
            />
          </div>
        </Card>

        <Card title="代理与重试" description="proxy-url、request-retry、max-retry-interval。">
          <div className="space-y-4">
            <Field label="proxy-url" hint="为空表示不使用代理。">
              <TextInput
                value={values.proxyUrl}
                onChange={(e) => update({ proxyUrl: e.currentTarget.value })}
                placeholder="http://127.0.0.1:7890"
                disabled={disabled}
              />
            </Field>
            <div className="grid gap-3 lg:grid-cols-2">
              <Field label="request-retry" hint="非负整数。">
                <TextInput
                  value={values.requestRetry}
                  onChange={(e) => update({ requestRetry: e.currentTarget.value })}
                  placeholder="0"
                  inputMode="numeric"
                  disabled={disabled}
                />
              </Field>
              <Field label="max-retry-interval" hint="非负整数（秒）。">
                <TextInput
                  value={values.maxRetryInterval}
                  onChange={(e) => update({ maxRetryInterval: e.currentTarget.value })}
                  placeholder="0"
                  inputMode="numeric"
                  disabled={disabled}
                />
              </Field>
            </div>
            <ToggleSwitch
              label="强制模型前缀"
              description="force-model-prefix"
              checked={values.forceModelPrefix}
              onCheckedChange={(next) => update({ forceModelPrefix: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="WebSocket 鉴权"
              description="ws-auth"
              checked={values.wsAuth}
              onCheckedChange={(next) => update({ wsAuth: next })}
              disabled={disabled}
            />
          </div>
        </Card>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="日志限制" description="logs-max-total-size-mb。">
          <div className="space-y-4">
            <Field label="logs-max-total-size-mb" hint="日志总大小上限（MB）。">
              <TextInput
                value={values.logsMaxTotalSizeMb}
                onChange={(e) => update({ logsMaxTotalSizeMb: e.currentTarget.value })}
                placeholder="0"
                inputMode="numeric"
                disabled={disabled}
              />
            </Field>
          </div>
        </Card>

        <Card
          title="配额超限策略"
          description="quota-exceeded.switch-project / switch-preview-model。"
        >
          <div className="space-y-4">
            <ToggleSwitch
              label="切换 Project"
              description="quota-exceeded.switch-project"
              checked={values.quotaSwitchProject}
              onCheckedChange={(next) => update({ quotaSwitchProject: next })}
              disabled={disabled}
            />
            <ToggleSwitch
              label="切换 Preview Model"
              description="quota-exceeded.switch-preview-model"
              checked={values.quotaSwitchPreviewModel}
              onCheckedChange={(next) => update({ quotaSwitchPreviewModel: next })}
              disabled={disabled}
            />
          </div>
        </Card>
      </div>

      <div className="space-y-6">
        <div className="grid gap-6 lg:grid-cols-2">
          <Card title="路由" description="routing.strategy。">
            <div className="space-y-4">
              <Field label="routing.strategy" hint="选择路由策略。">
                <SelectInput
                  value={values.routingStrategy}
                  onChange={(value) => update({ routingStrategy: value as RoutingStrategy })}
                  options={routingOptions}
                  disabled={disabled}
                  ariaLabel="routing.strategy"
                />
              </Field>
            </div>
          </Card>

          <Card
            title="Streaming"
            description="streaming.keepalive-seconds / bootstrap-retries / nonstream-keepalive-interval。"
          >
            <div className="space-y-4">
              <div className="grid gap-3 lg:grid-cols-2">
                <Field label="streaming.keepalive-seconds" hint="非负整数（秒）。">
                  <TextInput
                    value={values.streaming.keepaliveSeconds}
                    onChange={(e) =>
                      update({
                        streaming: {
                          ...values.streaming,
                          keepaliveSeconds: e.currentTarget.value,
                        },
                      })
                    }
                    placeholder="0"
                    inputMode="numeric"
                    disabled={disabled}
                  />
                </Field>
                <Field label="streaming.bootstrap-retries" hint="非负整数。">
                  <TextInput
                    value={values.streaming.bootstrapRetries}
                    onChange={(e) =>
                      update({
                        streaming: {
                          ...values.streaming,
                          bootstrapRetries: e.currentTarget.value,
                        },
                      })
                    }
                    placeholder="0"
                    inputMode="numeric"
                    disabled={disabled}
                  />
                </Field>
              </div>
              <Field label="nonstream-keepalive-interval" hint="非负整数（秒）。">
                <TextInput
                  value={values.streaming.nonstreamKeepaliveInterval}
                  onChange={(e) =>
                    update({
                      streaming: {
                        ...values.streaming,
                        nonstreamKeepaliveInterval: e.currentTarget.value,
                      },
                    })
                  }
                  placeholder="0"
                  inputMode="numeric"
                  disabled={disabled}
                />
              </Field>
            </div>
          </Card>
        </div>

        <UserAgentRoutingRulesEditor
          rules={values.routingUserAgentRules}
          disabled={disabled}
          onChange={(rules) => update({ routingUserAgentRules: rules })}
        />
      </div>

      <div className="space-y-6">
        <PayloadRulesEditor
          title="Payload 默认规则"
          description="匹配模型后，对请求 payload 追加/覆盖参数（对应 `payload.default`）。"
          rules={values.payloadDefaultRules}
          disabled={disabled}
          onChange={(payloadDefaultRules) => update({ payloadDefaultRules })}
        />
        <PayloadRulesEditor
          title="Payload 覆盖规则"
          description="匹配模型后，覆盖请求 payload 参数（对应 `payload.override`）。"
          rules={values.payloadOverrideRules}
          disabled={disabled}
          onChange={(payloadOverrideRules) => update({ payloadOverrideRules })}
        />
        <PayloadFilterRulesEditor
          rules={values.payloadFilterRules}
          disabled={disabled}
          onChange={(payloadFilterRules) => update({ payloadFilterRules })}
        />
      </div>
    </div>
  );
}
