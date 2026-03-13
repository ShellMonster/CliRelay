import { useCallback, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { HeaderInputList } from "@/components/ui/HeaderInputList";
import { ModelInputList } from "@/components/ui/ModelInputList";
import { useEdgeSwipeBack } from "@/hooks/useEdgeSwipeBack";
import { SecondaryScreenShell } from "@/components/common/SecondaryScreenShell";
import { modelsApi, providersApi } from "@/lib/http/apis";
import { useAuthStore, useConfigStore, useNotificationStore } from "@/stores";
import type { ProviderKeyConfig } from "@/types";
import { buildHeaderObject, headersToEntries } from "@/utils/headers";
import type { ModelInfo } from "@/utils/models";
import { entriesToModels } from "@/components/ui/modelInputListUtils";
import {
  buildOpenAIModelsEndpoint,
  excludedModelsToText,
  parseExcludedModels,
} from "@/components/providers/utils";
import type { ProviderFormState } from "@/components/providers";
import layoutStyles from "./AiProvidersEditLayout.module.scss";
import styles from "./AiProvidersPage.module.scss";

type LocationState = { fromAiProviders?: boolean } | null;

const buildEmptyForm = (): ProviderFormState => ({
  apiKey: "",
  name: "",
  prefix: "",
  baseUrl: "",
  proxyUrl: "",
  headers: [],
  models: [],
  excludedModels: [],
  modelEntries: [{ name: "", alias: "" }],
  excludedText: "",
});

const parseIndexParam = (value: string | undefined) => {
  if (!value) return null;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : null;
};

export function AiProvidersCodexEditPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const params = useParams<{ index?: string }>();

  const { showNotification } = useNotificationStore();
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const disableControls = connectionStatus !== "connected";

  const fetchConfig = useConfigStore((state) => state.fetchConfig);
  const updateConfigValue = useConfigStore((state) => state.updateConfigValue);
  const clearCache = useConfigStore((state) => state.clearCache);

  const [configs, setConfigs] = useState<ProviderKeyConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [form, setForm] = useState<ProviderFormState>(() => buildEmptyForm());
  const [discoveredModels, setDiscoveredModels] = useState<ModelInfo[]>([]);
  const [fetchingModels, setFetchingModels] = useState(false);
  const [modelFetchError, setModelFetchError] = useState("");
  const [modelSearch, setModelSearch] = useState("");
  const [selectedDiscovered, setSelectedDiscovered] = useState<Set<string>>(
    new Set(),
  );

  const hasIndexParam = typeof params.index === "string";
  const editIndex = useMemo(
    () => parseIndexParam(params.index),
    [params.index],
  );
  const invalidIndexParam = hasIndexParam && editIndex === null;

  const initialData = useMemo(() => {
    if (editIndex === null) return undefined;
    return configs[editIndex];
  }, [configs, editIndex]);

  const invalidIndex = editIndex !== null && !initialData;

  const title =
    editIndex !== null
      ? t("ai_providers.codex_edit_modal_title")
      : t("ai_providers.codex_add_modal_title");

  const handleBack = useCallback(() => {
    const state = location.state as LocationState;
    if (state?.fromAiProviders) {
      navigate(-1);
      return;
    }
    navigate("/ai-providers", { replace: true });
  }, [location.state, navigate]);

  const swipeRef = useEdgeSwipeBack({ onBack: handleBack });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        handleBack();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleBack]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");

    fetchConfig("codex-api-key")
      .then((value) => {
        if (cancelled) return;
        setConfigs(Array.isArray(value) ? (value as ProviderKeyConfig[]) : []);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : "";
        setError(message || t("notification.refresh_failed"));
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [fetchConfig, t]);

  useEffect(() => {
    if (loading) return;

    if (initialData) {
      setForm({
        ...initialData,
        headers: headersToEntries(initialData.headers),
        modelEntries: (initialData.models || []).map((model) => ({
          name: model.name,
          alias: model.alias ?? "",
        })),
        excludedText: excludedModelsToText(initialData.excludedModels),
      });
      return;
    }
    setForm(buildEmptyForm());
  }, [initialData, loading]);

  const canSave =
    !disableControls &&
    !saving &&
    !loading &&
    !invalidIndexParam &&
    !invalidIndex;
  const modelsEndpoint = useMemo(
    () => buildOpenAIModelsEndpoint(form.baseUrl ?? ""),
    [form.baseUrl],
  );

  const normalizedDiscoveredModels = useMemo(
    () =>
      discoveredModels
        .map((model, index) => {
          const name = String(model.name ?? "").trim();
          if (!name) return null;
          return {
            ...model,
            name,
            alias: String(model.alias ?? "").trim(),
            description: String(model.description ?? "").trim(),
            _key: `${name}:${index}`,
          };
        })
        .filter(
          (
            model,
          ): model is ModelInfo & {
            name: string;
            alias: string;
            description: string;
            _key: string;
          } => Boolean(model),
        ),
    [discoveredModels],
  );

  const filteredDiscoveredModels = useMemo(() => {
    const keyword = modelSearch.trim().toLowerCase();
    if (!keyword) return normalizedDiscoveredModels;
    return normalizedDiscoveredModels.filter((model) => {
      const name = model.name.toLowerCase();
      const alias = model.alias.toLowerCase();
      const description = model.description.toLowerCase();
      return (
        name.includes(keyword) ||
        alias.includes(keyword) ||
        description.includes(keyword)
      );
    });
  }, [modelSearch, normalizedDiscoveredModels]);

  const handleSave = useCallback(async () => {
    if (!canSave) return;

    const trimmedBaseUrl = (form.baseUrl ?? "").trim();
    const baseUrl = trimmedBaseUrl || undefined;
    if (!baseUrl) {
      showNotification(t("notification.codex_base_url_required"), "error");
      return;
    }

    setSaving(true);
    setError("");
    try {
      const payload: ProviderKeyConfig = {
        apiKey: form.apiKey.trim(),
        name: form.name?.trim() || undefined,
        prefix: form.prefix?.trim() || undefined,
        baseUrl,
        proxyUrl: form.proxyUrl?.trim() || undefined,
        headers: buildHeaderObject(form.headers),
        models: entriesToModels(form.modelEntries),
        excludedModels: parseExcludedModels(form.excludedText),
      };

      const nextList =
        editIndex !== null
          ? configs.map((item, idx) => (idx === editIndex ? payload : item))
          : [...configs, payload];

      await providersApi.saveCodexConfigs(nextList);
      updateConfigValue("codex-api-key", nextList);
      clearCache("codex-api-key");
      showNotification(
        editIndex !== null
          ? t("notification.codex_config_updated")
          : t("notification.codex_config_added"),
        "success",
      );
      handleBack();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "";
      setError(message);
      showNotification(
        `${t("notification.update_failed")}: ${message}`,
        "error",
      );
    } finally {
      setSaving(false);
    }
  }, [
    canSave,
    clearCache,
    configs,
    editIndex,
    form,
    handleBack,
    showNotification,
    t,
    updateConfigValue,
  ]);

  const fetchCodexModels = useCallback(async () => {
    const baseUrl = (form.baseUrl ?? "").trim();
    if (!baseUrl) {
      showNotification(
        t("ai_providers.openai_models_fetch_invalid_url"),
        "error",
      );
      return;
    }

    setFetchingModels(true);
    setModelFetchError("");
    try {
      const headerObject = buildHeaderObject(form.headers);
      const hasAuthHeader = Object.keys(headerObject).some(
        (key) => key.toLowerCase() === "authorization",
      );
      const apiKey = form.apiKey.trim();
      const list = await modelsApi.fetchModelsViaApiCall(
        baseUrl,
        hasAuthHeader ? undefined : apiKey || undefined,
        headerObject,
      );
      setDiscoveredModels(list);
      setSelectedDiscovered(new Set());
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "";
      setDiscoveredModels([]);
      setModelFetchError(
        `${t("ai_providers.openai_models_fetch_error")}: ${message || t("notification.refresh_failed")}`,
      );
    } finally {
      setFetchingModels(false);
    }
  }, [form.apiKey, form.baseUrl, form.headers, showNotification, t]);

  const handleApplyDiscoveredModels = useCallback(() => {
    if (!selectedDiscovered.size) return;

    let addedCount = 0;
    setForm((prev) => {
      const baseEntries = prev.modelEntries.filter(
        (entry) => entry.name.trim() || entry.alias.trim(),
      );
      const seen = new Set(
        baseEntries
          .map((entry) => entry.name.trim().toLowerCase())
          .filter(Boolean),
      );
      const nextEntries = [...baseEntries];

      normalizedDiscoveredModels.forEach((model) => {
        if (!selectedDiscovered.has(model.name)) return;
        const key = model.name.toLowerCase();
        if (seen.has(key)) return;
        seen.add(key);
        addedCount += 1;
        nextEntries.push({
          name: model.name,
          alias: model.alias || "",
        });
      });

      return {
        ...prev,
        modelEntries: nextEntries.length
          ? nextEntries
          : [{ name: "", alias: "" }],
      };
    });

    if (addedCount > 0) {
      showNotification(
        t("ai_providers.openai_models_fetch_added", { count: addedCount }),
        "success",
      );
    }
    setSelectedDiscovered(new Set());
  }, [normalizedDiscoveredModels, selectedDiscovered, showNotification, t]);

  const toggleDiscoveredModel = useCallback((name: string) => {
    setSelectedDiscovered((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  return (
    <SecondaryScreenShell
      ref={swipeRef}
      contentClassName={layoutStyles.content}
      title={title}
      onBack={handleBack}
      backLabel={t("common.back")}
      backAriaLabel={t("common.back")}
      rightAction={
        <Button
          size="sm"
          onClick={handleSave}
          loading={saving}
          disabled={!canSave}
        >
          {t("common.save")}
        </Button>
      }
      isLoading={loading}
      loadingLabel={t("common.loading")}
    >
      <Card>
        {error && <div className="error-box">{error}</div>}
        {invalidIndexParam || invalidIndex ? (
          <div className="hint">{t("common.invalid_provider_index")}</div>
        ) : (
          <>
            <Input
              label="渠道名称"
              placeholder="例如：Codex 主力渠道（必填）"
              value={form.name ?? ""}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, name: e.target.value }))
              }
              disabled={disableControls || saving}
            />
            <Input
              label={t("ai_providers.codex_add_modal_key_label")}
              value={form.apiKey}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, apiKey: e.target.value }))
              }
              disabled={disableControls || saving}
            />
            <Input
              label={t("ai_providers.prefix_label")}
              placeholder={t("ai_providers.prefix_placeholder")}
              value={form.prefix ?? ""}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, prefix: e.target.value }))
              }
              hint={t("ai_providers.prefix_hint")}
              disabled={disableControls || saving}
            />
            <Input
              label={t("ai_providers.codex_add_modal_url_label")}
              value={form.baseUrl ?? ""}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, baseUrl: e.target.value }))
              }
              disabled={disableControls || saving}
            />
            <Input
              label={t("ai_providers.codex_add_modal_proxy_label")}
              value={form.proxyUrl ?? ""}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, proxyUrl: e.target.value }))
              }
              disabled={disableControls || saving}
            />
            <HeaderInputList
              entries={form.headers}
              onChange={(entries) =>
                setForm((prev) => ({ ...prev, headers: entries }))
              }
              addLabel={t("common.custom_headers_add")}
              keyPlaceholder={t("common.custom_headers_key_placeholder")}
              valuePlaceholder={t("common.custom_headers_value_placeholder")}
              removeButtonTitle={t("common.delete")}
              removeButtonAriaLabel={t("common.delete")}
              disabled={disableControls || saving}
            />
            <div className={styles.modelConfigSection}>
              <div className={styles.modelConfigHeader}>
                <label className={styles.modelConfigTitle}>
                  {t("ai_providers.openai_add_modal_models_label")}
                </label>
                <div className={styles.modelConfigToolbar}>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() =>
                      setForm((prev) => ({
                        ...prev,
                        modelEntries: [
                          ...prev.modelEntries,
                          { name: "", alias: "" },
                        ],
                      }))
                    }
                    disabled={disableControls || saving || fetchingModels}
                  >
                    {t("ai_providers.openai_models_add_btn")}
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => void fetchCodexModels()}
                    loading={fetchingModels}
                    disabled={disableControls || saving}
                  >
                    {t("ai_providers.openai_models_fetch_button")}
                  </Button>
                </div>
              </div>
              <div className={styles.sectionHint}>
                {t("ai_providers.openai_models_hint")}
              </div>
              <ModelInputList
                entries={form.modelEntries}
                onChange={(entries) =>
                  setForm((prev) => ({ ...prev, modelEntries: entries }))
                }
                namePlaceholder={t("common.model_name_placeholder")}
                aliasPlaceholder={t("common.model_alias_placeholder")}
                disabled={disableControls || saving || fetchingModels}
                hideAddButton
                className={styles.modelInputList}
                rowClassName={styles.modelInputRow}
                inputClassName={styles.modelInputField}
                removeButtonClassName={styles.modelRowRemoveButton}
                removeButtonTitle={t("common.delete")}
                removeButtonAriaLabel={t("common.delete")}
              />
              <div className={styles.openaiModelsEndpointSection}>
                <label className={styles.openaiModelsEndpointLabel}>
                  {t("ai_providers.openai_models_fetch_url_label")}
                </label>
                <div className={styles.openaiModelsEndpointControls}>
                  <input
                    className={`input ${styles.openaiModelsEndpointInput}`}
                    readOnly
                    value={modelsEndpoint}
                  />
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => void fetchCodexModels()}
                    loading={fetchingModels}
                    disabled={disableControls || saving}
                  >
                    {t("ai_providers.openai_models_fetch_refresh")}
                  </Button>
                  <Button
                    size="sm"
                    onClick={handleApplyDiscoveredModels}
                    disabled={
                      disableControls ||
                      saving ||
                      fetchingModels ||
                      selectedDiscovered.size === 0
                    }
                  >
                    {t("ai_providers.openai_models_fetch_apply")}
                  </Button>
                </div>
              </div>
              <Input
                label={t("ai_providers.openai_models_search_label")}
                placeholder={t("ai_providers.openai_models_search_placeholder")}
                value={modelSearch}
                onChange={(e) => setModelSearch(e.target.value)}
                disabled={fetchingModels}
              />
              {modelFetchError && (
                <div className="error-box">{modelFetchError}</div>
              )}
              {fetchingModels ? (
                <div className={styles.sectionHint}>
                  {t("ai_providers.openai_models_fetch_loading")}
                </div>
              ) : normalizedDiscoveredModels.length === 0 ? (
                <div className={styles.sectionHint}>
                  {t("ai_providers.openai_models_fetch_hint")}
                </div>
              ) : filteredDiscoveredModels.length === 0 ? (
                <div className={styles.sectionHint}>
                  {t("ai_providers.openai_models_search_empty")}
                </div>
              ) : (
                <div className={styles.modelDiscoveryList}>
                  {filteredDiscoveredModels.map((model) => {
                    const checked = selectedDiscovered.has(model.name);
                    return (
                      <label
                        key={model._key}
                        className={`${styles.modelDiscoveryRow} ${
                          checked ? styles.modelDiscoveryRowSelected : ""
                        }`}
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => toggleDiscoveredModel(model.name)}
                        />
                        <div className={styles.modelDiscoveryMeta}>
                          <div className={styles.modelDiscoveryName}>
                            {model.name}
                            {model.alias && (
                              <span className={styles.modelDiscoveryAlias}>
                                {model.alias}
                              </span>
                            )}
                          </div>
                          {model.description && (
                            <div className={styles.modelDiscoveryDesc}>
                              {model.description}
                            </div>
                          )}
                        </div>
                      </label>
                    );
                  })}
                </div>
              )}
            </div>
            <div className="form-group">
              <label>{t("ai_providers.excluded_models_label")}</label>
              <textarea
                className="input"
                placeholder={t("ai_providers.excluded_models_placeholder")}
                value={form.excludedText}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, excludedText: e.target.value }))
                }
                rows={4}
                disabled={disableControls || saving}
              />
              <div className="hint">
                {t("ai_providers.excluded_models_hint")}
              </div>
            </div>
          </>
        )}
      </Card>
    </SecondaryScreenShell>
  );
}
