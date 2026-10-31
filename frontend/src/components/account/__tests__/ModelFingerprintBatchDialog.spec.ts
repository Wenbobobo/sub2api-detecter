import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

const { listRuns, getRun, createRun, cancelRun, showError, showSuccess, showInfo, showWarning } = vi.hoisted(() => ({
  listRuns: vi.fn(), getRun: vi.fn(), createRun: vi.fn(), cancelRun: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn(),
}))
vi.mock('@/api/admin/modelFingerprintBatch', () => ({
  modelFingerprintBatchAPI: { listRuns, getRun, createRun, cancelRun },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess, showInfo, showWarning }) }))

import ModelFingerprintBatchDialog from '../ModelFingerprintBatchDialog.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'

const detectorConfig = {
  api: 'auto', count: 3, parallel: 3, timeout_sec: 120, rounds: 1,
  no_stream: false, strict: false, effort: '', service_tier: 'default', allow_family_match: false,
}

const makeRun = (overrides: Record<string, unknown> = {}) => ({
  id: 11, account_id: 7, status: 'running', models: ['a', 'b'], detector_config: detectorConfig,
  concurrency: 2, total_models: 2, completed_models: 1, matched_models: 1, mismatched_models: 0,
  failed_models: 0, unsupported_models: 0, cancelled_models: 0, estimated_requests: 9,
  error_message: '', terminal_reason: 'completed', started_at: null, finished_at: null,
  created_at: '2026-10-30T10:00:00Z', updated_at: '2026-10-30T10:00:00Z',
  ...overrides,
})

const makeResult = (overrides: Record<string, unknown> = {}) => ({
  id: 1, run_id: 11, model_id: 'a', status: 'success', verdict: 'match', expected_model: 'a',
  detected_model: 'a', detected_model_name: 'A', detected_family: 'fam', probability: 0.91,
  latency_ms: 1200, error_message: '', detail: { foo: 'bar' }, detail_truncated: false,
  position: 0, started_at: null, finished_at: null,
  ...overrides,
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean, title: String },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})
const ConfirmDialogStub = defineComponent({
  props: { show: Boolean },
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-testid="confirm-stub"><button data-testid="confirm-yes" @click="$emit(\'confirm\')">yes</button></div>',
})

