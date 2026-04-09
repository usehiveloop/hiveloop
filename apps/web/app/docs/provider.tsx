'use client';

import { RootProvider } from 'fumadocs-ui/provider/base';
import type { ReactNode } from 'react';

export function DocsProvider({ children }: { children: ReactNode }) {
  return <RootProvider>{children}</RootProvider>;
}
