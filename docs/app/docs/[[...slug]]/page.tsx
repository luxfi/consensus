import { DocsPage, DocsBody } from 'fumadocs-ui/page';
import { notFound } from 'next/navigation';
import defaultMdxComponents from 'fumadocs-ui/mdx';

// Static list of all doc pages
const staticPages = [
  { slug: [], path: '@/content/docs/index.mdx' },
  { slug: ['benchmarks'], path: '@/content/docs/benchmarks.mdx' },
  { slug: ['sdk'], path: '@/content/docs/sdk/index.mdx' },
  { slug: ['sdk', 'go'], path: '@/content/docs/sdk/go.mdx' },
  { slug: ['sdk', 'c'], path: '@/content/docs/sdk/c.mdx' },
];

// Generate static params for all pages
export function generateStaticParams() {
  return staticPages.map(page => ({ slug: page.slug }));
}

// Map of slugs to dynamic imports
async function getContent(slug: string[]) {
  const slugKey = slug.join('/');

  switch (slugKey) {
    case '':
      return import('@/content/docs/index.mdx');
    case 'benchmarks':
      return import('@/content/docs/benchmarks.mdx');
    case 'sdk':
      return import('@/content/docs/sdk/index.mdx');
    case 'sdk/go':
      return import('@/content/docs/sdk/go.mdx');
    case 'sdk/c':
      return import('@/content/docs/sdk/c.mdx');
    default:
      return null;
  }
}

export default async function Page(props: {
  params: Promise<{ slug?: string[] }>;
}) {
  const params = await props.params;
  const slug = params.slug || [];

  const content = await getContent(slug);

  if (!content) {
    notFound();
  }

  const MDX = content.default;
  const { title, toc, full } = content as any;

  return (
    <DocsPage toc={toc} full={full}>
      <DocsBody>
        <h1>{title || 'Documentation'}</h1>
        <MDX components={{ ...defaultMdxComponents }} />
      </DocsBody>
    </DocsPage>
  );
}

export async function generateMetadata(props: {
  params: Promise<{ slug?: string[] }>;
}) {
  const params = await props.params;
  const slug = params.slug || [];

  const content = await getContent(slug);

  if (!content) {
    return {};
  }

  const { title, description } = content as any;

  return {
    title: title ? `${title} | Lux Consensus` : 'Lux Consensus',
    description,
  };
}
