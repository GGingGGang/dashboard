export type Target = {
  capabilities?: string[];
  id: string;
  name: string;
  service: string;
  environment: string;
};
export type Rule = {
  id: string;
  name: string;
  expression: string;
  threshold: number;
  unit: string;
  description: string;
  enabled: boolean;
};
export type Connection = {
  id: string;
  kind: string;
  name: string;
  url: string;
  browserUrl: string;
  auth: string;
  username: string;
  caFile: string;
  targets: Target[];
  rules: Rule[];
  hasSecret?: boolean;
};
export type Build = {
  connectionId: string;
  job: string;
  number: number;
  started: number;
  duration: number;
  status: string;
  rawStatus: string;
  commit: string;
  url: string;
  observed: number;
};
export type Deployment = {
  id: string;
  name: string;
  project: string;
  sync: string;
  health: string;
  revisions: string[];
  phase: string;
  message: string;
  finished: string;
  url: string;
};
export type Query = { expression: string; start: number; end: number };
export type QueryResult = {
  type: string;
  series: {
    labels: Record<string, string>;
    points: { time: number; value: number | null }[];
  }[];
  warnings: string[];
  truncated: boolean;
};
export type Snapshot = {
  modules?: Record<
    string,
    { attempted: string; lastSuccess: string; error: string }
  >;
  connectionId: string;
  attempted: string;
  lastSuccess: string;
  error: string;
  storageError: string;
  builds: Build[];
  queue: {
    id: number;
    job: string;
    since: number;
    reason: string;
    url: string;
  }[];
  deployments: Deployment[];
  rules: { rule: Rule; result: QueryResult; breaches: number; error: string }[];
  imported: number;
  backfillPending: boolean;
};
export type State = {
  connections: Connection[];
  snapshots: Snapshot[];
  providers: {
    kind: string;
    name: string;
    category: string;
    capabilities: string[];
    defaultAuth: string;
    authMethods: string[];
    pollSeconds?: number;
    query?: { language: string; defaultExpression: string; presets: Rule[] };
  }[];
  error: string;
  demo: boolean;
};
export type HistoryFilter = {
  connectionId: string;
  job: string;
  status: string;
  since: number;
  until: number;
  offset: number;
};
