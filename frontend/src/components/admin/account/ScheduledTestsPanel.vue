<template>
  <BaseDialog
    :show="show"
    :title="t('admin.scheduledTests.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- Add Plan Button -->
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.title') }}
        </p>
        <button
          @click="showAddForm = !showAddForm"
          class="btn btn-primary flex items-center gap-1.5 text-sm"
        >
          <Icon name="plus" size="sm" :stroke-width="2" />
          {{ t('admin.scheduledTests.addPlan') }}
        </button>
      </div>

      <!-- Add Plan Form -->
      <div
        v-if="showAddForm"
        class="rounded-xl border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-800 dark:bg-primary-900/20"
      >
        <div class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.scheduledTests.addPlan') }}
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.model') }}
            </label>
            <Select
              v-model="newPlan.model_id"
              :options="modelOptions"
              :placeholder="t('admin.scheduledTests.model')"
              :searchable="modelOptions.length > 5"
            />
          </div>
          <div>
            <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.cronExpression') }}
              <HelpTooltip>
                <template #trigger>
                  <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                    ?
                  </span>
                </template>
                <div class="space-y-1.5">
                  <p class="font-medium">{{ t('admin.scheduledTests.cronTooltipTitle') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipMeaning') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleEvery30Min') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleHourly') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleDaily') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipExampleWeekly') }}</p>
                  <p>{{ t('admin.scheduledTests.cronTooltipRange') }}</p>
                </div>
              </HelpTooltip>
            </label>
            <Input
              v-model="newPlan.cron_expression"
              :placeholder="'*/30 * * * *'"
              :hint="t('admin.scheduledTests.cronHelp')"
            />
          </div>
          <div>
            <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.maxResults') }}
              <HelpTooltip>
                <template #trigger>
                  <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                    ?
                  </span>
                </template>
                <div class="space-y-1.5">
                  <p class="font-medium">{{ t('admin.scheduledTests.maxResultsTooltipTitle') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipMeaning') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipBody') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipExample') }}</p>
                  <p>{{ t('admin.scheduledTests.maxResultsTooltipRange') }}</p>
                </div>
              </HelpTooltip>
            </label>
            <Input
              v-model="newPlan.max_results"
              type="number"
              placeholder="100"
            />
          </div>
          <div class="grid grid-cols-1 gap-3 sm:col-span-2 sm:grid-cols-2">
            <div class="flex items-start">
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle v-model="newPlan.enabled" />
                {{ t('admin.scheduledTests.enabled') }}
              </label>
            </div>
            <div class="flex items-start">
              <div>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <Toggle
                    data-testid="new-fingerprint-toggle"
                    :model-value="newPlan.test_mode === 'fingerprint'"
                    @update:model-value="(val: boolean) => (newPlan.test_mode = val ? 'fingerprint' : 'liveness')"
                  />
                  {{ t('admin.scheduledTests.fingerprintTest') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.scheduledTests.fingerprintTestHelp') }}
                </p>
              </div>
            </div>
            <div class="flex items-start">
              <div>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <Toggle v-model="newPlan.auto_recover" />
                  {{ t('admin.scheduledTests.autoRecover') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.scheduledTests.autoRecoverHelp') }}
                </p>
              </div>
            </div>
            <div class="flex items-start">
              <div>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <Toggle data-testid="new-auto-disable-toggle" v-model="newPlan.auto_disable" />
                  {{ t('admin.scheduledTests.autoDisable') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.scheduledTests.autoDisableHelp') }}
                </p>
              </div>
            </div>
          </div>
        </div>
        <div
          data-testid="new-detector-params"
          :class="[
            'mt-3 rounded-lg border border-indigo-100 bg-white/60 p-3 transition-opacity dark:border-indigo-900/50 dark:bg-dark-800/40',
            newDetectorDisabled ? 'pointer-events-none select-none opacity-50' : ''
          ]"
        >
          <div class="text-xs font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.scheduledTests.fingerprintParams') }}
          </div>
          <p class="mb-2 mt-0.5 text-xs text-gray-400 dark:text-gray-500">
            {{ t('admin.scheduledTests.fingerprintParamsHelp') }}
          </p>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorApi') }}
              </label>
              <Select
                data-testid="new-detector-api"
                v-model="newPlan.detector.api"
                :options="detectorApiOptions"
                :disabled="newDetectorDisabled"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorCount') }}
              </label>
              <Input
                data-testid="new-detector-count"
                v-model="newPlan.detector.count"
                type="number"
                placeholder="1-3"
                :disabled="newDetectorDisabled"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorParallel') }}
              </label>
              <Input
                data-testid="new-detector-parallel"
                v-model="newPlan.detector.parallel"
                type="number"
                placeholder="1-3"
                :disabled="newDetectorDisabled"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorTimeout') }}
              </label>
              <Input
                data-testid="new-detector-timeout_sec"
                v-model="newPlan.detector.timeout_sec"
                type="number"
                placeholder="1-600"
                :disabled="newDetectorDisabled"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorRounds') }}
              </label>
              <Input
                data-testid="new-detector-rounds"
                v-model="newPlan.detector.rounds"
                type="number"
                placeholder="1-5"
                :disabled="newDetectorDisabled"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorEffort') }}
              </label>
              <Input
                data-testid="new-detector-effort"
                v-model="newPlan.detector.effort"
                :placeholder="t('admin.scheduledTests.detectorEffortPlaceholder')"
                :disabled="newDetectorDisabled"
              />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('admin.scheduledTests.detectorServiceTier') }}
              </label>
              <Select
                data-testid="new-detector-service-tier"
                v-model="newPlan.detector.service_tier"
                :options="detectorServiceTierOptions"
                :disabled="newDetectorDisabled"
              />
            </div>
          </div>
          <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle v-model="newPlan.detector.no_stream" :disabled="newDetectorDisabled" />
                {{ t('admin.scheduledTests.detectorNoStream') }}
              </label>
            </div>
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle v-model="newPlan.detector.strict" :disabled="newDetectorDisabled" />
                {{ t('admin.scheduledTests.detectorStrict') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                {{ t('admin.scheduledTests.detectorStrictHelp') }}
              </p>
            </div>
            <div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <Toggle v-model="newPlan.detector.allow_family_match" :disabled="newDetectorDisabled" />
                {{ t('admin.scheduledTests.detectorAllowFamilyMatch') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                {{ t('admin.scheduledTests.detectorAllowFamilyMatchHelp') }}
              </p>
            </div>
          </div>
        </div>
        <div class="mt-3 flex justify-end gap-2">
          <button
            @click="showAddForm = false; resetNewPlan()"
            class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            @click="handleCreate"
            :disabled="!newPlan.model_id || !newPlan.cron_expression || creating"
            class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Icon v-if="creating" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
            {{ t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="md" class="animate-spin text-gray-400" :stroke-width="2" />
        <span class="ml-2 text-sm text-gray-500">{{ t('common.loading') }}...</span>
      </div>

      <!-- Empty State -->
      <div
        v-else-if="plans.length === 0"
        class="rounded-xl border border-dashed border-gray-300 py-10 text-center dark:border-dark-600"
      >
        <Icon name="calendar" size="lg" class="mx-auto mb-2 text-gray-400" :stroke-width="1.5" />
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.noPlans') }}
        </p>
      </div>

      <!-- Plans List -->
      <div v-else class="space-y-3">
        <div
          v-for="plan in plans"
          :key="plan.id"
          class="rounded-xl border border-gray-200 bg-white transition-all dark:border-dark-600 dark:bg-dark-800"
        >
          <!-- Plan Header -->
          <div
            class="flex cursor-pointer items-center justify-between px-4 py-3"
            @click="toggleExpand(plan.id)"
          >
            <div class="flex flex-1 items-center gap-4">
              <!-- Model -->
              <div class="min-w-0">
                <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ plan.model_id }}
                </div>
                <div class="mt-0.5 font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ plan.cron_expression }}
                </div>
              </div>

              <!-- Enabled Toggle -->
              <div class="flex items-center gap-1.5" @click.stop>
                <Toggle
                  :model-value="plan.enabled"
                  @update:model-value="(val: boolean) => handleToggleEnabled(plan, val)"
                />
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ plan.enabled ? t('admin.scheduledTests.enabled') : '' }}
                </span>
              </div>

              <!-- Auto Recover Badge -->
              <span
                v-if="plan.auto_recover"
                class="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-400"
              >
                {{ t('admin.scheduledTests.autoRecover') }}
              </span>

              <!-- Fingerprint Badge -->
              <span
                v-if="plan.test_mode === 'fingerprint'"
                data-testid="plan-fingerprint-badge"
                class="inline-flex items-center rounded-full bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300"
              >
                {{ t('admin.scheduledTests.fingerprintTest') }}
              </span>

              <!-- Auto Disable Badge -->
              <span
                v-if="plan.auto_disable"
                data-testid="plan-auto-disable-badge"
                class="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/20 dark:text-amber-400"
              >
                {{ t('admin.scheduledTests.autoDisable') }}
              </span>
            </div>

            <div class="flex items-center gap-3">
              <!-- Last Run -->
              <div v-if="plan.last_run_at" class="hidden text-right text-xs text-gray-500 dark:text-gray-400 sm:block">
                <div>{{ t('admin.scheduledTests.lastRun') }}</div>
                <div>{{ formatDateTime(plan.last_run_at) }}</div>
              </div>

              <!-- Next Run -->
              <div v-if="plan.next_run_at" class="hidden text-right text-xs text-gray-500 dark:text-gray-400 sm:block">
                <div>{{ t('admin.scheduledTests.nextRun') }}</div>
                <div>{{ formatDateTime(plan.next_run_at) }}</div>
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-1" @click.stop>
                <button
                  @click="startEdit(plan)"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-500 dark:hover:bg-blue-900/20"
                  :title="t('admin.scheduledTests.editPlan')"
                >
                  <Icon name="edit" size="sm" :stroke-width="2" />
                </button>
                <button
                  @click="confirmDeletePlan(plan)"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                  :title="t('admin.scheduledTests.deletePlan')"
                >
                  <Icon name="trash" size="sm" :stroke-width="2" />
                </button>
              </div>

              <!-- Expand indicator -->
              <Icon
                name="chevronDown"
                size="sm"
                :class="[
                  'text-gray-400 transition-transform duration-200',
                  expandedPlanId === plan.id ? 'rotate-180' : ''
                ]"
              />
            </div>
          </div>

          <!-- Edit Form -->
          <div
            v-if="editingPlanId === plan.id"
            class="border-t border-blue-100 bg-blue-50/50 px-4 py-3 dark:border-blue-900 dark:bg-blue-900/10"
            @click.stop
          >
            <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.editPlan') }}
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.model') }}
                </label>
                <Select
                  v-model="editForm.model_id"
                  :options="modelOptions"
                  :placeholder="t('admin.scheduledTests.model')"
                  :searchable="modelOptions.length > 5"
                />
              </div>
              <div>
                <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.cronExpression') }}
                  <HelpTooltip>
                    <template #trigger>
                      <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                        ?
                      </span>
                    </template>
                    <div class="space-y-1.5">
                      <p class="font-medium">{{ t('admin.scheduledTests.cronTooltipTitle') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipMeaning') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleEvery30Min') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleHourly') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleDaily') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipExampleWeekly') }}</p>
                      <p>{{ t('admin.scheduledTests.cronTooltipRange') }}</p>
                    </div>
                  </HelpTooltip>
                </label>
                <Input
                  v-model="editForm.cron_expression"
                  :placeholder="'*/30 * * * *'"
                  :hint="t('admin.scheduledTests.cronHelp')"
                />
              </div>
              <div>
                <label class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.maxResults') }}
                  <HelpTooltip>
                    <template #trigger>
                      <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400">
                        ?
                      </span>
                    </template>
                    <div class="space-y-1.5">
                      <p class="font-medium">{{ t('admin.scheduledTests.maxResultsTooltipTitle') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipMeaning') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipBody') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipExample') }}</p>
                      <p>{{ t('admin.scheduledTests.maxResultsTooltipRange') }}</p>
                    </div>
                  </HelpTooltip>
                </label>
                <Input
                  v-model="editForm.max_results"
                  type="number"
                  placeholder="100"
                />
              </div>
              <div class="grid grid-cols-1 gap-3 sm:col-span-2 sm:grid-cols-2">
                <div class="flex items-start">
                  <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <Toggle v-model="editForm.enabled" />
                    {{ t('admin.scheduledTests.enabled') }}
                  </label>
                </div>
                <div class="flex items-start">
                  <div>
                    <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <Toggle
                        data-testid="edit-fingerprint-toggle"
                        :model-value="editForm.test_mode === 'fingerprint'"
                        @update:model-value="(val: boolean) => (editForm.test_mode = val ? 'fingerprint' : 'liveness')"
                      />
                      {{ t('admin.scheduledTests.fingerprintTest') }}
                    </label>
                    <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                      {{ t('admin.scheduledTests.fingerprintTestHelp') }}
                    </p>
                  </div>
                </div>
                <div class="flex items-start">
                  <div>
                    <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <Toggle v-model="editForm.auto_recover" />
                      {{ t('admin.scheduledTests.autoRecover') }}
                    </label>
                    <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                      {{ t('admin.scheduledTests.autoRecoverHelp') }}
                    </p>
                  </div>
                </div>
                <div class="flex items-start">
                  <div>
                    <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <Toggle data-testid="edit-auto-disable-toggle" v-model="editForm.auto_disable" />
                      {{ t('admin.scheduledTests.autoDisable') }}
                    </label>
                    <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                      {{ t('admin.scheduledTests.autoDisableHelp') }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
            <div
              data-testid="edit-detector-params"
              :class="[
                'mt-3 rounded-lg border border-indigo-100 bg-white/60 p-3 transition-opacity dark:border-indigo-900/50 dark:bg-dark-800/40',
                editDetectorDisabled ? 'pointer-events-none select-none opacity-50' : ''
              ]"
              :disabled="editDetectorDisabled"
            >
              <div class="text-xs font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.scheduledTests.fingerprintParams') }}
              </div>
              <p class="mb-2 mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                {{ t('admin.scheduledTests.fingerprintParamsHelp') }}
              </p>
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorApi') }}
                  </label>
                  <Select
                    data-testid="edit-detector-api"
                    v-model="editForm.detector.api"
                    :options="detectorApiOptions"
                    :disabled="editDetectorDisabled"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorCount') }}
                  </label>
                  <Input
                    data-testid="edit-detector-count"
                    v-model="editForm.detector.count"
                    type="number"
                    placeholder="1-3"
                    :disabled="editDetectorDisabled"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorParallel') }}
                  </label>
                  <Input
                    data-testid="edit-detector-parallel"
                    v-model="editForm.detector.parallel"
                    type="number"
                    placeholder="1-3"
                    :disabled="editDetectorDisabled"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorTimeout') }}
                  </label>
                  <Input
                    data-testid="edit-detector-timeout_sec"
                    v-model="editForm.detector.timeout_sec"
                    type="number"
                    placeholder="1-600"
                    :disabled="editDetectorDisabled"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorRounds') }}
                  </label>
                  <Input
                    data-testid="edit-detector-rounds"
                    v-model="editForm.detector.rounds"
                    type="number"
                    placeholder="1-5"
                    :disabled="editDetectorDisabled"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorEffort') }}
                  </label>
                  <Input
                    data-testid="edit-detector-effort"
                    v-model="editForm.detector.effort"
                    :placeholder="t('admin.scheduledTests.detectorEffortPlaceholder')"
                    :disabled="editDetectorDisabled"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ t('admin.scheduledTests.detectorServiceTier') }}
                  </label>
                  <Select
                    data-testid="edit-detector-service-tier"
                    v-model="editForm.detector.service_tier"
                    :options="detectorServiceTierOptions"
                    :disabled="editDetectorDisabled"
                  />
                </div>
              </div>
              <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
                <div>
                  <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <Toggle v-model="editForm.detector.no_stream" :disabled="editDetectorDisabled" />
                    {{ t('admin.scheduledTests.detectorNoStream') }}
                  </label>
                </div>
                <div>
                  <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <Toggle v-model="editForm.detector.strict" :disabled="editDetectorDisabled" />
                    {{ t('admin.scheduledTests.detectorStrict') }}
                  </label>
                  <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('admin.scheduledTests.detectorStrictHelp') }}
                  </p>
                </div>
                <div>
                  <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <Toggle v-model="editForm.detector.allow_family_match" :disabled="editDetectorDisabled" />
                    {{ t('admin.scheduledTests.detectorAllowFamilyMatch') }}
                  </label>
                  <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('admin.scheduledTests.detectorAllowFamilyMatchHelp') }}
                  </p>
                </div>
              </div>
            </div>
            <div class="mt-3 flex justify-end gap-2">
              <button
                @click="cancelEdit"
                class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
              >
                {{ t('common.cancel') }}
              </button>
              <button
                @click="handleEdit"
                :disabled="!editForm.model_id || !editForm.cron_expression || updating"
                class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon v-if="updating" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
                {{ t('common.save') }}
              </button>
            </div>
          </div>

          <!-- Expanded Results Section -->
          <div
            v-if="expandedPlanId === plan.id"
            class="border-t border-gray-100 px-4 py-3 dark:border-dark-700"
          >
            <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.results') }}
            </div>

            <!-- Results Loading -->
            <div v-if="loadingResults" class="flex items-center justify-center py-4">
              <Icon name="refresh" size="sm" class="animate-spin text-gray-400" :stroke-width="2" />
              <span class="ml-2 text-xs text-gray-500">{{ t('common.loading') }}...</span>
            </div>

            <!-- No Results -->
            <div
              v-else-if="results.length === 0"
              class="py-4 text-center text-xs text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.scheduledTests.noResults') }}
            </div>

            <!-- Results List -->
            <div v-else class="max-h-64 space-y-2 overflow-y-auto">
              <div
                v-for="result in results"
                :key="result.id"
                class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <!-- Status / Verdict Badge -->
                    <span
                      data-testid="result-status-badge"
                      :class="['inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', resultBadgeClass(result)]"
                    >
                      {{ resultBadgeLabel(result) }}
                    </span>

                    <!-- Account Action -->
                    <span
                      v-if="result.account_action === 'disabled'"
                      data-testid="result-account-action"
                      class="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/20 dark:text-amber-400"
                    >
                      {{ t('admin.scheduledTests.accountActionDisabled') }}
                    </span>
                    <span
                      v-else-if="result.account_action === 'restored'"
                      data-testid="result-account-action"
                      class="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-400"
                    >
                      {{ t('admin.scheduledTests.accountActionRestored') }}
                    </span>

                    <!-- Latency -->
                    <span v-if="result.latency_ms > 0" class="text-xs text-gray-500 dark:text-gray-400">
                      {{ result.latency_ms }}ms
                    </span>
                  </div>

                  <!-- Started At -->
                  <span class="text-xs text-gray-400">
                    {{ formatDateTime(result.started_at) }}
                  </span>
                </div>

                <!-- Fingerprint verdict summary -->
                <div
                  v-if="result.verdict && (result.detected_model || result.expected_model || result.probability != null)"
                  data-testid="result-fingerprint-summary"
                  class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-600 dark:text-gray-400"
                >
                  <span v-if="result.detected_model || result.expected_model">
                    <span :title="result.detected_model_name || undefined">
                      {{ t('admin.scheduledTests.detectedModel') }}: <span class="font-mono">{{ result.detected_model || '-' }}</span>
                    </span>
                    <span class="mx-1 text-gray-400">·</span>
                    <span>
                      {{ t('admin.scheduledTests.expectedModel') }}: <span class="font-mono">{{ result.expected_model || '-' }}</span>
                    </span>
                  </span>
                  <span v-if="result.probability != null">
                    {{ t('admin.scheduledTests.confidence') }} {{ formatProbability(result.probability) }}
                  </span>
                  <span v-if="matchNote(result)" class="text-violet-600 dark:text-violet-400" data-testid="result-match-note">
                    {{ matchNote(result) }}
                  </span>
                </div>

                <!-- Response / Error (collapsible) -->
                <div v-if="result.error_message" class="mt-2">
                  <div
                    class="cursor-pointer text-xs font-medium text-red-600 dark:text-red-400"
                    @click="toggleResultDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.errorMessage') }}
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedResultIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedResultIds.has(result.id)"
                    class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300"
                  >{{ result.error_message }}</pre>
                </div>
                <div v-else-if="result.response_text" class="mt-2">
                  <div
                    class="cursor-pointer text-xs font-medium text-gray-600 dark:text-gray-400"
                    @click="toggleResultDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.responseText') }}
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedResultIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedResultIds.has(result.id)"
                    class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-gray-100 p-2 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300"
                  >{{ result.response_text }}</pre>
                </div>

                <!-- Fingerprint detail (collapsible) -->
                <div v-if="hasDetail(result)" class="mt-2">
                  <div
                    data-testid="result-detail-toggle"
                    class="cursor-pointer text-xs font-medium text-indigo-600 dark:text-indigo-400"
                    @click="toggleResultFingerprintDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.resultDetail') }}
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedDetailIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedDetailIds.has(result.id)"
                    data-testid="result-detail-json"
                    class="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded bg-indigo-50 p-2 text-xs text-gray-700 dark:bg-indigo-900/20 dark:text-gray-300"
                  >{{ formatDetail(result.detail) }}</pre>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.scheduledTests.deletePlan')"
      :message="t('admin.scheduledTests.confirmDelete')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import { Icon } from '@/components/icons'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import type { ScheduledTestPlan, ScheduledTestResult, ScheduledTestDetectorConfig } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  show: boolean
  accountId: number | null
  modelOptions: SelectOption[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

// State
const loading = ref(false)
const creating = ref(false)
const loadingResults = ref(false)
const plans = ref<ScheduledTestPlan[]>([])
const results = ref<ScheduledTestResult[]>([])
const expandedPlanId = ref<number | null>(null)
let resultsRequestId = 0
watch(expandedPlanId, () => {
  resultsRequestId++
  results.value = []
  loadingResults.value = false
}, { flush: 'sync' })
onBeforeUnmount(() => { resultsRequestId++ })
const expandedResultIds = reactive(new Set<number>())
const expandedDetailIds = reactive(new Set<number>())
const showAddForm = ref(false)
const showDeleteConfirm = ref(false)
const deletingPlan = ref<ScheduledTestPlan | null>(null)
const editingPlanId = ref<number | null>(null)
const updating = ref(false)
type TestMode = ScheduledTestPlan['test_mode']

// Form-side representation of detector_config: numeric fields are kept as
// strings because <Input> emits strings; they are converted on submit.
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

const fromDetectorForm = (form: DetectorForm): ScheduledTestDetectorConfig => {
  const defaults = defaultDetectorConfig()
  const strict = !!form.strict
  return {
    api: DETECTOR_APIS.includes(form.api as ScheduledTestDetectorConfig['api'])
      ? (form.api as ScheduledTestDetectorConfig['api'])
      : defaults.api,
    // strict mode requires a complete 3/3 sample set
    count: strict ? 3 : clampInt(form.count, 1, 3, defaults.count),
    parallel: clampInt(form.parallel, 1, 3, defaults.parallel),
    timeout_sec: clampInt(form.timeout_sec, 1, 600, defaults.timeout_sec),
    rounds: clampInt(form.rounds, 1, 5, defaults.rounds),
    no_stream: !!form.no_stream,
    strict,
    effort: (form.effort ?? '').trim(),
    service_tier: SERVICE_TIERS.includes(form.service_tier as ScheduledTestDetectorConfig['service_tier'])
      ? (form.service_tier as ScheduledTestDetectorConfig['service_tier'])
      : defaults.service_tier,
    allow_family_match: !!form.allow_family_match
  }
}

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

const editForm = reactive({
  model_id: '' as string,
  cron_expression: '' as string,
  max_results: '100' as string,
  enabled: true,
  auto_recover: false,
  test_mode: 'liveness' as TestMode,
  auto_disable: false,
  detector: toDetectorForm()
})

const newPlan = reactive({
  model_id: '' as string,
  cron_expression: '' as string,
  max_results: '100' as string,
  enabled: true,
  auto_recover: false,
  test_mode: 'liveness' as TestMode,
  auto_disable: false,
  detector: toDetectorForm()
})

const newDetectorDisabled = computed(() => newPlan.test_mode !== 'fingerprint')
const editDetectorDisabled = computed(() => editForm.test_mode !== 'fingerprint')

const resetNewPlan = () => {
  newPlan.model_id = ''
  newPlan.cron_expression = ''
  newPlan.max_results = '100'
  newPlan.enabled = true
  newPlan.auto_recover = false
  newPlan.test_mode = 'liveness'
  newPlan.auto_disable = false
  newPlan.detector = toDetectorForm()
}

// Load plans when dialog opens
watch(
  () => props.show,
  async (visible) => {
    if (visible && props.accountId) {
      await loadPlans()
    } else {
      plans.value = []
      results.value = []
      expandedPlanId.value = null
      expandedResultIds.clear()
      expandedDetailIds.clear()
      showAddForm.value = false
      showDeleteConfirm.value = false
    }
  }
)

const loadPlans = async () => {
  if (!props.accountId) return
  loading.value = true
  try {
    plans.value = await adminAPI.scheduledTests.listByAccount(props.accountId)
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to load plans')
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  if (!props.accountId || !newPlan.model_id || !newPlan.cron_expression) return
  creating.value = true
  try {
    const maxResults = Number(newPlan.max_results) || 100
    await adminAPI.scheduledTests.create({
      account_id: props.accountId,
      model_id: newPlan.model_id,
      cron_expression: newPlan.cron_expression,
      enabled: newPlan.enabled,
      max_results: maxResults,
      auto_recover: newPlan.auto_recover,
      test_mode: newPlan.test_mode,
      auto_disable: newPlan.auto_disable,
      detector_config: fromDetectorForm(newPlan.detector)
    })
    appStore.showSuccess(t('admin.scheduledTests.createSuccess'))
    showAddForm.value = false
    resetNewPlan()
    await loadPlans()
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to create plan')
  } finally {
    creating.value = false
  }
}

const handleToggleEnabled = async (plan: ScheduledTestPlan, enabled: boolean) => {
  try {
    const updated = await adminAPI.scheduledTests.update(plan.id, { enabled })
    const index = plans.value.findIndex((p) => p.id === plan.id)
    if (index !== -1) {
      plans.value[index] = updated
    }
    appStore.showSuccess(t('admin.scheduledTests.updateSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to update plan')
  }
}

const startEdit = (plan: ScheduledTestPlan) => {
  editingPlanId.value = plan.id
  editForm.model_id = plan.model_id
  editForm.cron_expression = plan.cron_expression
  editForm.max_results = String(plan.max_results)
  editForm.enabled = plan.enabled
  editForm.auto_recover = plan.auto_recover
  editForm.test_mode = plan.test_mode === 'fingerprint' ? 'fingerprint' : 'liveness'
  editForm.auto_disable = !!plan.auto_disable
  editForm.detector = toDetectorForm(plan.detector_config)
}

const cancelEdit = () => {
  editingPlanId.value = null
}

const handleEdit = async () => {
  if (!editingPlanId.value || !editForm.model_id || !editForm.cron_expression) return
  updating.value = true
  try {
    const updated = await adminAPI.scheduledTests.update(editingPlanId.value, {
      model_id: editForm.model_id,
      cron_expression: editForm.cron_expression,
      max_results: Number(editForm.max_results) || 100,
      enabled: editForm.enabled,
      auto_recover: editForm.auto_recover,
      test_mode: editForm.test_mode,
      auto_disable: editForm.auto_disable,
      detector_config: fromDetectorForm(editForm.detector)
    })
    const index = plans.value.findIndex((p) => p.id === editingPlanId.value)
    if (index !== -1) {
      plans.value[index] = updated
    }
    appStore.showSuccess(t('admin.scheduledTests.updateSuccess'))
    editingPlanId.value = null
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to update plan')
  } finally {
    updating.value = false
  }
}

const confirmDeletePlan = (plan: ScheduledTestPlan) => {
  deletingPlan.value = plan
  showDeleteConfirm.value = true
}

// ---------- result badges / detail helpers ----------
const VERDICT_BADGES: Record<string, { cls: string; key: string }> = {
  match: { cls: 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400', key: 'admin.scheduledTests.verdictMatch' },
  mismatch: { cls: 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400', key: 'admin.scheduledTests.verdictMismatch' },
  invalid_reply: { cls: 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400', key: 'admin.scheduledTests.verdictInvalidReply' },
  error: { cls: 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400', key: 'admin.scheduledTests.verdictError' },
  unsupported: { cls: 'bg-gray-200 text-gray-700 dark:bg-dark-600 dark:text-gray-300', key: 'admin.scheduledTests.verdictUnsupported' },
}

const STATUS_BADGES: Record<string, { cls: string; key: string }> = {
  success: { cls: 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400', key: 'admin.scheduledTests.success' },
  running: { cls: 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400', key: 'admin.scheduledTests.running' },
  failed: { cls: 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400', key: 'admin.scheduledTests.failed' },
}

const resultBadgeClass = (result: ScheduledTestResult): string => {
  const badge = (result.verdict && VERDICT_BADGES[result.verdict]) || STATUS_BADGES[result.status] || STATUS_BADGES.failed
  return badge.cls
}

const resultBadgeLabel = (result: ScheduledTestResult): string => {
  const badge = (result.verdict && VERDICT_BADGES[result.verdict]) || STATUS_BADGES[result.status] || STATUS_BADGES.failed
  return t(badge.key)
}

const formatProbability = (probability: number): string => `${Math.round(probability * 100)}%`

const matchNote = (result: ScheduledTestResult): string => {
  const detail = result.detail as Record<string, unknown> | null
  const expectation = detail && typeof detail === 'object' ? (detail as Record<string, unknown>).expectation : null
  if (!expectation || typeof expectation !== 'object') return ''
  const meta = expectation as Record<string, unknown>
  if (meta.matched_via === 'alias') return t('admin.scheduledTests.matchedViaAlias')
  if (meta.matched_via === 'family') return t('admin.scheduledTests.matchedViaFamily')
  return ''
}

const hasDetail = (result: ScheduledTestResult): boolean => {
  if (result.detail == null) return false
  if (typeof result.detail === 'object') return Object.keys(result.detail).length > 0
  return true
}

const formatDetail = (detail: ScheduledTestResult['detail']): string => {
  try {
    return JSON.stringify(detail, null, 2)
  } catch {
    return String(detail)
  }
}

const toggleResultFingerprintDetail = (resultId: number) => {
  if (expandedDetailIds.has(resultId)) {
    expandedDetailIds.delete(resultId)
  } else {
    expandedDetailIds.add(resultId)
  }
}

const handleDelete = async () => {
  if (!deletingPlan.value) return
  try {
    await adminAPI.scheduledTests.delete(deletingPlan.value.id)
    appStore.showSuccess(t('admin.scheduledTests.deleteSuccess'))
    plans.value = plans.value.filter((p) => p.id !== deletingPlan.value!.id)
    if (expandedPlanId.value === deletingPlan.value.id) {
      expandedPlanId.value = null
      results.value = []
    }
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to delete plan')
  } finally {
    showDeleteConfirm.value = false
    deletingPlan.value = null
  }
}

const toggleExpand = async (planId: number) => {
  if (expandedPlanId.value === planId) {
    expandedPlanId.value = null
    results.value = []
    expandedResultIds.clear()
    return
  }
  expandedPlanId.value = planId
  expandedResultIds.clear()
  loadingResults.value = true
  const requestId = resultsRequestId
  try {
    const data = await adminAPI.scheduledTests.listResults(planId, 20)
    if (requestId !== resultsRequestId) return
    results.value = data
  } catch (error: any) {
    if (requestId !== resultsRequestId) return
    appStore.showError(error?.message || 'Failed to load results')
    results.value = []
  } finally {
    if (requestId === resultsRequestId) loadingResults.value = false
  }
}

const toggleResultDetail = (resultId: number) => {
  if (expandedResultIds.has(resultId)) {
    expandedResultIds.delete(resultId)
  } else {
    expandedResultIds.add(resultId)
  }
}
</script>