import { docs } from '@/.source';
import { loader } from 'fumadocs-core/source';
import type { InferPageType } from 'fumadocs-core/source';

export const source = loader({
  baseUrl: '/docs',
  source: docs,
});

export type Page = InferPageType<typeof source>;
