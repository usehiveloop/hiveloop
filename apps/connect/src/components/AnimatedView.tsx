import { type ReactNode } from 'react'
import { motion } from 'motion/react'

const SLIDE_OFFSET = 60

interface Props {
  viewKey: string
  direction: string
  children: ReactNode
}

export function AnimatedView({ viewKey, direction, children }: Props) {
  return (
    <motion.div
      key={viewKey}
      custom={direction}
      initial="enter"
      animate="center"
      exit="exit"
      variants={{
        enter: (dir: string) => ({
          x: dir === 'forward' ? SLIDE_OFFSET : -SLIDE_OFFSET,
          opacity: 0,
        }),
        center: { x: 0, opacity: 1 },
        exit: (dir: string) => ({
          x: dir === 'forward' ? -SLIDE_OFFSET : SLIDE_OFFSET,
          opacity: 0,
        }),
      }}
      transition={{ duration: 0.2, ease: [0.25, 0.1, 0.25, 1] }}
      className="absolute inset-0 p-[inherit]"
    >
      {children}
    </motion.div>
  )
}

export function FadeView({ viewKey, children }: { viewKey: string; children: ReactNode }) {
  return (
    <motion.div
      key={viewKey}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.2 }}
      className="absolute inset-0 p-[inherit]"
    >
      {children}
    </motion.div>
  )
}
