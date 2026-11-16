import { HELP_GUIDES, type HelpGuide } from "../content.generated";

export type { HelpGuide };
export { HELP_GUIDES };

export function findGuide(slug: string): HelpGuide | undefined {
  return HELP_GUIDES.find((g) => g.slug === slug);
}
