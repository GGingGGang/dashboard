import type { State, Connection, ConnectionTest, Query, QueryResult, HistoryFilter, Rule, Build } from "../shared/types";
import type { SearchRequest, LogResult, TraceSearchResult, TraceResult } from "../shared/types";

export type Bridge = {
  GetState(): Promise<State>;
  SaveConnection(input: {
    connection: Connection;
    secret: string;
  }): Promise<string>;
  DeleteConnection(id: string): Promise<void>;
  TestConnection(input: {
    connection: Connection;
    secret: string;
  }): Promise<ConnectionTest>;
  Query(id: string, q: Query): Promise<QueryResult>;
  SearchLogs(id: string, q: SearchRequest): Promise<LogResult>;
  SearchTraces(id: string, q: SearchRequest): Promise<TraceSearchResult>;
  Trace(id: string, traceId: string): Promise<TraceResult>;
  Presets(kind: string): Promise<Rule[]>;
  History(
    f: HistoryFilter,
  ): Promise<{ builds: Build[]; total: number; size: number }>;
  GetPreference(key: string): Promise<string>;
  SavePreference(key: string, value: string): Promise<void>;
  Backup(): Promise<string>;
  OpenLink(url: string): Promise<void>;
  PickCA(): Promise<string>;
  Refresh(): Promise<void>;
};
declare global {
  interface Window {
    go?: { main: { App: Bridge } };
  }
}
