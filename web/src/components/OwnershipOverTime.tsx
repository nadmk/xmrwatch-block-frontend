import React, { useEffect, useMemo, useRef, useState } from 'react'
import * as echarts from 'echarts'
import { Block } from '../lib/api'
import { fetchBlockHeader } from '../lib/api'

function bucketTs(ts: number, stepSec: number) {
  return Math.floor(ts / stepSec) * stepSec
}

export default function OwnershipOverTime({ blocks, step = 3600 }: { blocks: Block[]; step?: number }) {
  const ref = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)
  const [enriched, setEnriched] = useState<Block[]>(blocks)
  const [cache] = useState(() => new Map<number, number>()) // height -> timestamp

  useEffect(() => {
    let cancelled = false
    async function enrich() {
      const result: Block[] = [...blocks]
      const promises: Promise<void>[] = []
      for (let i = 0; i < result.length; i++) {
        const b = result[i]
        if (b.pool === 'Unknown' && (!b.timestamp || b.timestamp === 0)) {
          const h = b.height
          if (cache.has(h)) {
            result[i] = { ...b, timestamp: cache.get(h)! }
          } else {
            promises.push(
              fetchBlockHeader(h).then((hdr) => {
                if (hdr && hdr.status === 'OK' && hdr.timestamp) {
                  cache.set(h, hdr.timestamp)
                }
              }).catch(() => {})
            )
          }
        }
      }
      if (promises.length) await Promise.allSettled(promises)
      // second pass to apply cache
      for (let i = 0; i < result.length; i++) {
        const b = result[i]
        if (b.pool === 'Unknown' && (!b.timestamp || b.timestamp === 0)) {
          const h = b.height
          if (cache.has(h)) result[i] = { ...b, timestamp: cache.get(h)! }
        }
      }
      if (!cancelled) setEnriched(result)
    }
    enrich()
    return () => { cancelled = true }
  }, [blocks])

  const { times, series } = useMemo(() => {
    const byPool: Record<string, Record<number, number>> = {}
    const timesSet = new Set<number>()
    for (const b of enriched) {
      const ts = b.timestamp || 0
      const t = bucketTs(ts, step)
      timesSet.add(t)
      if (!byPool[b.pool]) byPool[b.pool] = {}
      byPool[b.pool][t] = (byPool[b.pool][t] || 0) + 1
    }
    const times = Array.from(timesSet).sort((a, b) => a - b)
    const series = Object.entries(byPool).map(([pool, buckets]) => ({
      name: pool,
      type: 'line' as const,
      stack: 'ownership',
      areaStyle: {},
      smooth: true,
      emphasis: { focus: 'series' as const },
      data: times.map(t => buckets[t] || 0),
    }))
    return { times, series }
  }, [enriched, step])

  useEffect(() => {
    if (!ref.current) return
    if (!chartRef.current) chartRef.current = echarts.init(ref.current)
    const option: echarts.EChartsOption = {
      backgroundColor: 'transparent',
      tooltip: { trigger: 'axis' },
      legend: { textStyle: { color: '#cbd5e1' } },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: times.map(t => t ? new Date(t * 1000).toLocaleString() : 'Unknown time'),
        axisLabel: { color: '#94a3b8' },
        axisLine: { lineStyle: { color: '#334155' } },
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: '#94a3b8' },
        splitLine: { lineStyle: { color: '#1f2937' } },
      },
      series,
    }
    chartRef.current.setOption(option)
    const ch = chartRef.current
    const onResize = () => ch.resize()
    window.addEventListener('resize', onResize)
    return () => { window.removeEventListener('resize', onResize) }
  }, [times, series])

  return <div ref={ref} style={{ width: '100%', height: 320 }} />
}
