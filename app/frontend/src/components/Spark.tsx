// 小さい折れ線のグラフ。values を左から右へ並べ、max を上の端にする (mock/ の spark と同じ形)
export function Spark({ values, max, className = '' }: { values: number[]; max: number; className?: string }) {
  const n = Math.max(values.length, 2)
  const pts = values.map((v, i) => `${i},${(29 - Math.min(v, max) / (max || 1) * 27).toFixed(2)}`)
  return (
    <svg className={`spark ${className}`} viewBox={`0 0 ${n - 1} 30`} preserveAspectRatio="none">
      {pts.length > 1 && <>
        <path className="spark-area" d={`M0,30 L${pts.join(' L')} L${values.length - 1},30 Z`} />
        <polyline className="spark-line" points={pts.join(' ')} />
      </>}
    </svg>
  )
}

export function Meter({ value, alt = false }: { value: number; alt?: boolean }) {
  return <div className={`meter ${alt ? 'alt' : ''}`}><i style={{ width: `${Math.max(0, Math.min(100, value))}%` }} /></div>
}
