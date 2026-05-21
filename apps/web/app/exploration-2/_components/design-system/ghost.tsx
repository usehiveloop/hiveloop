"use client"

import React from "react"

interface GhostProps {
  color: string
  bgColor: string
  size?: number
  className?: string
}

export function Ghost({ color, bgColor, size = 64, className = "" }: GhostProps) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      style={{ color }}
      fill="currentColor"
      className={className}
    >
      <path
        d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z"
        fill="currentColor"
      />
      <ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill={bgColor} />
      <ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill={bgColor} />
      <path
        d="M 80 550 C 40 600, 0 620, -60 650 C -120 680, -140 720, -180 750 C -220 780, -240 820, -260 850 C -280 880, -300 920, -340 950"
        fill="none"
        stroke="currentColor"
        strokeWidth="54"
        strokeLinecap="round"
      />
    </svg>
  )
}
