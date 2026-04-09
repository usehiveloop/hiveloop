import { DocsLayout } from 'fumadocs-ui/layouts/docs';
import { baseOptions } from '@/lib/layout.shared';
import { source } from '@/lib/source';
import { DocsProvider } from './provider';
import { TreeContextProvider } from 'fumadocs-ui/contexts/tree';
import { NextProvider } from 'fumadocs-core/framework/next';
import type { ReactNode } from 'react';

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <NextProvider>
      <TreeContextProvider tree={source.getPageTree()}>
        <DocsProvider>
          <DocsLayout
            {...baseOptions()}
            tree={source.getPageTree()}
          >
            {children}
          </DocsLayout>
        </DocsProvider>
      </TreeContextProvider>
    </NextProvider>
  );
}
