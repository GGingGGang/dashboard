import type { QueryResult } from "../shared/types";
export type FormField = { type: "string" | "number" | "integer" | "boolean"; title?: string; description?: string; default?: unknown; enum?: (string | number | boolean)[]; minimum?: number; maximum?: number; maxLength?: number };
export type FormSchema = { type: "object"; properties: Record<string, FormField>; required?: string[]; additionalProperties?: false };
export type CardContribution = { id: string; title: string; capability: string; renderer: "table" | "scalar" | "timeseries"; configSchema?: FormSchema | null };
export type QueryContribution = Omit<CardContribution, "configSchema"> & { inputSchema?: FormSchema | null };
export type PluginState = {
  manifest: {
    id: string; version: string; contractVersion: number;
    providers: { id: string; name: string; executable: string; configSchema: FormSchema | null }[] | null;
    cards: CardContribution[] | null;
    dashboards: { id: string; title: string; cards: string[] }[] | null;
    queries: QueryContribution[] | null;
  };
  enabled: boolean;
};
export type SourceState = { id: string; name: string; targetId?: string; capabilities: string[]; revision: number };
export type ContributionRequest = { pluginId: string; kind: "card" | "query"; id: string; input: Record<string, unknown> };
export type ExtensionBinding = ContributionRequest & { sourceId: string };
export type DataResult = { type: "table" | "scalar" | "timeseries"; columns?: {name: string; type: string}[]; rows?: (string | number | boolean | null)[][]; value?: string | number | boolean; series?: QueryResult["series"]; warnings?: string[] };
