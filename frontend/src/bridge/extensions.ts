// Declarative UI contributions are intentionally opaque until their renderers
// are implemented; these management APIs never execute frontend plugin code.
export type PluginState = {
  manifest: {
    id: string;
    version: string;
    contractVersion: number;
    providers: { id: string; name: string; executable: string; configSchema: unknown }[] | null;
    cards: unknown[] | null;
    dashboards: unknown[] | null;
    queries: unknown[] | null;
  };
  enabled: boolean;
};

export type SourceState = {
  id: string;
  name: string;
  targetId?: string;
  capabilities: string[];
  revision: number;
};
