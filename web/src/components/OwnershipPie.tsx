import React, { useEffect, useRef } from 'react'
import * as echarts from 'echarts'
import { Ownership } from '../lib/api'

export default function OwnershipPie({ data }: { data: Ownership[] }) {
  const ref = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!ref.current) return
    if (!chartRef.current) {
      chartRef.current = echarts.init(ref.current, undefined, { renderer: 'canvas' })
    }
    const option: echarts.EChartsOption = {
      backgroundColor: 'transparent',
      tooltip: { trigger: 'item', formatter: '{b}: {d}%' },
      series: [
        {
          type: 'pie',
          radius: ['40%', '70%'],
          avoidLabelOverlap: true,
          label: { show: true, color: '#e5e7eb' },
          labelLine: { show: true },
          emphasis: { label: { show: true, fontSize: 16, fontWeight: 'bold' } },
          data: data.map(d => ({ name: d.pool, value: d.percentage })),
        },
      ],
    }
    chartRef.current.setOption(option)
    const ch = chartRef.current
    const onResize = () => ch.resize()
    window.addEventListener('resize', onResize)
    return () => { window.removeEventListener('resize', onResize) }
  }, [data])

  return <div ref={ref} style={{ width: '100%', height: 320 }} />
}
