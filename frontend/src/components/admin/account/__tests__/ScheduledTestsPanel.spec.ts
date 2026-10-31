import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

import ScheduledTestsPanel from '../ScheduledTestsPanel.vue'
import Toggle from '@/components/common/Toggle.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'

const { listByAccount, listResults, create, update, showError, showSuccess } = vi.hoisted(() => ({
  listByAccount: vi.fn(), listResults: vi.fn(), create: vi.fn(), update: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(),
}))
vi.mock('@/api/admin', () => ({
  adminAPI: { scheduledTests: { listByAccount, listResults, create, update } },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))

const detectorConfig = {
  api: 'messages', count: 2, parallel: 1, timeout_sec: 60, rounds: 2,
  no_stream: true, strict: false, effort: 'high', service_tier: 'flex', allow_family_match: true,
}

const plan = (overrides: Record<string, unknown> = {}) => ({
  id: 1, account_id: 1, model_id: 'claude-sonnet-4-5', cron_expression: '*/30 * * * *',
  enabled: true, max_results: 50, auto_recover: false, test_mode: 'liveness', auto_disable: false,
  detector_config: detectorConfig, last_run_at: null, next_run_at: null, created_at: '', updated_at: '',
  ...overrides,
})

async function openPanel() {
  const wrapper = shallowMount(ScheduledTestsPanel, {
    props: { show: false, accountId: 1, modelOptions: [{ value: 'claude-sonnet-4-5', label: 'Sonnet' }] },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const byTestId = (wrapper: VueWrapper<any>, comp: any, id: string) =>
  wrapper.findAllComponents(comp).find(c => c.attributes('data-testid') === id)!

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const saveButton = (wrapper: VueWrapper<any>) =>
  wrapper.findAll('button').find(b => b.text() === 'common.save')!

describe('ScheduledTestsPanel fingerprint settings', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listByAccount.mockResolvedValue([])
    listResults.mockResolvedValue([])
  })

  it('disables the detector params until fingerprint mode is enabled', async () => {
    const wrapper = await openPanel()
    await wrapper.find('button.btn-primary').trigger('click')
    expect(wrapper.find('[data-testid="new-detector-params"]').classes()).toContain('pointer-events-none')
    expect(byTestId(wrapper, Input, 'new-detector-count').props('disabled')).toBe(true)
    expect(byTestId(wrapper, Select, 'new-detector-api').props('disabled')).toBe(true)

    byTestId(wrapper, Toggle, 'new-fingerprint-toggle').vm.$emit('update:modelValue', true)
    await flushPromises()

    expect(wrapper.find('[data-testid="new-detector-params"]').classes()).not.toContain('pointer-events-none')
    expect(byTestId(wrapper, Input, 'new-detector-count').props('disabled')).toBe(false)
    expect(byTestId(wrapper, Select, 'new-detector-api').props('disabled')).toBe(false)
  })

  it('sends test_mode, auto_disable and detector_config when creating a plan', async () => {
    create.mockResolvedValue(plan())
    const wrapper = await openPanel()
    await wrapper.find('button.btn-primary').trigger('click')
    wrapper.findAllComponents(Select)[0].vm.$emit('update:modelValue', 'claude-sonnet-4-5')
    wrapper.findAllComponents(Input)[0].vm.$emit('update:modelValue', '*/10 * * * *')
    byTestId(wrapper, Toggle, 'new-fingerprint-toggle').vm.$emit('update:modelValue', true)
    byTestId(wrapper, Toggle, 'new-auto-disable-toggle').vm.$emit('update:modelValue', true)
    byTestId(wrapper, Input, 'new-detector-count').vm.$emit('update:modelValue', '2')
    byTestId(wrapper, Input, 'new-detector-timeout_sec').vm.$emit('update:modelValue', '9999')
    await flushPromises()

    await saveButton(wrapper).trigger('click')
    await flushPromises()

    expect(create).toHaveBeenCalledTimes(1)
    expect(create.mock.calls[0][0]).toEqual({
      account_id: 1,
      model_id: 'claude-sonnet-4-5',
      cron_expression: '*/10 * * * *',
      enabled: true,
      max_results: 100,
      auto_recover: false,
      test_mode: 'fingerprint',
      auto_disable: true,
      detector_config: {
        api: 'auto', count: 2, parallel: 3, timeout_sec: 600, rounds: 1,
        no_stream: false, strict: false, effort: '', service_tier: 'default', allow_family_match: false,
      },
    })
  })


  it('loads existing detector_config into the edit form and saves it back', async () => {
    listByAccount.mockResolvedValue([plan({ test_mode: 'fingerprint', auto_disable: true })])
    update.mockResolvedValue(plan({ test_mode: 'fingerprint', auto_disable: true }))
    const wrapper = await openPanel()
    expect(wrapper.find('[data-testid="plan-fingerprint-badge"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-auto-disable-badge"]').exists()).toBe(true)

    await wrapper.find('button[title="admin.scheduledTests.editPlan"]').trigger('click')
    expect(wrapper.find('[data-testid="edit-detector-params"]').classes()).not.toContain('pointer-events-none')
    expect(byTestId(wrapper, Input, 'edit-detector-effort').props('modelValue')).toBe('high')

    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, expect.objectContaining({
      test_mode: 'fingerprint',
      auto_disable: true,
      detector_config: detectorConfig,
    }))
  })

  it('uses default detector_config when a legacy plan has none', async () => {
    listByAccount.mockResolvedValue([plan({ detector_config: null })])
    update.mockResolvedValue(plan())
    const wrapper = await openPanel()
    await wrapper.find('button[title="admin.scheduledTests.editPlan"]').trigger('click')
    expect(wrapper.find('[data-testid="edit-detector-params"]').classes()).toContain('pointer-events-none')
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(update.mock.calls[0][1]).toMatchObject({
      test_mode: 'liveness',
      auto_disable: false,
      detector_config: {
        api: 'auto', count: 3, parallel: 3, timeout_sec: 120, rounds: 1,
        no_stream: false, strict: false, effort: '', service_tier: 'default', allow_family_match: false,
      },
    })
  })
})