async function openDialog(props: Record<string, unknown> = {}) {
  const wrapper = mount(ModelFingerprintBatchDialog, {
    props: { show: false, accountId: 7, models: ['a', 'b', 'a', ' '], ...props },
    global: {
      stubs: { BaseDialog: BaseDialogStub, ConfirmDialog: ConfirmDialogStub, Select: true, Icon: true },
    },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const byTestId = (wrapper: VueWrapper<any>, comp: any, id: string) =>
  wrapper.findAllComponents(comp).find(c => c.attributes('data-testid') === id)!

describe('ModelFingerprintBatchDialog', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listRuns.mockResolvedValue([])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows empty state and de-duplicated model count when no runs exist', async () => {
    const wrapper = await openDialog()
    expect(listRuns).toHaveBeenCalledWith(7, 1)
    expect(getRun).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="no-runs"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="estimated-requests"]').exists()).toBe(true)
  })

  it('loads the latest run on open and renders its results', async () => {
    const run = makeRun({ status: 'completed', completed_models: 2, matched_models: 1, mismatched_models: 1 })
    listRuns.mockResolvedValue([run])
    getRun.mockResolvedValue({
      run,
      results: [
        makeResult(),
        makeResult({ id: 2, model_id: 'b', verdict: 'mismatch', detected_model: 'x', detected_model_name: '', probability: 0.5 }),
      ],
    })
    const wrapper = await openDialog()
    expect(getRun).toHaveBeenCalledWith(7, 11)
    const rows = wrapper.findAll('[data-testid="result-row"]')
    expect(rows).toHaveLength(2)
    const badges = wrapper.findAll('[data-testid="result-badge"]')
    expect(badges[0].text()).toBe('admin.scheduledTests.verdictMatch')
    expect(badges[0].classes()).toContain('bg-green-100')
    expect(badges[1].text()).toBe('admin.scheduledTests.verdictMismatch')
    expect(badges[1].classes()).toContain('bg-red-100')
    expect(rows[0].text()).toContain('91%')
    expect(wrapper.get('[data-testid="run-status"]').text()).toBe('admin.accounts.modelFingerprintBatch.statusCompleted')
    // expand detail JSON
    expect(wrapper.find('[data-testid="result-detail"]').exists()).toBe(false)
    await rows[0].trigger('click')
    expect(wrapper.get('[data-testid="result-detail"]').text()).toContain('"foo": "bar"')
    // completed run → start button offers rerun, no cancel
    expect(wrapper.find('[data-testid="fingerprint-cancel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="fingerprint-start"]').text()).toBe('admin.accounts.modelFingerprintBatch.rerun')
  })

  it('submits models snapshot, clamped concurrency and detector_config', async () => {
    const run = makeRun({ status: 'running', completed_models: 0 })
    createRun.mockResolvedValue({ run, results: [makeResult({ status: 'pending', verdict: '', probability: null })] })
    const wrapper = await openDialog()

    await byTestId(wrapper, Input, 'batch-concurrency').vm.$emit('update:modelValue', '9')
    await byTestId(wrapper, Input, 'detector-rounds').vm.$emit('update:modelValue', '2')
    await byTestId(wrapper, Input, 'detector-timeout_sec').vm.$emit('update:modelValue', '9999')
    await byTestId(wrapper, Toggle, 'detector-strict').vm.$emit('update:modelValue', true)
    await byTestId(wrapper, Input, 'detector-count').vm.$emit('update:modelValue', '1')

    await wrapper.get('[data-testid="fingerprint-start"]').trigger('click')
    await flushPromises()

    expect(createRun).toHaveBeenCalledTimes(1)
    expect(createRun.mock.calls[0][0]).toBe(7)
    expect(createRun.mock.calls[0][1]).toEqual({
      models: ['a', 'b'],
      concurrency: 3,
      detector_config: { ...detectorConfig, rounds: 2, timeout_sec: 600, strict: true, count: 3 },
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.modelFingerprintBatch.startSuccess')
  })

  it('renders progress while running, polls until completion and emits run-finished', async () => {
    vi.useFakeTimers()
    const running = makeRun({ status: 'running', completed_models: 1, total_models: 4 })
    listRuns.mockResolvedValue([running])
    getRun.mockResolvedValueOnce({
      run: running,
      results: [makeResult(), makeResult({ id: 2, model_id: 'b', status: 'running', verdict: '' })],
    })
    const wrapper = await openDialog()

    expect(wrapper.get('[data-testid="progress-text"]').text()).toContain('25%')
    expect(wrapper.get('[role="progressbar"]').attributes('style')).toContain('width: 25%')
    expect(wrapper.get('[data-testid="run-status"]').text()).toBe('admin.accounts.modelFingerprintBatch.statusRunning')
    const badges = wrapper.findAll('[data-testid="result-badge"]')
    expect(badges[1].text()).toBe('admin.accounts.modelFingerprintBatch.running')
    expect(badges[1].classes()).toContain('bg-blue-100')
    expect(wrapper.find('[data-testid="fingerprint-cancel"]').exists()).toBe(true)

    const done = makeRun({ status: 'completed', completed_models: 4, total_models: 4, matched_models: 3, unsupported_models: 1 })
    getRun.mockResolvedValue({
      run: done,
      results: [makeResult(), makeResult({ id: 2, model_id: 'b', status: 'success', verdict: 'unsupported', probability: null })],
    })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(getRun).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="progress-text"]').text()).toContain('100%')
    const finalBadges = wrapper.findAll('[data-testid="result-badge"]')
    expect(finalBadges[1].text()).toBe('admin.scheduledTests.verdictUnsupported')
    expect(finalBadges[1].classes()).toContain('bg-gray-200')
    expect(wrapper.emitted('run-finished')?.[0]?.[0]).toMatchObject({ id: 11, status: 'completed' })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.modelFingerprintBatch.finished')

    // polling stopped after completion
    await vi.advanceTimersByTimeAsync(6000)
    expect(getRun).toHaveBeenCalledTimes(2)
  })


  it('cancels an active run after confirmation', async () => {
    const running = makeRun({ status: 'running' })
    listRuns.mockResolvedValue([running])
    getRun.mockResolvedValueOnce({ run: running, results: [] })
    const cancelled = makeRun({ status: 'cancelled' })
    cancelRun.mockResolvedValue(cancelled)
    getRun.mockResolvedValue({ run: cancelled, results: [] })
    const wrapper = await openDialog()

    await wrapper.get('[data-testid="fingerprint-cancel"]').trigger('click')
    await wrapper.get('[data-testid="confirm-yes"]').trigger('click')
    await flushPromises()

    expect(cancelRun).toHaveBeenCalledWith(7, 11)
    expect(wrapper.get('[data-testid="run-status"]').text()).toBe('admin.accounts.modelFingerprintBatch.statusCancelled')
    expect(wrapper.find('[data-testid="fingerprint-cancel"]').exists()).toBe(false)
    expect(showInfo).toHaveBeenCalledWith('admin.accounts.modelFingerprintBatch.cancelledToast')
  })

  it('stops polling when the dialog is closed', async () => {
    vi.useFakeTimers()
    const running = makeRun({ status: 'running' })
    listRuns.mockResolvedValue([running])
    getRun.mockResolvedValue({ run: running, results: [] })
    const wrapper = await openDialog()
    expect(getRun).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-testid="fingerprint-close"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    await wrapper.setProps({ show: false })
    await vi.advanceTimersByTimeAsync(6000)
    expect(getRun).toHaveBeenCalledTimes(1)
  })

  it('shows an error toast when starting fails', async () => {
    createRun.mockRejectedValue({ status: 409, message: 'busy' })
    const wrapper = await openDialog()
    await wrapper.get('[data-testid="fingerprint-start"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.accounts.modelFingerprintBatch.startFailed')
  })
})