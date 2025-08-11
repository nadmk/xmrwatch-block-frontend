import axios from 'axios'

const apiBase = '' // same origin when served via Go; for dev, you can set VITE_API_BASE

export const client = axios.create({
  baseURL: (import.meta as any).env?.VITE_API_BASE || apiBase,
  timeout: 15000,
})

export type Block = {
  height: number
  id: string
  timestamp: number
  reward: number
  pool: string
  valid: boolean
  miner: string
}

export type Ownership = {
  pool: string
  count: number
  percentage: number
}

export async function fetchBlocks(params: { limit?: number; onlyValid?: boolean; since?: number } = {}) {
  const res = await client.get<{ blocks: Block[] }>(`/api/blocks`, { params })
  return res.data.blocks
}

export async function fetchOwnership(params: { lastN?: number; since?: number; onlyValid?: boolean } = {}) {
  const res = await client.get<{ ownership: Ownership[] }>(`/api/ownership`, { params })
  return res.data.ownership
}

export async function fetchPools() {
  const res = await client.get<{ pools: string[] }>(`/api/pools`)
  return res.data.pools
}

export type BlockHeader = {
  status: string
  height: number
  timestamp: number
  reward: number
  hash: string
}

export async function fetchBlockHeader(height: number): Promise<BlockHeader> {
  const res = await client.get(`/api/block_header`, { params: { height } })
  return res.data as BlockHeader
}
