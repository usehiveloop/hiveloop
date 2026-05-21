"use client"

import React from "react"

export interface Theme {
  name: string
  bg: string
  text: string
  muted: string
  primary: string
  primaryText: string
  secondary: string
  secondaryBorder: string
  secondaryText: string
  pillFrom: string
  pillVia: string
  pillTo: string
  glowLeft: string
  glowCenter: string
  glowRight: string
  navBg: string
  navBorder: string
}

export const ROSE_THEME: Theme = {
  name: "Rose",
  bg: "#FFFAFA",
  text: "#2D0A0F",
  muted: "#7C4A52",
  primary: "#881337",
  primaryText: "#FFFFFF",
  secondary: "#FFFFFF",
  secondaryBorder: "#FECDD3",
  secondaryText: "#881337",
  pillFrom: "#FB7185",
  pillVia: "#FDA4AF",
  pillTo: "#FECDD3",
  glowLeft: "#FFE4E6",
  glowCenter: "#FFF1F2",
  glowRight: "#FECDD3",
  navBg: "rgba(255,255,255,0.88)",
  navBorder: "rgba(136,19,55,0.07)",
}

/*
export const MIDNIGHT_THEME: Theme = {
  name: "Midnight",
  bg: "#F0F4F8",
  text: "#0A192F",
  muted: "#4A5568",
  primary: "#1E3A5F",
  primaryText: "#FFFFFF",
  secondary: "#FFFFFF",
  secondaryBorder: "#CBD5E0",
  secondaryText: "#1E3A5F",
  pillFrom: "#60A5FA",
  pillVia: "#818CF8",
  pillTo: "#A78BFA",
  glowLeft: "#BFDBFE",
  glowCenter: "#C7D2FE",
  glowRight: "#DDD6FE",
  navBg: "rgba(255,255,255,0.88)",
  navBorder: "rgba(30,58,95,0.08)",
}
*/

export const THEMES: Theme[] = [ROSE_THEME]
