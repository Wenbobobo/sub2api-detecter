<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.modelFingerprintBatch.dialogTitle')"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-4" data-testid="fingerprint-batch-dialog">
      <!-- Subtitle -->
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="text-sm text-gray-500 dark:text-gray-400" data-testid="fingerprint-subtitle">
          {{ t('admin.accounts.modelFingerprintBatch.dialogSubtitle', { count: normalizedModels.length }) }}
        </p>
        <p class="text-xs text-gray-400 dark:text-gray-500">
          {{ t('admin.accounts.modelFingerprintBatch.backgroundHint') }}
        </p>
      </div>
      <p v-if="wildcardSkippedCount > 0" class="text-xs text-amber-600 dark:text-amber-400" data-testid="wildcard-skipped">
        {{ t('admin.accounts.modelFingerprintBatch.wildcardSkipped', { count: wildcardSkippedCount }) }}
      </p>

      <!-- Params -->
      <div class="rounded-lg border border-violet-100 bg-white/60 p-3 dark:border-violet-900/50 dark:bg-dark-800/40">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.accounts.modelFingerprintBatch.concurrency') }}
            </label>
            <Input
              data-testid="batch-concurrency"
              v-model="form.concurrency"
              type="number"
              placeholder="1-3"
              :hint="t('admin.accounts.modelFingerprintBatch.concurrencyHelp')"
              :disabled="isActive"
            />
          </div>
          <div class="flex items-end sm:col-span-2">
            <button
              type="button"
              data-testid="toggle-advanced"
              class="flex items-center gap-1 text-xs font-medium text-violet-600 hover:text-violet-700 dark:text-violet-400"
              @click="showAdvanced = !showAdvanced"
            >
              <Icon name="chevronDown" size="xs" :class="['transition-transform', showAdvanced ? 'rotate-180' : '']" />
              {{ t('admin.accounts.modelFingerprintBatch.advancedParams') }}
            </button>
          </div>
        </div>

        <div v-show="showAdvanced" class="mt-3" data-testid="advanced-params">
          <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.accounts.modelFingerprintBatch.paramsHelp') }}</p>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorApi') }}</label>
              <Select data-testid="detector-api" v-model="form.detector.api" :options="detectorApiOptions" :disabled="isActive" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorCount') }}</label>
              <Input data-testid="detector-count" v-model="form.detector.count" type="number" placeholder="1-3" :disabled="isActive || form.detector.strict" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorParallel') }}</label>
              <Input data-testid="detector-parallel" v-model="form.detector.parallel" type="number" placeholder="1-3" :disabled="isActive" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorTimeout') }}</label>
              <Input data-testid="detector-timeout_sec" v-model="form.detector.timeout_sec" type="number" placeholder="1-600" :disabled="isActive" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorRounds') }}</label>
              <Input data-testid="detector-rounds" v-model="form.detector.rounds" type="number" placeholder="1-5" :disabled="isActive" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorEffort') }}</label>
              <Input data-testid="detector-effort" v-model="form.detector.effort" :placeholder="t('admin.scheduledTests.detectorEffortPlaceholder')" :disabled="isActive" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.scheduledTests.detectorServiceTier') }}</label>
              <Select data-testid="detector-service-tier" v-model="form.detector.service_tier" :options="detectorServiceTierOptions" :disabled="isActive" />
            </div>
          </div>
          <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle data-testid="detector-no-stream" v-model="form.detector.no_stream" :disabled="isActive" />
                {{ t('admin.scheduledTests.detectorNoStream') }}
              </label>
            </div>
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle data-testid="detector-strict" v-model="form.detector.strict" :disabled="isActive" />
                {{ t('admin.scheduledTests.detectorStrict') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.scheduledTests.detectorStrictHelp') }}</p>
            </div>
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle data-testid="detector-allow-family" v-model="form.detector.allow_family_match" :disabled="isActive" />
                {{ t('admin.scheduledTests.detectorAllowFamilyMatch') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.scheduledTests.detectorAllowFamilyMatchHelp') }}</p>
            </div>
          </div>
        </div>

        <p class="mt-3 text-xs text-amber-600 dark:text-amber-400" data-testid="estimated-requests">
          {{ t('admin.accounts.modelFingerprintBatch.estimatedRequests', { count: estimatedRequests }) }}
        </p>
      </div>


      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-6">
        <Icon name="refresh" size="md" class="animate-spin text-gray-400" :stroke-width="2" />
        <span class="ml-2 text-sm text-gray-500">{{ t('common.loading') }}...</span>
      </div>

      <div
        v-else-if="!run"
        class="rounded-xl border border-dashed border-gray-300 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        data-testid="no-runs"
      >
        {{ normalizedModels.length > 0 ? t('admin.accounts.modelFingerprintBatch.noRuns') : t('admin.accounts.modelFingerprintBatch.noModels') }}
      </div>

      <template v-else>
        <!-- Run header -->
        <div class="space-y-2" data-testid="run-progress">
          <div class="flex flex-wrap items-center justify-between gap-2 text-sm text-gray-700 dark:text-gray-300">
            <div class="flex items-center gap-2">
              <span :class="['rounded px-2 py-0.5 text-xs font-medium', runStatusClass]" data-testid="run-status">
                {{ runStatusLabel }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.modelFingerprintBatch.latestRun', { time: formatDateTime(run.created_at) }) }}
              </span>
            </div>
            <span class="font-medium text-gray-900 dark:text-white" data-testid="progress-text">
              {{ t('admin.accounts.modelFingerprintBatch.progressCount', { done: run.completed_models, total: run.total_models }) }} · {{ progressPercent }}%
            </span>
          </div>
          <div class="h-2 w-full rounded-full bg-gray-200 dark:bg-dark-700">
            <div
              role="progressbar"
              :aria-valuenow="progressPercent"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-label="`${t('admin.accounts.modelFingerprintBatch.progress')}: ${progressPercent}%`"
              class="h-2 rounded-full bg-violet-600 transition-all"
              :style="{ width: `${progressPercent}%` }"
            ></div>
          </div>
          <div class="flex flex-wrap gap-3 text-xs" data-testid="run-summary">
            <span class="text-gray-600 dark:text-gray-400">{{ t('admin.accounts.modelFingerprintBatch.summaryTotal') }} {{ run.total_models }}</span>
            <span class="text-gray-600 dark:text-gray-400">{{ t('admin.accounts.modelFingerprintBatch.summaryCompleted') }} {{ run.completed_models }}</span>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.modelFingerprintBatch.summaryCancelled') }} {{ run.cancelled_models }}</span>
            <span class="text-green-600 dark:text-green-400">{{ t('admin.accounts.modelFingerprintBatch.summaryMatched') }} {{ run.matched_models }}</span>
            <span class="text-red-600 dark:text-red-400">{{ t('admin.accounts.modelFingerprintBatch.summaryMismatched') }} {{ run.mismatched_models }}</span>
            <span class="text-red-600 dark:text-red-400">{{ t('admin.accounts.modelFingerprintBatch.summaryFailed') }} {{ run.failed_models }}</span>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.modelFingerprintBatch.summaryUnsupported') }} {{ run.unsupported_models }}</span>
          </div>
          <p v-if="run.error_message" class="text-xs text-red-600 dark:text-red-400">
            {{ t('admin.accounts.modelFingerprintBatch.runError', { message: run.error_message }) }}
          </p>
        </div>

        <!-- Results table -->
        <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-full text-left text-xs" data-testid="results-table">
            <thead class="bg-gray-50 text-gray-500 dark:bg-dark-700 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.model') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.verdict') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.detectedModel') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.expectedModel') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.confidence') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.latency') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.accounts.modelFingerprintBatch.error') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <template v-for="result in results" :key="result.id">
                <tr
                  data-testid="result-row"
                  :class="['text-gray-700 dark:text-gray-300', hasDetail(result) ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/50' : '']"
                  @click="toggleDetail(result)"
                >
                  <td class="whitespace-nowrap px-3 py-2 font-mono">
                    <span class="inline-flex items-center gap-1">
                      <Icon
                        v-if="hasDetail(result)"
                        name="chevronDown"
                        size="xs"
                        :class="['transition-transform', expandedIds.has(result.id) ? 'rotate-180' : '']"
                      />
                      {{ result.model_id }}
                    </span>
                  </td>
                  <td class="px-3 py-2">
                    <span :class="['rounded px-2 py-0.5 text-xs font-medium', resultBadgeClass(result)]" data-testid="result-badge">
                      {{ resultBadgeLabel(result) }}
                    </span>
                    <span v-if="resultMatchNote(result)" class="mt-0.5 block text-[10px] text-violet-600 dark:text-violet-400">
                      {{ resultMatchNote(result) }}
                    </span>
                  </td>
                  <td class="px-3 py-2 font-mono">{{ result.detected_model_name || result.detected_model || '-' }}</td>
                  <td class="px-3 py-2 font-mono">{{ result.expected_model || '-' }}</td>
                  <td class="px-3 py-2">{{ result.probability != null ? formatProbability(result.probability) : '-' }}</td>
                  <td class="px-3 py-2">{{ result.latency_ms > 0 ? `${result.latency_ms}ms` : '-' }}</td>
                  <td class="max-w-xs truncate px-3 py-2 text-red-600 dark:text-red-400" :title="result.error_message">
                    {{ result.error_message || '' }}
                  </td>
                </tr>
                <tr v-if="expandedIds.has(result.id) && hasDetail(result)" data-testid="result-detail">
                  <td colspan="7" class="bg-gray-50 px-3 py-2 dark:bg-dark-800">
                    <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.accounts.modelFingerprintBatch.detail') }}</div>
                    <pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all rounded bg-white p-2 font-mono text-[11px] text-gray-700 dark:bg-dark-900 dark:text-gray-300">{{ formatDetail(result.detail) }}</pre>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button
          type="button"
          data-testid="fingerprint-close"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          @click="handleClose"
        >
          {{ t('admin.accounts.modelFingerprintBatch.close') }}
        </button>
        <button
          v-if="isActive"
          type="button"
          data-testid="fingerprint-cancel"
          :disabled="cancelling"
          class="flex items-center gap-1.5 rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-60"
          @click="showCancelConfirm = true"
        >
          <Icon v-if="cancelling" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          {{ cancelling ? t('admin.accounts.modelFingerprintBatch.cancelling') : t('admin.accounts.modelFingerprintBatch.cancel') }}
        </button>
        <button
          v-else
          type="button"
          data-testid="fingerprint-start"
          :disabled="starting || loading || normalizedModels.length === 0 || !accountId"
          class="flex items-center gap-1.5 rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-700 disabled:cursor-not-allowed disabled:opacity-60"
          @click="handleStart"
        >
          <Icon v-if="starting" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          {{ starting ? t('admin.accounts.modelFingerprintBatch.starting') : run ? t('admin.accounts.modelFingerprintBatch.rerun') : t('admin.accounts.modelFingerprintBatch.start') }}
        </button>
      </div>
    </template>

    <ConfirmDialog
      :show="showCancelConfirm"
      :title="t('admin.accounts.modelFingerprintBatch.cancelConfirmTitle')"
      :message="t('admin.accounts.modelFingerprintBatch.cancelConfirmMessage')"
      :confirm-text="t('admin.accounts.modelFingerprintBatch.cancelConfirm')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleCancelRun"
      @cancel="showCancelConfirm = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { modelFingerprintBatchAPI } from '@/api/admin/modelFingerprintBatch'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import type {
  ModelFingerprintBatchRun,
  ModelFingerprintBatchResult,
  ScheduledTestDetectorConfig
} from '@/types'

const POLL_INTERVAL_MS = 2000
const MAX_MODELS = 100
const ACTIVE_STATUSES = ['pending', 'running', 'cancelling']

const props = defineProps<{
  show: boolean
  accountId?: number | null
  models: string[]
}>()

const emit = defineEmits<{
  close: []
  'run-finished': [run: ModelFingerprintBatchRun]
}>()

const { t } = useI18n()
const appStore = useAppStore()

// ---------- detector form (mirrors ScheduledTestsPanel) ----------
interface DetectorForm {
  api: string
  count: string
  parallel: string
  timeout_sec: string
  rounds: string
  no_stream: boolean
  strict: boolean
  effort: string
  service_tier: string
  allow_family_match: boolean
}

const DETECTOR_APIS: ScheduledTestDetectorConfig['api'][] = ['auto', 'responses', 'chat_completions', 'messages']
const SERVICE_TIERS: ScheduledTestDetectorConfig['service_tier'][] = ['flex', 'default', 'fast', 'ultrafast']
const DEFAULT_CONCURRENCY = 2

const defaultDetectorConfig = (): ScheduledTestDetectorConfig => ({
  api: 'auto',
  count: 3,
  parallel: 3,
  timeout_sec: 120,
  rounds: 1,
  no_stream: false,
  strict: false,
  effort: '',
  service_tier: 'default',
  allow_family_match: false
})

const toDetectorForm = (cfg?: Partial<ScheduledTestDetectorConfig> | null): DetectorForm => {
  const merged = { ...defaultDetectorConfig(), ...(cfg ?? {}) }
  return {
    api: merged.api,
    count: String(merged.count),
    parallel: String(merged.parallel),
    timeout_sec: String(merged.timeout_sec),
    rounds: String(merged.rounds),
    no_stream: !!merged.no_stream,
    strict: !!merged.strict,
    effort: merged.effort ?? '',
    service_tier: merged.service_tier,
    allow_family_match: !!merged.allow_family_match
  }
}

const clampInt = (raw: string, min: number, max: number, fallback: number): number => {
  const n = Math.trunc(Number(raw))
  if (!Number.isFinite(n) || String(raw).trim() === '') return fallback
  return Math.min(max, Math.max(min, n))
}

const fromDetectorForm = (f: DetectorForm): ScheduledTestDetectorConfig => {
  const defaults = defaultDetectorConfig()
  const strict = !!f.strict
  return {
    api: DETECTOR_APIS.includes(f.api as ScheduledTestDetectorConfig['api'])
      ? (f.api as ScheduledTestDetectorConfig['api'])
      : defaults.api,
    // strict mode requires a complete 3/3 sample set
    count: strict ? 3 : clampInt(f.count, 1, 3, defaults.count),
    parallel: clampInt(f.parallel, 1, 3, defaults.parallel),
    timeout_sec: clampInt(f.timeout_sec, 1, 600, defaults.timeout_sec),
    rounds: clampInt(f.rounds, 1, 5, defaults.rounds),
    no_stream: !!f.no_stream,
    strict,
    effort: (f.effort ?? '').trim(),
    service_tier: SERVICE_TIERS.includes(f.service_tier as ScheduledTestDetectorConfig['service_tier'])
      ? (f.service_tier as ScheduledTestDetectorConfig['service_tier'])
      : defaults.service_tier,
    allow_family_match: !!f.allow_family_match
  }
}

const form = reactive({
  concurrency: String(DEFAULT_CONCURRENCY),
  detector: toDetectorForm()
})

const detectorApiOptions = computed<SelectOption[]>(() => [
  { value: 'auto', label: t('admin.scheduledTests.detectorApiAuto') },
  { value: 'responses', label: t('admin.scheduledTests.detectorApiResponses') },
  { value: 'chat_completions', label: t('admin.scheduledTests.detectorApiChatCompletions') },
  { value: 'messages', label: t('admin.scheduledTests.detectorApiMessages') }
])

const detectorServiceTierOptions = computed<SelectOption[]>(() => [
  { value: 'flex', label: t('admin.scheduledTests.serviceTierFlex') },
  { value: 'default', label: t('admin.scheduledTests.serviceTierDefault') },
  { value: 'fast', label: t('admin.scheduledTests.serviceTierFast') },
  { value: 'ultrafast', label: t('admin.scheduledTests.serviceTierUltrafast') }
])

// ---------- state ----------
const showAdvanced = ref(false)
const loading = ref(false)
const starting = ref(false)
const cancelling = ref(false)
const showCancelConfirm = ref(false)
const run = ref<ModelFingerprintBatchRun | null>(null)
const results = ref<ModelFingerprintBatchResult[]>([])
const expandedIds = reactive(new Set<number>())

/** Trimmed, de-duplicated, capped snapshot of the models to test. Wildcard
 * entries cannot be fingerprinted (the detector needs an exact model id). */
const splitModels = computed(() => {
  const seen = new Set<string>()
  const wildcard = new Set<string>()
  const exact: string[] = []
  for (const raw of props.models ?? []) {
    const model = String(raw ?? '').trim()
    if (!model) continue
    if (model.includes('*')) {
      wildcard.add(model)
      continue
    }
    if (seen.has(model)) continue
    seen.add(model)
    exact.push(model)
    if (exact.length >= MAX_MODELS) break
  }
  return { exact, wildcard: wildcard.size }
})
const normalizedModels = computed(() => splitModels.value.exact)
const wildcardSkippedCount = computed(() => splitModels.value.wildcard)


const isActive = computed(() => !!run.value && ACTIVE_STATUSES.includes(run.value.status))

const estimatedRequests = computed(() => {
  const cfg = fromDetectorForm(form.detector)
  return normalizedModels.value.length * cfg.count * cfg.rounds
})

const progressPercent = computed(() => {
  const r = run.value
  if (!r || !r.total_models) return 0
  return Math.min(100, Math.max(0, Math.round(((r.completed_models + r.cancelled_models) / r.total_models) * 100)))
})

const RUN_STATUS: Record<string, { cls: string; key: string }> = {
  pending: { cls: 'bg-gray-200 text-gray-700 dark:bg-dark-600 dark:text-gray-300', key: 'statusPending' },
  running: { cls: 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400', key: 'statusRunning' },
  cancelling: { cls: 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400', key: 'statusCancelling' },
  completed: { cls: 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400', key: 'statusCompleted' },
  cancelled: { cls: 'bg-gray-200 text-gray-700 dark:bg-dark-600 dark:text-gray-300', key: 'statusCancelled' },
  failed: { cls: 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400', key: 'statusFailed' }
}
const runStatusClass = computed(() => RUN_STATUS[run.value?.status ?? '']?.cls ?? RUN_STATUS.pending.cls)
const runStatusLabel = computed(() => {
  const entry = RUN_STATUS[run.value?.status ?? '']
  return entry ? t(`admin.accounts.modelFingerprintBatch.${entry.key}`) : String(run.value?.status ?? '')
})

// ---------- result badges (aligned with ScheduledTestsPanel VERDICT_BADGES) ----------
const GREEN_BADGE = 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
const RED_BADGE = 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'
const BLUE_BADGE = 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400'
const GRAY_BADGE = 'bg-gray-200 text-gray-700 dark:bg-dark-600 dark:text-gray-300'

const VERDICT_BADGES: Record<string, { cls: string; key: string }> = {
  match: { cls: GREEN_BADGE, key: 'admin.scheduledTests.verdictMatch' },
  mismatch: { cls: RED_BADGE, key: 'admin.scheduledTests.verdictMismatch' },
  invalid_reply: { cls: RED_BADGE, key: 'admin.scheduledTests.verdictInvalidReply' },
  error: { cls: RED_BADGE, key: 'admin.scheduledTests.verdictError' },
  unsupported: { cls: GRAY_BADGE, key: 'admin.scheduledTests.verdictUnsupported' }
}

const STATUS_BADGES: Record<string, { cls: string; key: string }> = {
  pending: { cls: GRAY_BADGE, key: 'admin.accounts.modelFingerprintBatch.pending' },
  running: { cls: BLUE_BADGE, key: 'admin.accounts.modelFingerprintBatch.running' },
  cancelled: { cls: GRAY_BADGE, key: 'admin.accounts.modelFingerprintBatch.cancelled' },
  failed: { cls: RED_BADGE, key: 'admin.accounts.modelFingerprintBatch.failed' },
  success: { cls: GREEN_BADGE, key: 'admin.accounts.modelFingerprintBatch.success' }
}

const badgeFor = (result: ModelFingerprintBatchResult) =>
  (result.verdict ? VERDICT_BADGES[result.verdict] : undefined) ??
  STATUS_BADGES[result.status] ??
  STATUS_BADGES.pending

const resultBadgeClass = (result: ModelFingerprintBatchResult): string => badgeFor(result).cls
const resultBadgeLabel = (result: ModelFingerprintBatchResult): string => t(badgeFor(result).key)

const formatProbability = (p: number): string => `${Math.round(p * 100)}%`

const resultMatchNote = (result: ModelFingerprintBatchResult): string => {
  const detail = result.detail as Record<string, unknown> | null
  const expectation = detail && typeof detail === 'object' ? (detail as Record<string, unknown>).expectation : null
  if (!expectation || typeof expectation !== 'object') return ''
  const meta = expectation as Record<string, unknown>
  if (meta.matched_via === 'alias') return t('admin.scheduledTests.matchedViaAlias')
  if (meta.matched_via === 'family') return t('admin.scheduledTests.matchedViaFamily')
  return ''
}

const hasDetail = (result: ModelFingerprintBatchResult): boolean => {
  const detail = result.detail
  if (detail == null) return false
  if (typeof detail === 'object') return Object.keys(detail).length > 0
  return true
}

const formatDetail = (detail: ModelFingerprintBatchResult['detail']): string => {
  try {
    return JSON.stringify(detail, null, 2)
  } catch {
    return String(detail)
  }
}

const toggleDetail = (result: ModelFingerprintBatchResult) => {
  if (!hasDetail(result)) return
  if (expandedIds.has(result.id)) expandedIds.delete(result.id)
  else expandedIds.add(result.id)
}

// ---------- polling & race protection ----------
let version = 0
let pollTimer: ReturnType<typeof setInterval> | null = null
let pollInFlight = false

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

const isHidden = () => typeof document !== 'undefined' && document.visibilityState === 'hidden'

const applyDetail = (detail: { run: ModelFingerprintBatchRun; results: ModelFingerprintBatchResult[] }) => {
  run.value = detail.run
  results.value = detail.results ?? []
}

const handleRunSettled = (finished: ModelFingerprintBatchRun) => {
  stopPolling()
  emit('run-finished', finished)
  if (finished.status === 'completed') {
    appStore.showSuccess(
      t('admin.accounts.modelFingerprintBatch.finished', {
        matched: finished.matched_models,
        mismatched: finished.mismatched_models,
        failed: finished.failed_models,
        unsupported: finished.unsupported_models
      })
    )
  } else if (finished.status === 'cancelled') {
    appStore.showInfo(t('admin.accounts.modelFingerprintBatch.cancelledToast'))
  } else if (finished.status === 'failed') {
    appStore.showError(t('admin.accounts.modelFingerprintBatch.runError', { message: finished.error_message || finished.status }))
  }
}

const pollOnce = async () => {
  const accountId = props.accountId
  const runId = run.value?.id
  if (!accountId || !runId || pollInFlight || isHidden()) return
  const current = version
  pollInFlight = true
  try {
    const detail = await modelFingerprintBatchAPI.getRun(accountId, runId)
    if (current !== version) return
    const wasActive = isActive.value
    applyDetail(detail)
    if (wasActive && !ACTIVE_STATUSES.includes(detail.run.status)) {
      handleRunSettled(detail.run)
    }
  } catch {
    // transient polling failures are ignored; next tick retries
  } finally {
    pollInFlight = false
  }
}

const startPolling = () => {
  stopPolling()
  pollTimer = setInterval(() => {
    void pollOnce()
  }, POLL_INTERVAL_MS)
}

const onVisibilityChange = () => {
  if (!isHidden() && pollTimer && isActive.value) void pollOnce()
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', onVisibilityChange)
}

const resetState = () => {
  version += 1
  stopPolling()
  pollInFlight = false
  run.value = null
  results.value = []
  expandedIds.clear()
  loading.value = false
  starting.value = false
  cancelling.value = false
  showCancelConfirm.value = false
}

const loadLatest = async () => {
  const accountId = props.accountId
  if (!accountId) return
  const current = version
  loading.value = true
  try {
    const runs = await modelFingerprintBatchAPI.listRuns(accountId, 1)
    if (current !== version) return
    const latest = runs[0]
    if (!latest) return
    const detail = await modelFingerprintBatchAPI.getRun(accountId, latest.id)
    if (current !== version) return
    applyDetail(detail)
    // Prefill params from the latest run so "rerun" repeats the same settings.
    form.detector = toDetectorForm(detail.run.detector_config)
    if (detail.run.concurrency) form.concurrency = String(detail.run.concurrency)
    if (ACTIVE_STATUSES.includes(detail.run.status)) startPolling()
  } catch (error) {
    if (current !== version) return
    const message = extractApiErrorMessage(error, '')
    appStore.showError(t('admin.accounts.modelFingerprintBatch.loadLatestFailed', { message }))
  } finally {
    if (current === version) loading.value = false
  }
}

const newIdempotencyKey = (): string => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const handleStart = async () => {
  const accountId = props.accountId
  const models = [...normalizedModels.value]
  if (!accountId || models.length === 0 || starting.value || isActive.value) return
  const current = version
  starting.value = true
  try {
    const detail = await modelFingerprintBatchAPI.createRun(
      accountId,
      {
        models,
        concurrency: clampInt(form.concurrency, 1, 3, DEFAULT_CONCURRENCY),
        detector_config: fromDetectorForm(form.detector)
      },
      newIdempotencyKey()
    )
    if (current !== version) return
    expandedIds.clear()
    applyDetail(detail)
    appStore.showSuccess(t('admin.accounts.modelFingerprintBatch.startSuccess', { count: models.length }))
    if (ACTIVE_STATUSES.includes(detail.run.status)) {
      startPolling()
    } else {
      handleRunSettled(detail.run)
    }
  } catch (error) {
    if (current !== version) return
    const httpStatus = (error as { response?: { status?: number } })?.response?.status
    if (httpStatus === 409) {
      // Another run is already active (e.g. double click from another tab):
      // show it instead of failing.
      appStore.showInfo(t('admin.accounts.modelFingerprintBatch.activeRunExists'))
      await loadLatest()
      return
    }
    const message = extractApiErrorMessage(error, '')
    appStore.showError(t('admin.accounts.modelFingerprintBatch.startFailed', { message }))
  } finally {
    if (current === version) starting.value = false
  }
}

const handleCancelRun = async () => {
  showCancelConfirm.value = false
  const accountId = props.accountId
  const runId = run.value?.id
  if (!accountId || !runId || cancelling.value) return
  const current = version
  cancelling.value = true
  try {
    const updated = await modelFingerprintBatchAPI.cancelRun(accountId, runId)
    if (current !== version) return
    if (updated) {
      run.value = { ...run.value!, ...updated }
      if (!ACTIVE_STATUSES.includes(updated.status)) {
        handleRunSettled(updated)
      } else {
        await pollOnce()
      }
    }
  } catch (error) {
    if (current !== version) return
    const message = extractApiErrorMessage(error, '')
    appStore.showError(t('admin.accounts.modelFingerprintBatch.cancelFailed', { message }))
  } finally {
    if (current === version) cancelling.value = false
  }
}

const handleClose = () => {
  // Backend run continues; we only stop observing it.
  stopPolling()
  emit('close')
}

watch(
  () => props.show,
  (visible) => {
    resetState()
    if (visible) void loadLatest()
  },
  { immediate: true }
)

watch(
  () => [props.accountId, normalizedModels.value.join('\n')],
  (next, prev) => {
    if (!props.show) return
    if (next[0] === prev?.[0] && next[1] === prev?.[1]) return
    if (next[0] !== prev?.[0]) {
      resetState()
      void loadLatest()
    }
  }
)

onBeforeUnmount(() => {
  version += 1
  stopPolling()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', onVisibilityChange)
  }
})

defineExpose({ loadLatest, handleStart })
</script>