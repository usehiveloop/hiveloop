import { docs } from 'collections/server';
import { loader } from 'fumadocs-core/source';
import { HugeiconsIcon } from '@hugeicons/react';
import { BookOpen01Icon, CompassIcon, DashboardSquare01Icon, ApiIcon } from '@hugeicons/core-free-icons';

const icons: Record<string, typeof BookOpen01Icon> = {
  BookOpen01Icon,
  CompassIcon,
  DashboardSquare01Icon,
  ApiIcon,
};

export const source = loader({
  baseUrl: '/docs',
  source: docs.toFumadocsSource(),
  icon(name) {
    if (!name || !(name in icons)) return undefined;
    return <HugeiconsIcon icon={icons[name]} className="size-full" />;
  },
});
