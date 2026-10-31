/**
 * Admin Model Fingerprint Batch API endpoints
 * Async batch fingerprint runs over an account's model whitelist
 */

import { apiClient } from '../client'
import type {
  ModelFingerprintBatchRun,
  ModelFingerprintBatchRunDetail,
  CreateModelFingerprintBatchRequest
} from '@/types'

const basePath = (accountId: number) => `/admin/accounts/${accountId}/model-fingerprint-runs`

/**
 * Create (and start) a batch fingerprint run for an account
 * @param accountId - Account ID
 * @param req - Models snapshot, batch concurrency and detector config
 * @returns Created run with its pending per-model results
 */
export async function createRun(
  accountId: number,
  req: CreateModelFingerprintBatchRequest,
  idempotencyKey?: string
): Promise<ModelFingerprintBatchRunDetail> {
  const { data } = await apiClient.post<ModelFingerprintBatchRunDetail>(basePath(accountId), req, {
    timeout: 60000,
    headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined
  })
  return { run: data.run, results: data.results ?? [] }
}

/**
 * List recent batch fingerprint runs for an account (newest first)
 * @param accountId - Account ID
 * @param limit - Optional max number of runs
 */
export async function listRuns(accountId: number, limit?: number): Promise<ModelFingerprintBatchRun[]> {
  const { data } = await apiClient.get<ModelFingerprintBatchRun[]>(basePath(accountId), {
    params: limit ? { limit } : undefined
  })
  return data ?? []
}

/**
 * Get one run with its per-model results
 * @param accountId - Account ID
 * @param runId - Run ID
 */
export async function getRun(accountId: number, runId: number): Promise<ModelFingerprintBatchRunDetail> {
  const { data } = await apiClient.get<ModelFingerprintBatchRunDetail>(`${basePath(accountId)}/${runId}`)
  return { run: data.run, results: data.results ?? [] }
}

/**
 * Cancel an active run
 * @param accountId - Account ID
 * @param runId - Run ID
 * @returns Updated run
 */
export async function cancelRun(accountId: number, runId: number): Promise<ModelFingerprintBatchRun> {
  const { data } = await apiClient.post<ModelFingerprintBatchRun>(`${basePath(accountId)}/${runId}/cancel`)
  return data
}

export const modelFingerprintBatchAPI = {
  createRun,
  listRuns,
  getRun,
  cancelRun
}

export default modelFingerprintBatchAPI
