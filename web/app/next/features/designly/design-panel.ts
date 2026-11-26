// What the design's bar asks of the panels around it: the tools panel on the right («Инструменты» and
// «Макет») and the people's comments of the viewing mode. The bar only has their buttons; the host draws them.

/** A tab of the tools panel: the chat's tools, or a part of «Макет». */
export const DESIGN_TAB = {
  tools: 'tools',
  info: 'info',
  tweaks: 'tweaks',
  comments: 'comments',
  pages: 'pages',
  versions: 'versions',
} as const;
export type TDesignTab = (typeof DESIGN_TAB)[keyof typeof DESIGN_TAB];

export interface IDesignPanel {
  /** The tab on screen; null while the panel is shut. */
  tab: TDesignTab | null;
  /** Opens the panel on a tab; the tab already on screen shuts it. */
  open: (tab: TDesignTab) => void;
}

/** «Комментарий для команды» of the viewing mode. */
export interface IDesignComments {
  /** A place or a block is being picked for a comment. */
  on: boolean;
  toggle: () => void;
}
