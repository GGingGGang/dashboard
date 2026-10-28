// Package platform owns core registration and persistence contracts.
package platform

import sdk "github.com/GGingGGang/dashboard/sdk"

// Aliases preserve existing core callers and JSON contracts during SDK extraction.
type Connection = sdk.Connection
type Target = sdk.Target
type ProviderInfo = sdk.ProviderInfo
type QueryInfo = sdk.QueryInfo
type BuildSource = sdk.BuildSource
type QueueSource = sdk.QueueSource
type Provider = sdk.Provider
type TargetDiscoverer = sdk.TargetDiscoverer
type CI = sdk.CI
type DeploymentSource = sdk.DeploymentSource
type MetricSource = sdk.MetricSource
type CD = sdk.CD
type Monitoring = sdk.Monitoring
type Build = sdk.Build
type QueueItem = sdk.QueueItem
type Deployment = sdk.Deployment
type Query = sdk.Query
type Point = sdk.Point
type Series = sdk.Series
type QueryResult = sdk.QueryResult
type Rule = sdk.Rule
type RuleResult = sdk.RuleResult
type Snapshot = sdk.Snapshot
type CollectionStatus = sdk.CollectionStatus
type SearchRequest = sdk.SearchRequest
type Observation = sdk.Observation
type LogSource = sdk.LogSource
type TraceSource = sdk.TraceSource
type LogEntry = sdk.LogEntry
type LogResult = sdk.LogResult
type TraceSummary = sdk.TraceSummary
type TraceSearchResult = sdk.TraceSearchResult
type Attribute = sdk.Attribute
type Span = sdk.Span
type TraceResult = sdk.TraceResult
type JSONRequest = sdk.JSONRequest
type JSONResult = sdk.JSONResult
type APIError = sdk.APIError
type Definition = sdk.Definition

func CloseProvider(p Provider) error         { return sdk.CloseProvider(p) }
func CloneTargets(targets []Target) []Target { return sdk.CloneTargets(targets) }
func ValidateConnection(c Connection) error  { return sdk.ValidateConnection(c) }
