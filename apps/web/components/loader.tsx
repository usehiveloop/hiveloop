"use client"

import { useState, useEffect } from "react"
import { AnimatePresence, motion } from "motion/react"
import { LogoMark } from "@/components/logo"

const LOADING_WORDS = [
  "Accomplishing",
  "Actioning",
  "Actualizing",
  "Baking",
  "Brewing",
  "Calculating",
  "Cerebrating",
  "Churning",
  "Clauding",
  "Coalescing",
  "Cogitating",
  "Computing",
  "Conjuring",
  "Considering",
  "Cooking",
  "Crafting",
  "Creating",
  "Crunching",
  "Deliberating",
  "Determining",
  "Doing",
  "Effecting",
  "Finagling",
  "Forging",
  "Forming",
  "Generating",
  "Hatching",
  "Herding",
  "Honking",
  "Hustling",
  "Ideating",
  "Inferring",
  "Manifesting",
  "Marinating",
  "Moseying",
  "Mulling",
  "Mustering",
  "Musing",
  "Noodling",
  "Percolating",
  "Pondering",
  "Processing",
  "Puttering",
  "Reticulating",
  "Ruminating",
  "Schlepping",
  "Shucking",
  "Simmering",
  "Smooshing",
  "Spinning",
  "Stewing",
  "Synthesizing",
  "Thinking",
  "Transmuting",
  "Vibing",
  "Working",
]

function randomWord() {
  return LOADING_WORDS[Math.floor(Math.random() * LOADING_WORDS.length)]
}

interface LoaderProps {
  title?: string
  description?: string
}

export function Loader({ title, description }: LoaderProps) {
  const [word, setWord] = useState(randomWord)

  useEffect(() => {
    if (title) return
    const interval = setInterval(() => setWord(randomWord()), 3000)
    return () => clearInterval(interval)
  }, [title])

  return (
    <div className="flex flex-col items-center justify-center gap-3 py-12">
      <LogoMark className="h-12 w-12 animate-[spin_1s_linear_infinite,pulse_2s_ease-in-out_infinite]" />
      <div className="text-center h-12 relative overflow-hidden">
        {title ? (
          <p className="text-lg font-medium text-foreground mt-4">{title}</p>
        ) : (
          <AnimatePresence mode="wait">
            <motion.p
              key={word}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: 0.3, ease: "easeInOut" as const }}
              className="text-lg font-medium text-foreground mt-4"
            >
              {word}...
            </motion.p>
          </AnimatePresence>
        )}
      </div>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
    </div>
  )
}
