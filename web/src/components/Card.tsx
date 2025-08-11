import { PropsWithChildren } from 'react'

export default function Card({ children, className = '' }: PropsWithChildren<{ className?: string }>) {
  return (
    <div className={`card p-4 ${className}`}>
      {children}
    </div>
  )
}
