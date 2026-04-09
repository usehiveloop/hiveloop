import { docs } from 'collections/server';
import { loader } from 'fumadocs-core/source';
import { HugeiconsIcon } from '@hugeicons/react';
import BookOpen01Icon from '@hugeicons/core-free-icons/BookOpen01Icon';
import CompassIcon from '@hugeicons/core-free-icons/CompassIcon';
import DashboardSquare01Icon from '@hugeicons/core-free-icons/DashboardSquare01Icon';
import ApiIcon from '@hugeicons/core-free-icons/ApiIcon';

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
