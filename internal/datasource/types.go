package datasourceresource

import "axiomnizam.bitbd.net/axiomnizam/internal/datasource/models"

// =====================================================
// Type aliases — re-export all domain types from models/
// so existing code that references datasourceresource.Type
// continues to compile without changes.
// =====================================================

// --- Resource types ---

type DataSourceV1Resource = models.DataSourceV1Resource
type DataSourceSpec = models.DataSourceSpec
type DataSourceResourceStatus = models.DataSourceResourceStatus

// --- Legacy handler types ---

type DataSourceResource = models.DataSourceResource
type DataSourceMetadata = models.DataSourceMetadata
type DataSourceStatus = models.DataSourceStatus

// --- Interfaces and runtime types ---

type Prober = models.Prober
type DataSourceReconciler = models.DataSourceReconciler

// --- Constants ---

const DataSourceKind = models.DataSourceKind
const DataSourceAPIVersion = models.DataSourceAPIVersion

// --- Constructor ---

var NewDataSourceReconciler = models.NewDataSourceReconciler
