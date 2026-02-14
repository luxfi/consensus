// @ts-nocheck
import { browser } from '@hanzo/docs-mdx/runtime/browser';
import type * as Config from '../source.config';

const create = browser<typeof Config, import("@hanzo/docs-mdx/runtime/types").InternalTypeConfig & {
  DocData: {
  }
}>();
const browserCollections = {
  docs: create.doc("docs", {"api.mdx": () => import("../content/docs/api.mdx?collection=docs"), "configuration.mdx": () => import("../content/docs/configuration.mdx?collection=docs"), "index.mdx": () => import("../content/docs/index.mdx?collection=docs"), "photon.mdx": () => import("../content/docs/photon.mdx?collection=docs"), "quasar.mdx": () => import("../content/docs/quasar.mdx?collection=docs"), "wave.mdx": () => import("../content/docs/wave.mdx?collection=docs"), }),
};
export default browserCollections;