/**
 * A string selector
 *
 * @alpha
 */

import { versionedAPIs } from '../selectors/apis';
import { versionedComponents } from '../selectors/components';
import { versionedPages } from '../selectors/pages';

export type StringSelector = string;

/**
 * A function selector with an argument
 *
 * @alpha
 */
export type FunctionSelector = (id: string, addiotionalArgs: string) => string;

/**
 * A function selector without argument
 *
 * @alpha
 */
export type CssSelector = () => string;

/**
 * @alpha
 */
export interface Selectors {
  [key: string]: StringSelector | FunctionSelector | CssSelector | UrlSelector | Selectors;
}

/**
 * @alpha
 */
export type E2ESelectorVariant<S extends Selectors> = {
  [P in keyof S]: S[P];
};

/**
 * @alpha
 */
export interface UrlSelector extends Selectors {
  url: string | FunctionSelector;
}

export type VersionedSelectors = {
  pages: typeof versionedPages;
  components: typeof versionedComponents;
  apis: typeof versionedAPIs;
};
