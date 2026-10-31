import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() }),
}))
vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: { syncUpstreamModels: vi.fn(), syncUpstreamModelsPreview: vi.fn() },
}))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))

const DialogStub = defineComponent({
  name: 'ModelFingerprintBatchDialog',
  props: { show: Boolean, accountId: Number, models: { type: Array, default: () => [] } },
  emits: ['close', 'run-finished'],
  template: '<div data-testid="dialog-stub" :data-show="String(show)"><button data-testid="stub-finish" @click="$emit(\'run-finished\', { id: 1 })" /></div>',
})

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const P = 'admin.accounts.modelFingerprintBatch'

function mountSelector(props: Record<string, unknown> = {}) {
  return mount(ModelWhitelistSelector, {
    props: { modelValue: [], platform: 'anthropic', ...props },
    global: { stubs: { ModelIcon: true, ModelFingerprintBatchDialog: DialogStub } },
  })
}

describe('ModelWhitelistSelector fingerprint button', () => {
  beforeEach(() => vi.clearAllMocks())

  it('is hidden without a saved account id (create / bulk / risk-control)', () => {
    expect(mountSelector({ modelValue: ['a'] }).find('[data-testid="run-model-fingerprint"]').exists()).toBe(false)
    expect(mountSelector({ modelValue: ['a'], accountId: 0 }).find('[data-testid="run-model-fingerprint"]').exists()).toBe(false)
  })

  it('is hidden when showFingerprintButton is false', () => {
    const wrapper = mountSelector({ modelValue: ['a'], accountId: 7, showFingerprintButton: false })
    expect(wrapper.find('[data-testid="run-model-fingerprint"]').exists()).toBe(false)
  })

  it('is disabled with a hint when the whitelist is empty', async () => {
    const wrapper = mountSelector({ modelValue: [], accountId: 7 })
    const button = wrapper.get('[data-testid="run-model-fingerprint"]')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe(`${P}.buttonRequiresModels`)
    await button.trigger('click')
    expect(wrapper.find('[data-testid="dialog-stub"]').exists()).toBe(false)
  })

  it('is placed between sync and clear buttons', () => {
    const wrapper = mountSelector({ modelValue: ['a'], accountId: 7 })
    const ids = wrapper.findAll('button').map(b => b.attributes('data-testid') || b.text())
    const syncIdx = ids.indexOf('sync-upstream-models')
    const fpIdx = ids.indexOf('run-model-fingerprint')
    const clearIdx = ids.indexOf('admin.accounts.clearAllModels')
    expect(syncIdx).toBeGreaterThanOrEqual(0)
    expect(fpIdx).toBeGreaterThan(syncIdx)
    expect(clearIdx).toBeGreaterThan(fpIdx)
  })

  it('opens the dialog with the current (unsaved) models snapshot and forwards run-finished', async () => {
    const wrapper = mountSelector({ modelValue: ['claude-a', 'claude-b'], accountId: 7 })
    const button = wrapper.get('[data-testid="run-model-fingerprint"]')
    expect(button.attributes('disabled')).toBeUndefined()
    await button.trigger('click')
    await flushPromises()
    const dialog = wrapper.getComponent(DialogStub)
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('accountId')).toBe(7)
    expect(dialog.props('models')).toEqual(['claude-a', 'claude-b'])

    await wrapper.get('[data-testid="stub-finish"]').trigger('click')
    expect(wrapper.emitted('fingerprint-run-finished')?.[0]?.[0]).toEqual({ id: 1 })

    dialog.vm.$emit('close')
    await flushPromises()
    expect(wrapper.getComponent(DialogStub).props('show')).toBe(false)
  })
})