describe('ScheduledTestsPanel fingerprint results', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listByAccount.mockResolvedValue([plan({ test_mode: 'fingerprint' })])
  })

  it('renders mismatch verdict, model comparison, confidence, action and detail', async () => {
    listResults.mockResolvedValue([{
      id: 9, plan_id: 1, status: 'failed', response_text: '', error_message: 'mismatch',
      latency_ms: 8421, started_at: '2026-10-03T00:00:00Z', finished_at: '', created_at: '',
      verdict: 'mismatch', expected_model: 'claude-sonnet-4-5', detected_model: 'gpt-5.6-sol',
      detected_model_name: 'GPT-5.6 Sol', detected_family: 'gpt', probability: 0.93,
      account_action: 'disabled', detail: { rounds: [{ decision: 'scored' }] },
    }])
    const wrapper = await openPanel()
    await wrapper.findAll('.cursor-pointer')[0].trigger('click')
    await flushPromises()

    const badge = wrapper.find('[data-testid="result-status-badge"]')
    expect(badge.text()).toBe('admin.scheduledTests.verdictMismatch')
    expect(badge.classes()).toContain('bg-red-100')
    const summary = wrapper.find('[data-testid="result-fingerprint-summary"]').text()
    expect(summary).toContain('gpt-5.6-sol')
    expect(summary).toContain('claude-sonnet-4-5')
    expect(summary).toContain('93%')
    expect(wrapper.find('[data-testid="result-account-action"]').text()).toBe('admin.scheduledTests.accountActionDisabled')

    expect(wrapper.find('[data-testid="result-detail-json"]').exists()).toBe(false)
    await wrapper.find('[data-testid="result-detail-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="result-detail-json"]').text()).toContain('"decision": "scored"')
  })

  it('falls back to the status badge when verdict is empty', async () => {
    listResults.mockResolvedValue([{
      id: 3, plan_id: 1, status: 'success', response_text: 'ok', error_message: '', latency_ms: 10,
      started_at: '2026-10-03T00:00:00Z', finished_at: '', created_at: '', verdict: '',
      expected_model: '', detected_model: '', detected_model_name: '', detected_family: '',
      probability: null, account_action: '', detail: null,
    }])
    const wrapper = await openPanel()
    await wrapper.findAll('.cursor-pointer')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="result-status-badge"]').text()).toBe('admin.scheduledTests.success')
    expect(wrapper.find('[data-testid="result-fingerprint-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-detail-toggle"]').exists()).toBe(false)
  })
})