// Server-only collections loader
// IMPORTANT: this file must NOT be imported by any app/* files
import 'server-only';
import { docs } from '@/.source';
import { loader as createLoader } from 'fumadocs-core/source';

// Create loader with proper configuration
export const loader = createLoader({
  baseUrl: '/docs',
  source: docs,
});

// Precompute all slugs for SSG
export const getAllDocParams = () => {
  return loader.getPages().map((page) => ({
    slug: page.slugs,
  }));
};
