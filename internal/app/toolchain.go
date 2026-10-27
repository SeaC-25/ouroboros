package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/contracts"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/toolchain"
	"gopkg.in/yaml.v3"
)

type ToolchainStatus struct {
	Name            string
	Version         string
	Runner          string
	Parser          string
	CapabilityCount int
	Health          toolchain.HealthReport
	Dependencies    []toolchain.DependencyReport
	InstallPlan     []toolchain.DependencyInstallStep
}

type ToolchainContractCheck struct {
	Status              string
	Mode                string
	ManifestCount       int
	ToolCount           int
	DomainPackCount     int
	PatternCount        int
	SemanticActionCount int
	Errors              []string
}

type AuthoredToolArtifact struct {
	Status       string   `json:"status"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Parser       string   `json:"parser"`
	AdapterID    string   `json:"adapter_id"`
	Operation    string   `json:"operation"`
	Capabilities []string `json:"capabilities"`
	Dir          string   `json:"dir"`
	SpecPath     string   `json:"spec_path"`
	ManifestPath string   `json:"manifest_path"`
}

type ManagedToolBundle struct {
	Status                    string `json:"status"`
	Name                      string `json:"name"`
	Kind                      string `json:"kind"`
	Version                   string `json:"version"`
	Source                    string `json:"source"`
	SHA256                    string `json:"sha256"`
	ProvenanceStatus          string `json:"provenance_status"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	BundleDir                 string `json:"bundle_dir"`
	ManifestPath              string `json:"manifest_path"`
	PayloadPath               string `json:"payload_path"`
	InstallPath               string `json:"install_path"`
	Error                     string `json:"error,omitempty"`
}

type ManagedToolBundleImportOptions struct {
	Import                 bool
	Name                   string
	Kind                   string
	Version                string
	Source                 string
	PayloadPath            string
	ExpectedSHA256         string
	ExpectedSHA256Provided bool
}

type ManagedToolBundleImportResult struct {
	Status            string `json:"status"`
	Mode              string `json:"mode"`
	Action            string `json:"action"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Version           string `json:"version"`
	Source            string `json:"source"`
	SHA256            string `json:"sha256"`
	ExpectedSHA256    string `json:"expected_sha256,omitempty"`
	SourcePayloadPath string `json:"source_payload_path"`
	BundleDir         string `json:"bundle_dir"`
	ManifestPath      string `json:"manifest_path"`
	PayloadPath       string `json:"payload_path"`
	InstallPath       string `json:"install_path"`
}

type ReleaseManagedToolBundle struct {
	Status                    string `json:"status"`
	Mode                      string `json:"mode"`
	Name                      string `json:"name"`
	Kind                      string `json:"kind,omitempty"`
	Version                   string `json:"version,omitempty"`
	Source                    string `json:"source,omitempty"`
	SHA256                    string `json:"sha256,omitempty"`
	ProvenanceStatus          string `json:"provenance_status,omitempty"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status,omitempty"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status,omitempty"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	ReleaseBundleDir          string `json:"release_bundle_dir"`
	ReleaseManifestPath       string `json:"release_manifest_path"`
	ReleasePayloadPath        string `json:"release_payload_path,omitempty"`
	Error                     string `json:"error,omitempty"`
}

type ReleaseManagedToolBundleStatus struct {
	Status                    string `json:"status"`
	Mode                      string `json:"mode"`
	Name                      string `json:"name"`
	Kind                      string `json:"kind,omitempty"`
	Version                   string `json:"version,omitempty"`
	Source                    string `json:"source,omitempty"`
	SHA256                    string `json:"sha256,omitempty"`
	ExpectedSHA256            string `json:"expected_sha256,omitempty"`
	ExpectedSHA256Status      string `json:"expected_sha256_status,omitempty"`
	ProvenanceStatus          string `json:"provenance_status,omitempty"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status,omitempty"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status,omitempty"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	DigestStatus              string `json:"digest_status"`
	DigestAlgorithm           string `json:"digest_algorithm,omitempty"`
	DigestScope               string `json:"digest_scope,omitempty"`
	ImportStatus              string `json:"import_status"`
	LinkStatus                string `json:"link_status"`
	ReleaseBundleDir          string `json:"release_bundle_dir"`
	ReleaseManifestPath       string `json:"release_manifest_path"`
	ReleasePayloadPath        string `json:"release_payload_path,omitempty"`
	WorkspaceBundleDir        string `json:"workspace_bundle_dir,omitempty"`
	WorkspaceManifestPath     string `json:"workspace_manifest_path,omitempty"`
	WorkspacePayloadPath      string `json:"workspace_payload_path,omitempty"`
	InstallPath               string `json:"install_path,omitempty"`
	Error                     string `json:"error,omitempty"`
}

type ReleaseManagedToolBundleStatusSource struct {
	Status               string `json:"status"`
	Mode                 string `json:"mode"`
	Path                 string `json:"path"`
	ReleaseRoot          string `json:"release_root"`
	Total                int    `json:"total"`
	Available            int    `json:"available"`
	Blocked              int    `json:"blocked"`
	NotImported          int    `json:"not_imported"`
	Imported             int    `json:"imported"`
	Linked               int    `json:"linked"`
	Conflict             int    `json:"conflict"`
	ExpectedSHA256Bundle string `json:"expected_sha256_bundle,omitempty"`
	ExpectedSHA256       string `json:"expected_sha256,omitempty"`
	ExpectedSHA256Status string `json:"expected_sha256_status,omitempty"`
	SetBy                string `json:"set_by,omitempty"`
	SetAt                string `json:"set_at,omitempty"`
	ClearedBy            string `json:"cleared_by,omitempty"`
	ClearedAt            string `json:"cleared_at,omitempty"`
	Reason               string `json:"reason,omitempty"`
}

type ReleaseManagedToolBundleStatusSourceOptions struct {
	Set                    bool
	SetBy                  string
	Reason                 string
	ExpectedSHA256Bundle   string
	ExpectedSHA256         string
	ExpectedSHA256Provided bool
}

type ReleaseManagedToolBundleStatusSourceClearOptions struct {
	Clear     bool
	ClearedBy string
	Reason    string
}

type ReleaseManagedToolBundleStatusOptions struct {
	ExpectedSHA256         string
	ExpectedSHA256Provided bool
}

type ManagedToolBundleUpdateRequestOptions struct {
	Request     bool
	RequestedBy string
	Reason      string
}

type ManagedToolBundleUpdateRequest struct {
	Status              string `json:"status"`
	Mode                string `json:"mode"`
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	Source              string `json:"source,omitempty"`
	CurrentVersion      string `json:"current_version"`
	CurrentSHA256       string `json:"current_sha256"`
	LatestVersion       string `json:"latest_version"`
	LatestSHA256        string `json:"latest_sha256"`
	UpdateStatus        string `json:"update_status"`
	UpdateCheckedAt     string `json:"update_checked_at,omitempty"`
	UpdateMessage       string `json:"update_message,omitempty"`
	ReleaseRoot         string `json:"release_root"`
	ReleaseBundleDir    string `json:"release_bundle_dir"`
	ReleaseManifestPath string `json:"release_manifest_path"`
	ReleasePayloadPath  string `json:"release_payload_path,omitempty"`
	Path                string `json:"path,omitempty"`
	RequestedBy         string `json:"requested_by,omitempty"`
	RequestedAt         string `json:"requested_at,omitempty"`
	Reason              string `json:"reason,omitempty"`
}

type ManagedToolBundleUpdateRequestSummary struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
}

type ManagedToolBundleUpdateDownloadPlanOptions struct {
	Plan                  bool
	SourceURL             string
	ExpectedSHA256        string
	ArchiveFormat         string
	ArchiveExpectedSHA256 string
	PlannedBy             string
	Reason                string
}

type ManagedToolBundleUpdateDownloadPlan struct {
	Status                      string `json:"status"`
	Mode                        string `json:"mode"`
	Name                        string `json:"name"`
	Kind                        string `json:"kind"`
	Source                      string `json:"source,omitempty"`
	CurrentVersion              string `json:"current_version"`
	CurrentSHA256               string `json:"current_sha256"`
	LatestVersion               string `json:"latest_version"`
	LatestSHA256                string `json:"latest_sha256"`
	UpdateStatus                string `json:"update_status"`
	UpdateCheckedAt             string `json:"update_checked_at,omitempty"`
	UpdateMessage               string `json:"update_message,omitempty"`
	ExpectedSHA256              string `json:"expected_sha256"`
	ExpectedSHA256Status        string `json:"expected_sha256_status"`
	ArchiveFormat               string `json:"archive_format,omitempty"`
	ArchiveExpectedSHA256       string `json:"archive_expected_sha256,omitempty"`
	ArchiveExpectedSHA256Status string `json:"archive_expected_sha256_status,omitempty"`
	SourceURL                   string `json:"source_url"`
	RequestPath                 string `json:"request_path"`
	ReleaseRoot                 string `json:"release_root"`
	ReleaseBundleDir            string `json:"release_bundle_dir"`
	ReleaseManifestPath         string `json:"release_manifest_path"`
	ReleasePayloadPath          string `json:"release_payload_path,omitempty"`
	Path                        string `json:"path,omitempty"`
	PlannedBy                   string `json:"planned_by,omitempty"`
	PlannedAt                   string `json:"planned_at,omitempty"`
	Reason                      string `json:"reason,omitempty"`
}

type ManagedToolBundleUpdateDownloadPlanSummary struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
}

type ManagedToolBundleUpdateDownloadExecutionOptions struct {
	Execute    bool
	HTTPClient *http.Client
	Actor      string
	Reason     string
}

type ManagedToolBundleUpdateDownloadExecution struct {
	Status                 string `json:"status"`
	Mode                   string `json:"mode"`
	Action                 string `json:"action"`
	Name                   string `json:"name"`
	Kind                   string `json:"kind"`
	Source                 string `json:"source,omitempty"`
	CurrentVersion         string `json:"current_version"`
	CurrentSHA256          string `json:"current_sha256"`
	LatestVersion          string `json:"latest_version"`
	LatestSHA256           string `json:"latest_sha256"`
	UpdateStatus           string `json:"update_status"`
	UpdateCheckedAt        string `json:"update_checked_at,omitempty"`
	UpdateMessage          string `json:"update_message,omitempty"`
	SourceURL              string `json:"source_url"`
	ExpectedSHA256         string `json:"expected_sha256"`
	ActualSHA256           string `json:"actual_sha256,omitempty"`
	DigestStatus           string `json:"digest_status"`
	DigestAlgorithm        string `json:"digest_algorithm,omitempty"`
	DigestScope            string `json:"digest_scope,omitempty"`
	ArchiveFormat          string `json:"archive_format,omitempty"`
	ArchivePath            string `json:"archive_path,omitempty"`
	ArchiveExpectedSHA256  string `json:"archive_expected_sha256,omitempty"`
	ArchiveActualSHA256    string `json:"archive_actual_sha256,omitempty"`
	ArchiveDigestStatus    string `json:"archive_digest_status,omitempty"`
	ArchiveDigestAlgorithm string `json:"archive_digest_algorithm,omitempty"`
	ArchiveDigestScope     string `json:"archive_digest_scope,omitempty"`
	HTTPStatus             int    `json:"http_status,omitempty"`
	ContentLength          int64  `json:"content_length,omitempty"`
	BytesWritten           int64  `json:"bytes_written,omitempty"`
	RedirectPolicy         string `json:"redirect_policy,omitempty"`
	RedirectLocation       string `json:"redirect_location,omitempty"`
	RedirectLocationStatus string `json:"redirect_location_status,omitempty"`
	PlanPath               string `json:"plan_path"`
	RequestPath            string `json:"request_path"`
	ReleaseRoot            string `json:"release_root"`
	ReleaseBundleDir       string `json:"release_bundle_dir"`
	ReleaseManifestPath    string `json:"release_manifest_path"`
	ReleasePayloadPath     string `json:"release_payload_path,omitempty"`
	PayloadPath            string `json:"payload_path,omitempty"`
	Path                   string `json:"path,omitempty"`
	Actor                  string `json:"actor,omitempty"`
	ExecutedAt             string `json:"executed_at,omitempty"`
	Reason                 string `json:"reason,omitempty"`
	Error                  string `json:"error,omitempty"`
}

type ManagedToolBundleUpdateDownloadExecutionSummary struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type ManagedToolBundleUpdateApplyOptions struct {
	Apply  bool
	Actor  string
	Reason string
}

type ManagedToolBundleUpdateApplyExecution struct {
	Status                string `json:"status"`
	Mode                  string `json:"mode"`
	Action                string `json:"action"`
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	Source                string `json:"source,omitempty"`
	CurrentVersion        string `json:"current_version"`
	CurrentSHA256         string `json:"current_sha256"`
	LatestVersion         string `json:"latest_version"`
	LatestSHA256          string `json:"latest_sha256"`
	UpdateStatus          string `json:"update_status"`
	UpdateCheckedAt       string `json:"update_checked_at,omitempty"`
	UpdateMessage         string `json:"update_message,omitempty"`
	SourceURL             string `json:"source_url"`
	ExpectedSHA256        string `json:"expected_sha256"`
	ActualSHA256          string `json:"actual_sha256"`
	DigestStatus          string `json:"digest_status"`
	DigestAlgorithm       string `json:"digest_algorithm"`
	DigestScope           string `json:"digest_scope"`
	DownloadExecutionPath string `json:"download_execution_path"`
	PlanPath              string `json:"plan_path"`
	RequestPath           string `json:"request_path"`
	ReleaseRoot           string `json:"release_root"`
	ReleaseBundleDir      string `json:"release_bundle_dir"`
	ReleaseManifestPath   string `json:"release_manifest_path"`
	ReleasePayloadPath    string `json:"release_payload_path,omitempty"`
	StagedPayloadPath     string `json:"staged_payload_path"`
	WorkspaceBundleDir    string `json:"workspace_bundle_dir"`
	WorkspaceManifestPath string `json:"workspace_manifest_path"`
	WorkspacePayloadPath  string `json:"workspace_payload_path"`
	InstallPath           string `json:"install_path"`
	Path                  string `json:"path,omitempty"`
	Actor                 string `json:"actor,omitempty"`
	AppliedAt             string `json:"applied_at,omitempty"`
	Reason                string `json:"reason,omitempty"`
	Error                 string `json:"error,omitempty"`
}

type ManagedToolBundleUpdateApplyExecutionSummary struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type ReleaseManagedToolBundleInstallPlanOptions struct {
	ExpectedSHA256         string
	ExpectedSHA256Provided bool
}

type ReleaseManagedToolBundleInstallOptions struct {
	Execute                bool
	ExpectedSHA256         string
	ExpectedSHA256Provided bool
}

type ManagedToolBundleInstallPlan struct {
	Status                    string `json:"status"`
	Mode                      string `json:"mode"`
	Action                    string `json:"action"`
	Pack                      string `json:"pack"`
	Dependency                string `json:"dependency"`
	Kind                      string `json:"kind"`
	Version                   string `json:"version"`
	Source                    string `json:"source"`
	SHA256                    string `json:"sha256"`
	ProvenanceStatus          string `json:"provenance_status"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	PayloadPath               string `json:"payload_path"`
	InstallPath               string `json:"install_path"`
}

type ManagedToolBundleInstallOptions struct {
	Execute bool
}

type ManagedToolBundleInstallResult struct {
	Status                    string `json:"status"`
	Mode                      string `json:"mode"`
	Action                    string `json:"action"`
	Pack                      string `json:"pack"`
	Dependency                string `json:"dependency"`
	Kind                      string `json:"kind"`
	Version                   string `json:"version"`
	Source                    string `json:"source"`
	SHA256                    string `json:"sha256"`
	ProvenanceStatus          string `json:"provenance_status"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	PayloadPath               string `json:"payload_path"`
	InstallPath               string `json:"install_path"`
}

type ReleaseManagedToolBundleInstallPlan struct {
	Status                    string `json:"status"`
	Mode                      string `json:"mode"`
	Action                    string `json:"action"`
	Pack                      string `json:"pack"`
	Dependency                string `json:"dependency"`
	Kind                      string `json:"kind"`
	Version                   string `json:"version"`
	Source                    string `json:"source"`
	SHA256                    string `json:"sha256"`
	ExpectedSHA256            string `json:"expected_sha256,omitempty"`
	ExpectedSHA256Status      string `json:"expected_sha256_status,omitempty"`
	ProvenanceStatus          string `json:"provenance_status"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	ReleaseBundleDir          string `json:"release_bundle_dir"`
	ReleasePayloadPath        string `json:"release_payload_path"`
	WorkspaceBundleDir        string `json:"workspace_bundle_dir"`
	WorkspacePayloadPath      string `json:"workspace_payload_path"`
	InstallPath               string `json:"install_path"`
}

type ReleaseManagedToolBundleInstallResult struct {
	Status                    string `json:"status"`
	Mode                      string `json:"mode"`
	Action                    string `json:"action"`
	Pack                      string `json:"pack"`
	Dependency                string `json:"dependency"`
	Kind                      string `json:"kind"`
	Version                   string `json:"version"`
	Source                    string `json:"source"`
	SHA256                    string `json:"sha256"`
	ExpectedSHA256            string `json:"expected_sha256,omitempty"`
	ExpectedSHA256Status      string `json:"expected_sha256_status,omitempty"`
	ProvenanceStatus          string `json:"provenance_status"`
	ProvenanceSourceURL       string `json:"provenance_source_url,omitempty"`
	ProvenanceUpstreamVersion string `json:"provenance_upstream_version,omitempty"`
	ProvenanceRetrievedAt     string `json:"provenance_retrieved_at,omitempty"`
	ProvenanceVerifiedBy      string `json:"provenance_verified_by,omitempty"`
	CorpusStatus              string `json:"corpus_status,omitempty"`
	CorpusName                string `json:"corpus_name,omitempty"`
	CorpusSource              string `json:"corpus_source,omitempty"`
	CorpusRetrievedAt         string `json:"corpus_retrieved_at,omitempty"`
	CorpusFileCount           int    `json:"corpus_file_count,omitempty"`
	CorpusSHA256              string `json:"corpus_sha256,omitempty"`
	CorpusDigestStatus        string `json:"corpus_digest_status,omitempty"`
	CorpusDigestAlgorithm     string `json:"corpus_digest_algorithm,omitempty"`
	CorpusDigestScope         string `json:"corpus_digest_scope,omitempty"`
	SignatureStatus           string `json:"signature_status"`
	SignatureAlgorithm        string `json:"signature_algorithm,omitempty"`
	SignatureKeyID            string `json:"signature_key_id,omitempty"`
	SignatureSignedAt         string `json:"signature_signed_at,omitempty"`
	SignatureSignedBy         string `json:"signature_signed_by,omitempty"`
	UpdateStatus              string `json:"update_status"`
	UpdateLatestVersion       string `json:"update_latest_version,omitempty"`
	UpdateLatestSHA256        string `json:"update_latest_sha256,omitempty"`
	UpdateCheckedAt           string `json:"update_checked_at,omitempty"`
	UpdateMessage             string `json:"update_message,omitempty"`
	ReleaseBundleDir          string `json:"release_bundle_dir"`
	ReleasePayloadPath        string `json:"release_payload_path"`
	WorkspaceBundleDir        string `json:"workspace_bundle_dir"`
	WorkspacePayloadPath      string `json:"workspace_payload_path"`
	InstallPath               string `json:"install_path"`
}

type ToolCompilePreviewOptions struct {
	Action           compiler.SemanticAction
	Budget           compiler.CompileBudget
	DomainPriorities []compiler.DomainPriority
	FrontierScores   []compiler.CandidateFrontierScore
	HistoricalStats  []compiler.CandidateHistory
}

type ToolCompilePreview struct {
	Workspace string                  `json:"workspace"`
	Action    compiler.SemanticAction `json:"action"`
	Mode      string                  `json:"mode"`
	Nodes     []compiler.PlanNode     `json:"nodes"`
	Warnings  []string                `json:"warnings,omitempty"`
}

type ToolCompilePreviewReview struct {
	Status     string                              `json:"status"`
	Mode       string                              `json:"mode"`
	Path       string                              `json:"path,omitempty"`
	Preview    ToolCompilePreview                  `json:"preview"`
	Annotation *ToolCompilePreviewReviewAnnotation `json:"annotation,omitempty"`
}

type ToolCompilePreviewReviewAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type ToolCompilePreviewReviewAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type ToolCompilePreviewReviewComparison struct {
	Status       string                                  `json:"status"`
	Mode         string                                  `json:"mode"`
	Base         ToolCompilePreviewReviewComparisonSide  `json:"base"`
	Candidate    ToolCompilePreviewReviewComparisonSide  `json:"candidate"`
	Summary      ToolCompilePreviewReviewComparisonStats `json:"summary"`
	NodeDiffs    []ToolCompilePreviewReviewNodeDiff      `json:"node_diffs,omitempty"`
	WarningDiffs []ToolCompilePreviewReviewWarningDiff   `json:"warning_diffs,omitempty"`
}

type ToolCompilePreviewReviewComparisonSide struct {
	Path     string                  `json:"path"`
	Action   compiler.SemanticAction `json:"action"`
	Nodes    int                     `json:"nodes"`
	Warnings int                     `json:"warnings"`
	Decision string                  `json:"decision,omitempty"`
}

type ToolCompilePreviewReviewComparisonStats struct {
	NodeDelta          int `json:"node_delta"`
	WarningDelta       int `json:"warning_delta"`
	AddedNodes         int `json:"added_nodes"`
	RemovedNodes       int `json:"removed_nodes"`
	ChangedNodes       int `json:"changed_nodes"`
	ReorderedNodes     int `json:"reordered_nodes"`
	ScoreChangedNodes  int `json:"score_changed_nodes"`
	SignalChangedNodes int `json:"signal_changed_nodes"`
}

type ToolCompilePreviewReviewNodeDiff struct {
	Change           string   `json:"change"`
	ManifestName     string   `json:"manifest_name"`
	ToolID           string   `json:"tool_id,omitempty"`
	AdapterID        string   `json:"adapter_id,omitempty"`
	Operation        string   `json:"operation,omitempty"`
	ParserID         string   `json:"parser_id,omitempty"`
	TargetKind       string   `json:"target_kind,omitempty"`
	DependencyName   string   `json:"dependency_name,omitempty"`
	DependencyKind   string   `json:"dependency_kind,omitempty"`
	Capability       string   `json:"capability"`
	BaseID           string   `json:"base_id,omitempty"`
	CandidateID      string   `json:"candidate_id,omitempty"`
	BaseIndex        int      `json:"base_index"`
	CandidateIndex   int      `json:"candidate_index"`
	BaseScore        float64  `json:"base_score,omitempty"`
	CandidateScore   float64  `json:"candidate_score,omitempty"`
	BaseSignals      []string `json:"base_signals,omitempty"`
	CandidateSignals []string `json:"candidate_signals,omitempty"`
}

type ToolCompilePreviewReviewWarningDiff struct {
	Change  string `json:"change"`
	Warning string `json:"warning"`
}

type ToolCompilePreviewReviewSelection struct {
	Path            string                  `json:"path,omitempty"`
	Status          string                  `json:"status"`
	Mode            string                  `json:"mode"`
	ReviewPath      string                  `json:"review_path"`
	Action          compiler.SemanticAction `json:"action"`
	ReviewNodes     int                     `json:"review_nodes"`
	ReviewWarnings  int                     `json:"review_warnings"`
	SupersededPaths []string                `json:"superseded_paths,omitempty"`
	SelectedBy      string                  `json:"selected_by,omitempty"`
	SelectedAt      string                  `json:"selected_at,omitempty"`
	ClearedBy       string                  `json:"cleared_by,omitempty"`
	ClearedAt       string                  `json:"cleared_at,omitempty"`
	Reason          string                  `json:"reason,omitempty"`
}

type ToolCompilePreviewReviewSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type ToolCompilePreviewReviewSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

type ToolCompilePreviewReviewExecutionDispatch struct {
	Path           string                  `json:"path,omitempty"`
	Status         string                  `json:"status"`
	Mode           string                  `json:"mode"`
	ReviewPath     string                  `json:"review_path"`
	SelectionPath  string                  `json:"selection_path"`
	Action         compiler.SemanticAction `json:"action"`
	ReviewNodes    int                     `json:"review_nodes"`
	ReviewWarnings int                     `json:"review_warnings"`
	CreatedAt      string                  `json:"created_at,omitempty"`
	Actor          string                  `json:"actor,omitempty"`
	Reason         string                  `json:"reason,omitempty"`
}

type ToolCompilePreviewReviewExecutionDispatchOptions struct {
	Actor     string
	Reason    string
	CreatedAt time.Time
}

type ToolCompilePreviewReviewExecution struct {
	Path           string                  `json:"path,omitempty"`
	Status         string                  `json:"status"`
	Mode           string                  `json:"mode"`
	RunID          string                  `json:"run_id"`
	DispatchPath   string                  `json:"dispatch_path,omitempty"`
	ReviewPath     string                  `json:"review_path"`
	SelectionPath  string                  `json:"selection_path,omitempty"`
	Action         compiler.SemanticAction `json:"action"`
	ReviewNodes    int                     `json:"review_nodes"`
	ReviewWarnings int                     `json:"review_warnings"`
	EventCount     int                     `json:"event_count"`
	ExecutedAt     string                  `json:"executed_at,omitempty"`
	Actor          string                  `json:"actor,omitempty"`
	Reason         string                  `json:"reason,omitempty"`
	Error          string                  `json:"error,omitempty"`
	Warnings       []string                `json:"warnings,omitempty"`
}

type ToolCompilePreviewReviewExecutionOptions struct {
	Execute         bool
	Actor           string
	Reason          string
	ExecutedAt      time.Time
	ToolRunner      ToolExecutionRunner
	ToolConcurrency int
	DispatchPath    string
}

type ToolCompilePreviewSignalArtifact struct {
	Path    string                    `json:"path"`
	Status  string                    `json:"status"`
	Mode    string                    `json:"mode"`
	Signals ToolCompilePreviewSignals `json:"signals"`
}

type ToolCompilePreviewSignalDeriveOptions struct {
	Write        bool
	MaxPlanNodes int
}

type ToolCompilePreviewSignalDerivation struct {
	Path    string                                    `json:"path"`
	Status  string                                    `json:"status"`
	Mode    string                                    `json:"mode"`
	RunID   string                                    `json:"run_id"`
	Signals ToolCompilePreviewSignals                 `json:"signals"`
	Summary ToolCompilePreviewSignalDerivationSummary `json:"summary"`
}

type ToolCompilePreviewSignalDerivationSummary struct {
	ToolExecutions int `json:"tool_executions"`
	Successes      int `json:"successes"`
	Failures       int `json:"failures"`
	Ignored        int `json:"ignored"`
	FrontierScores int `json:"frontier_scores"`
}

type ToolCompilePreviewSignals struct {
	SchemaVersion    int                                      `json:"schema_version,omitempty"`
	Budget           ToolCompilePreviewSignalBudget           `json:"budget,omitempty"`
	DomainPriorities []ToolCompilePreviewSignalDomainPriority `json:"domain_priorities,omitempty"`
	FrontierScores   []ToolCompilePreviewSignalFrontierScore  `json:"frontier_scores,omitempty"`
	HistoricalStats  []ToolCompilePreviewSignalHistory        `json:"historical_stats,omitempty"`
}

type ToolCompilePreviewSignalBudget struct {
	MaxPlanNodes int `json:"max_plan_nodes,omitempty"`
}

type ToolCompilePreviewSignalDomainPriority struct {
	Domain string  `json:"domain"`
	Weight float64 `json:"weight"`
}

type ToolCompilePreviewSignalFrontierScore struct {
	ManifestName string                       `json:"manifest_name"`
	Capability   string                       `json:"capability"`
	Path         ToolCompilePreviewSignalPath `json:"path"`
}

type ToolCompilePreviewSignalPath struct {
	ID                 string  `json:"id,omitempty"`
	Target             string  `json:"target,omitempty"`
	Hypothesis         string  `json:"hypothesis,omitempty"`
	ExpectedUnlock     string  `json:"expected_unlock,omitempty"`
	MissionValue       float64 `json:"mission_value,omitempty"`
	UnlockPotential    float64 `json:"unlock_potential,omitempty"`
	EvidenceConfidence float64 `json:"evidence_confidence,omitempty"`
	Novelty            float64 `json:"novelty,omitempty"`
	Cost               float64 `json:"cost,omitempty"`
	DuplicateRisk      float64 `json:"duplicate_risk,omitempty"`
	Noise              float64 `json:"noise,omitempty"`
	ExhaustedPenalty   float64 `json:"exhausted_penalty,omitempty"`
}

type ToolCompilePreviewSignalHistory struct {
	ManifestName string `json:"manifest_name"`
	Capability   string `json:"capability"`
	Successes    int    `json:"successes,omitempty"`
	Failures     int    `json:"failures,omitempty"`
}

type ToolchainInstallRequest struct {
	Status      string   `json:"status"`
	Pack        string   `json:"pack"`
	Dependency  string   `json:"dependency"`
	Kind        string   `json:"kind"`
	State       string   `json:"state"`
	Required    bool     `json:"required"`
	Command     []string `json:"command"`
	Hint        string   `json:"hint,omitempty"`
	Source      string   `json:"source,omitempty"`
	ImageDigest string   `json:"image_digest,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Path        string   `json:"path,omitempty"`
}

type ToolchainInstallRequestSummary struct {
	Total    int `json:"total"`
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Denied   int `json:"denied"`
}

type ToolchainInstallDispatchPlan struct {
	Summary ToolchainInstallRequestSummary `json:"summary"`
	Ready   []ToolchainInstallRequest      `json:"ready"`
	Skipped []ToolchainInstallDispatchSkip `json:"skipped"`
}

type ToolchainInstallDispatchSkip struct {
	Request ToolchainInstallRequest `json:"request"`
	Reason  string                  `json:"reason"`
}

type ToolchainInstallDispatch struct {
	Status  string                         `json:"status"`
	Mode    string                         `json:"mode"`
	Path    string                         `json:"path,omitempty"`
	Summary ToolchainInstallRequestSummary `json:"summary"`
	Ready   []ToolchainInstallRequest      `json:"ready"`
	Skipped []ToolchainInstallDispatchSkip `json:"skipped"`
}

type ToolchainInstallDispatchExecutionOptions struct {
	Execute bool
}

type ToolchainInstallDispatchExecution struct {
	Status       string                                    `json:"status"`
	Mode         string                                    `json:"mode"`
	Path         string                                    `json:"path,omitempty"`
	DispatchPath string                                    `json:"dispatch_path"`
	Warnings     []string                                  `json:"warnings,omitempty"`
	Results      []ToolchainInstallDispatchExecutionResult `json:"results"`
	Skipped      []ToolchainInstallDispatchSkip            `json:"skipped"`
}

type ToolchainInstallDispatchExecutionResult struct {
	RequestPath string   `json:"request_path,omitempty"`
	Pack        string   `json:"pack"`
	Dependency  string   `json:"dependency"`
	Command     []string `json:"command"`
	Status      string   `json:"status"`
	Output      string   `json:"output,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type ToolchainInstallRetryPlan struct {
	Summary ToolchainInstallRetrySummary     `json:"summary"`
	Ready   []ToolchainInstallRetryCandidate `json:"ready"`
	Skipped []ToolchainInstallRetrySkip      `json:"skipped"`
}

type ToolchainInstallRetrySummary struct {
	Total   int `json:"total"`
	Ready   int `json:"ready"`
	Skipped int `json:"skipped"`
}

type ToolchainInstallRetryCandidate struct {
	ExecutionPath string   `json:"execution_path"`
	DispatchPath  string   `json:"dispatch_path"`
	RequestPath   string   `json:"request_path,omitempty"`
	Pack          string   `json:"pack"`
	Dependency    string   `json:"dependency"`
	Command       []string `json:"command"`
	LastStatus    string   `json:"last_status"`
	LastOutput    string   `json:"last_output,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
}

type ToolchainInstallRetrySkip struct {
	ExecutionPath string `json:"execution_path"`
	DispatchPath  string `json:"dispatch_path"`
	RequestPath   string `json:"request_path,omitempty"`
	Pack          string `json:"pack"`
	Dependency    string `json:"dependency"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
}

type ToolchainInstallRetryDispatch struct {
	Status  string                           `json:"status"`
	Mode    string                           `json:"mode"`
	Path    string                           `json:"path,omitempty"`
	Summary ToolchainInstallRetrySummary     `json:"summary"`
	Ready   []ToolchainInstallRetryCandidate `json:"ready"`
	Skipped []ToolchainInstallRetrySkip      `json:"skipped"`
}

type ToolchainInstallRetryDispatchExecutionOptions struct {
	Execute bool
}

type ToolchainInstallRetryDispatchExecution struct {
	Status            string                                `json:"status"`
	Mode              string                                `json:"mode"`
	Path              string                                `json:"path,omitempty"`
	RetryDispatchPath string                                `json:"retry_dispatch_path"`
	Warnings          []string                              `json:"warnings,omitempty"`
	Results           []ToolchainInstallRetryDispatchResult `json:"results"`
	Skipped           []ToolchainInstallRetrySkip           `json:"skipped"`
}

type ToolchainInstallRetryDispatchResult struct {
	ExecutionPath string   `json:"execution_path"`
	DispatchPath  string   `json:"dispatch_path"`
	RequestPath   string   `json:"request_path,omitempty"`
	Pack          string   `json:"pack"`
	Dependency    string   `json:"dependency"`
	Command       []string `json:"command"`
	LastStatus    string   `json:"last_status"`
	LastOutput    string   `json:"last_output,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
	Status        string   `json:"status"`
	Output        string   `json:"output,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func SummarizeToolchainInstallRequests(requests []ToolchainInstallRequest) ToolchainInstallRequestSummary {
	summary := ToolchainInstallRequestSummary{Total: len(requests)}
	for _, request := range requests {
		switch strings.ToLower(strings.TrimSpace(request.Status)) {
		case "pending":
			summary.Pending++
		case "approved":
			summary.Approved++
		case "denied":
			summary.Denied++
		}
	}
	return summary
}

func (s ToolchainInstallRequestSummary) Line() string {
	return fmt.Sprintf("total=%d pending=%d approved=%d denied=%d", s.Total, s.Pending, s.Approved, s.Denied)
}

func (s ToolchainInstallRequestSummary) Compact() string {
	return fmt.Sprintf("p%d/a%d/d%d", s.Pending, s.Approved, s.Denied)
}

func LoadToolCompilePreviewSignals(root string) (ToolCompilePreviewSignals, bool, error) {
	path := toolCompilePreviewSignalsPath(root)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ToolCompilePreviewSignals{}, false, nil
	}
	if err != nil {
		return ToolCompilePreviewSignals{}, false, fmt.Errorf("read compile preview signals: %w", err)
	}
	var signals ToolCompilePreviewSignals
	if err := json.Unmarshal(data, &signals); err != nil {
		return ToolCompilePreviewSignals{}, false, fmt.Errorf("decode compile preview signals: %w", err)
	}
	if signals.SchemaVersion != 0 && signals.SchemaVersion != 1 {
		return ToolCompilePreviewSignals{}, false, fmt.Errorf("unsupported compile preview signals schema_version: %d", signals.SchemaVersion)
	}
	return signals, true, nil
}

func WriteToolCompilePreviewSignals(root string, signals ToolCompilePreviewSignals) (ToolCompilePreviewSignalArtifact, error) {
	normalized, err := normalizeToolCompilePreviewSignals(signals)
	if err != nil {
		return ToolCompilePreviewSignalArtifact{}, err
	}
	if _, err := toolCompilePreviewOptionsFromSignals(normalized); err != nil {
		return ToolCompilePreviewSignalArtifact{}, err
	}
	dir := filepath.Join(root, "toolchain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolCompilePreviewSignalArtifact{}, fmt.Errorf("create compile preview signals directory: %w", err)
	}
	path := toolCompilePreviewSignalsPath(root)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return ToolCompilePreviewSignalArtifact{}, fmt.Errorf("encode compile preview signals: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ToolCompilePreviewSignalArtifact{}, fmt.Errorf("write compile preview signals: %w", err)
	}
	return ToolCompilePreviewSignalArtifact{
		Path:    path,
		Status:  "written",
		Mode:    "metadata-only",
		Signals: normalized,
	}, nil
}

func DeriveToolCompilePreviewSignalsFromReplay(ctx context.Context, root, runID string, options ToolCompilePreviewSignalDeriveOptions) (ToolCompilePreviewSignalDerivation, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("run id is required")
	}
	if options.MaxPlanNodes < 0 {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("max_plan_nodes must be non-negative")
	}
	eventDBPath := filepath.Join(root, "replay", "events.db")
	if _, err := os.Stat(eventDBPath); err != nil {
		if os.IsNotExist(err) {
			return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("replay event log not found: %s", eventDBPath)
		}
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("stat replay event log: %w", err)
	}
	store, err := eventlog.OpenSQLite(eventDBPath)
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, err
	}
	defer store.Close()

	events, err := store.Replay(ctx, runID)
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, err
	}
	if len(events) == 0 {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("run %s has no replay events", runID)
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("load toolchain manifests: %w", err)
	}
	signals, summary, err := toolCompilePreviewSignalsFromReplayEvents(events, manifests, options)
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, err
	}
	derivation := ToolCompilePreviewSignalDerivation{
		Path:    toolCompilePreviewSignalsPath(root),
		Status:  "dry-run",
		Mode:    "metadata-only",
		RunID:   runID,
		Signals: signals,
		Summary: summary,
	}
	if options.Write {
		artifact, err := WriteToolCompilePreviewSignals(root, signals)
		if err != nil {
			return ToolCompilePreviewSignalDerivation{}, err
		}
		derivation.Path = artifact.Path
		derivation.Status = artifact.Status
		derivation.Signals = artifact.Signals
	}
	return derivation, nil
}

func PreviewToolCompilePlan(root string, options ToolCompilePreviewOptions) (ToolCompilePreview, error) {
	persistedSignals, found, err := LoadToolCompilePreviewSignals(root)
	if err != nil {
		return ToolCompilePreview{}, err
	}
	if found {
		merged, err := mergeToolCompilePreviewOptions(options, persistedSignals)
		if err != nil {
			return ToolCompilePreview{}, err
		}
		options = merged
	}
	action := options.Action
	if strings.TrimSpace(string(action)) == "" {
		action = compiler.ActionDiscoverAssets
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolCompilePreview{}, fmt.Errorf("load toolchain manifests: %w", err)
	}
	plan, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
		Action: action,
		Evidence: compiler.EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "fact-workspace",
					Type:       evidence.NodeTarget,
					Label:      filepath.Base(root),
					SourceKind: "workspace",
				},
			},
		},
		Manifests:        manifests,
		Budget:           options.Budget,
		DomainPriorities: append([]compiler.DomainPriority(nil), options.DomainPriorities...),
		FrontierScores:   append([]compiler.CandidateFrontierScore(nil), options.FrontierScores...),
		HistoricalStats:  append([]compiler.CandidateHistory(nil), options.HistoricalStats...),
	})
	if err != nil {
		return ToolCompilePreview{}, err
	}
	return ToolCompilePreview{
		Workspace: root,
		Action:    action,
		Mode:      "metadata-only",
		Nodes:     append([]compiler.PlanNode(nil), plan.Nodes...),
		Warnings:  append([]string(nil), plan.Warnings...),
	}, nil
}

func CreateToolCompilePreviewReview(root string, options ToolCompilePreviewOptions) (ToolCompilePreviewReview, error) {
	preview, err := PreviewToolCompilePlan(root, options)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	review := ToolCompilePreviewReview{
		Status:  "dry-run",
		Mode:    "metadata-only",
		Preview: preview,
	}
	dir := toolCompilePreviewReviewDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("create compile preview review directory: %w", err)
	}
	review.Path = filepath.Join(dir, safeToolCompilePreviewReviewName(review)+".json")
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("encode compile preview review: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(review.Path, data, 0o644); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("write compile preview review: %w", err)
	}
	return review, nil
}

func ListToolCompilePreviewReviews(root string) ([]ToolCompilePreviewReview, error) {
	doctor, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	paths, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob compile preview reviews: %w", err)
	}
	sort.Strings(paths)
	reviews := make([]ToolCompilePreviewReview, 0, len(paths))
	for _, path := range paths {
		review, err := readToolCompilePreviewReviewFile(path)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func ReadToolCompilePreviewReview(root, reviewPath string) (ToolCompilePreviewReview, error) {
	doctor, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReview{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolCompilePreviewReviewPath(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	return readToolCompilePreviewReviewFile(path)
}

func AnnotateToolCompilePreviewReview(root, reviewPath string, options ToolCompilePreviewReviewAnnotationOptions) (ToolCompilePreviewReview, error) {
	annotation, err := normalizeToolCompilePreviewReviewAnnotation(options)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReview{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolCompilePreviewReviewPath(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	review, err := readToolCompilePreviewReviewFile(path)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	review.Annotation = &annotation
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("encode compile preview review annotation: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("write compile preview review annotation: %w", err)
	}
	review.Path = path
	return review, nil
}

func CompareToolCompilePreviewReviews(root, baseReviewPath, candidateReviewPath string) (ToolCompilePreviewReviewComparison, error) {
	base, err := ReadToolCompilePreviewReview(root, baseReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewComparison{}, err
	}
	candidate, err := ReadToolCompilePreviewReview(root, candidateReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewComparison{}, err
	}
	return compareToolCompilePreviewReviewValues(base, candidate), nil
}

func SelectToolCompilePreviewReview(root, reviewPath string, options ToolCompilePreviewReviewSelectionOptions) (ToolCompilePreviewReviewSelection, error) {
	review, err := ReadToolCompilePreviewReview(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	reviews, err := ListToolCompilePreviewReviews(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := make([]string, 0, len(reviews))
	for _, candidate := range reviews {
		if candidate.Path == "" || candidate.Path == review.Path {
			continue
		}
		superseded = append(superseded, candidate.Path)
	}
	sort.Strings(superseded)
	selection := ToolCompilePreviewReviewSelection{
		Path:            toolCompilePreviewReviewSelectionPath(root),
		Status:          "current",
		Mode:            "metadata-only",
		ReviewPath:      review.Path,
		Action:          review.Preview.Action,
		ReviewNodes:     len(review.Preview.Nodes),
		ReviewWarnings:  len(review.Preview.Warnings),
		SupersededPaths: superseded,
		SelectedBy:      selectedBy,
		SelectedAt:      selectedAt.UTC().Format(time.RFC3339),
		Reason:          strings.TrimSpace(options.Reason),
	}
	if err := writeToolCompilePreviewReviewSelection(selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	return selection, nil
}

func ClearToolCompilePreviewReviewSelection(root string, options ToolCompilePreviewReviewSelectionClearOptions) (ToolCompilePreviewReviewSelection, error) {
	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	if !found {
		selection = ToolCompilePreviewReviewSelection{
			Path:   toolCompilePreviewReviewSelectionPath(root),
			Mode:   "metadata-only",
			Status: "cleared",
		}
	}
	clearedBy := strings.TrimSpace(options.ClearedBy)
	if clearedBy == "" {
		clearedBy = "operator"
	}
	clearedAt := options.ClearedAt
	if clearedAt.IsZero() {
		clearedAt = time.Now().UTC()
	}
	selection.Path = toolCompilePreviewReviewSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeToolCompilePreviewReviewSelection(selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	return selection, nil
}

func writeToolCompilePreviewReviewSelection(selection ToolCompilePreviewReviewSelection) error {
	if err := os.MkdirAll(filepath.Dir(selection.Path), 0o755); err != nil {
		return fmt.Errorf("create compile preview review selection directory: %w", err)
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compile preview review selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write compile preview review selection: %w", err)
	}
	return nil
}

func toolCompilePreviewReviewSelectionIsCurrent(selection ToolCompilePreviewReviewSelection) bool {
	return strings.TrimSpace(selection.Status) == "current"
}

func toolCompilePreviewReviewSelectionIsCleared(selection ToolCompilePreviewReviewSelection) bool {
	return strings.TrimSpace(selection.Status) == "cleared"
}

func ReadToolCompilePreviewReviewSelection(root string) (ToolCompilePreviewReviewSelection, bool, error) {
	doctor, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, false, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path := toolCompilePreviewReviewSelectionPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolCompilePreviewReviewSelection{}, false, nil
		}
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("read compile preview review selection %s: %w", path, err)
	}
	var selection ToolCompilePreviewReviewSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("decode compile preview review selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func CreateToolCompilePreviewReviewExecutionDispatch(root, reviewPath string, options ToolCompilePreviewReviewExecutionDispatchOptions) (ToolCompilePreviewReviewExecutionDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	resolvedReviewPath, err := resolveToolCompilePreviewReviewPath(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	review, err := readToolCompilePreviewReviewFile(resolvedReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	if strings.TrimSpace(review.Status) != "dry-run" || strings.TrimSpace(review.Mode) != "metadata-only" {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("compile preview review must be dry-run metadata-only before dispatch")
	}
	if len(review.Preview.Nodes) == 0 {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("compile preview review has no nodes to dispatch")
	}
	selection, selectionFound, err := readToolCompilePreviewReviewSelectionForExecution(root)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	if !selectionFound || !toolCompilePreviewReviewSelectionIsCurrent(selection) {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("compile preview review execution dispatch requires a current review selection")
	}
	if strings.TrimSpace(selection.ReviewPath) != strings.TrimSpace(review.Path) {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("compile preview review is not the current selection: selected=%s review=%s", selection.ReviewPath, review.Path)
	}
	createdAt := options.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	dispatch := ToolCompilePreviewReviewExecutionDispatch{
		Status:         "dry-run",
		Mode:           "metadata-only",
		ReviewPath:     review.Path,
		SelectionPath:  selection.Path,
		Action:         review.Preview.Action,
		ReviewNodes:    len(review.Preview.Nodes),
		ReviewWarnings: len(review.Preview.Warnings),
		CreatedAt:      createdAt.UTC().Format(time.RFC3339),
		Actor:          actor,
		Reason:         strings.TrimSpace(options.Reason),
	}
	if err := writeToolCompilePreviewReviewExecutionDispatch(root, &dispatch); err != nil {
		return dispatch, err
	}
	return dispatch, nil
}

func ExecuteToolCompilePreviewReview(ctx context.Context, root, reviewPath string, options ToolCompilePreviewReviewExecutionOptions) (ToolCompilePreviewReviewExecution, error) {
	if !options.Execute {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review execution requires explicit execute")
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	scopeConfig, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("load scope config: %w", err)
	}
	if err := ValidateScopeConfig(scopeConfig); err != nil {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("invalid scope config: %w", err)
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("load toolchain manifests: %w", err)
	}
	if err := validatePackExecutionPoliciesAgainstManifests(scopeConfig.Execution.PackPolicies, manifests); err != nil {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("invalid scope config: %w", err)
	}
	resolvedReviewPath, err := resolveToolCompilePreviewReviewPath(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	review, err := readToolCompilePreviewReviewFile(resolvedReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	if strings.TrimSpace(review.Status) != "dry-run" || strings.TrimSpace(review.Mode) != "metadata-only" {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review must be dry-run metadata-only before execution")
	}
	if len(review.Preview.Nodes) == 0 {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review has no nodes to execute")
	}
	selection, selectionFound, selectionErr := readToolCompilePreviewReviewSelectionForExecution(root)
	if selectionErr != nil {
		return ToolCompilePreviewReviewExecution{}, selectionErr
	}
	if !selectionFound || !toolCompilePreviewReviewSelectionIsCurrent(selection) {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review execution requires a current review selection")
	}
	if strings.TrimSpace(selection.ReviewPath) != strings.TrimSpace(review.Path) {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review is not the current selection: selected=%s review=%s", selection.ReviewPath, review.Path)
	}

	executedAt := options.ExecutedAt
	if executedAt.IsZero() {
		executedAt = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	runID := fmt.Sprintf("compile-preview-review-execution-%d", executedAt.UTC().UnixNano())
	execution := ToolCompilePreviewReviewExecution{
		Status:         "completed",
		Mode:           "execute",
		RunID:          runID,
		DispatchPath:   strings.TrimSpace(options.DispatchPath),
		ReviewPath:     review.Path,
		Action:         review.Preview.Action,
		ReviewNodes:    len(review.Preview.Nodes),
		ReviewWarnings: len(review.Preview.Warnings),
		ExecutedAt:     executedAt.UTC().Format(time.RFC3339),
		Actor:          actor,
		Reason:         strings.TrimSpace(options.Reason),
		Warnings:       append([]string(nil), doctor.Warnings...),
	}
	execution.SelectionPath = selection.Path
	if _, err := ensureToolCompilePreviewReviewExecutionDirectory(root); err != nil {
		return execution, err
	}
	if err := os.MkdirAll(filepath.Join(root, "replay"), 0o755); err != nil {
		return execution, fmt.Errorf("create replay directory: %w", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return execution, err
	}
	defer store.Close()
	persistedEvents := []eventlog.Event{}
	runOptions := RunOptions{
		ToolRunner:                 options.ToolRunner,
		ToolConcurrency:            options.ToolConcurrency,
		RequireSafetyAuthorization: true,
	}
	if err := appendRunEvent(ctx, store, workspaceSafetyEvent(runID, doctor, scopeConfig, true), runOptions, &persistedEvents); err != nil {
		return execution, err
	}
	if err := appendRunEvent(ctx, store, stageEvent(runID, "tool-execution", "Execute selected compile preview review nodes", "started"), runOptions, &persistedEvents); err != nil {
		return execution, err
	}
	err = runToolPlanNodesExecutionStage(ctx, root, runID, "tool-execution", "Execute selected compile preview review nodes", review.Preview.Nodes, scopeConfig.Scope.Targets, scopeConfig.Scope.Exclusions, scopeConfig.Scope.TargetPolicy, scopeConfig.Execution.Isolation, scopeConfig.Execution.Container, scopeConfig.Execution.ResourceLimits, scopeConfig.Execution.PackPolicies, scopeConfig.Execution.WorkerPools, toolExecutionConcurrency(scopeConfig, runOptions), toolRunnerFromOptions(runOptions), store, runOptions, &persistedEvents)
	execution.EventCount = len(persistedEvents)
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		if writeErr := writeToolCompilePreviewReviewExecution(root, &execution); writeErr != nil {
			return execution, writeErr
		}
		return execution, err
	}
	if err := writeToolCompilePreviewReviewExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func ExecuteToolCompilePreviewReviewExecutionDispatch(ctx context.Context, root, dispatchPath string, options ToolCompilePreviewReviewExecutionOptions) (ToolCompilePreviewReviewExecution, error) {
	if !options.Execute {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review execution dispatch requires explicit execute")
	}
	dispatch, err := ReadToolCompilePreviewReviewExecutionDispatch(root, dispatchPath)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review execution dispatch must be dry-run metadata-only before execution")
	}
	review, err := ReadToolCompilePreviewReview(root, dispatch.ReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	if strings.TrimSpace(review.Status) != "dry-run" || strings.TrimSpace(review.Mode) != "metadata-only" {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review must be dry-run metadata-only before execution")
	}
	if dispatch.Action != review.Preview.Action || dispatch.ReviewNodes != len(review.Preview.Nodes) || dispatch.ReviewWarnings != len(review.Preview.Warnings) {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("compile preview review execution dispatch is stale: dispatch nodes=%d warnings=%d action=%s review nodes=%d warnings=%d action=%s", dispatch.ReviewNodes, dispatch.ReviewWarnings, dispatch.Action, len(review.Preview.Nodes), len(review.Preview.Warnings), review.Preview.Action)
	}
	options.DispatchPath = dispatch.Path
	return ExecuteToolCompilePreviewReview(ctx, root, review.Path, options)
}

func ListToolCompilePreviewReviewExecutionDispatches(root string) ([]ToolCompilePreviewReviewExecutionDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	if err := rejectUnsafeAuthoredToolDirectory(toolCompilePreviewReviewExecutionDispatchDir(root), "compile preview review execution dispatch directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewExecutionDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob compile preview review execution dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]ToolCompilePreviewReviewExecutionDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readToolCompilePreviewReviewExecutionDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadToolCompilePreviewReviewExecutionDispatch(root, dispatchPath string) (ToolCompilePreviewReviewExecutionDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolCompilePreviewReviewExecutionDispatchPath(root, dispatchPath)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	return readToolCompilePreviewReviewExecutionDispatchFile(path)
}

func writeToolCompilePreviewReviewExecutionDispatch(root string, dispatch *ToolCompilePreviewReviewExecutionDispatch) error {
	dir, err := ensureToolCompilePreviewReviewExecutionDispatchDirectory(root)
	if err != nil {
		return err
	}
	name := safeInstallRequestName(strings.Join([]string{
		"compile-preview-review-execution-dispatch",
		dispatch.ReviewPath,
		dispatch.SelectionPath,
		dispatch.CreatedAt,
	}, "|"))
	dispatch.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compile preview review execution dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := rejectUnsafeAuthoredToolFile(dispatch.Path, "compile preview review execution dispatch file"); err != nil {
		return err
	}
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return fmt.Errorf("write compile preview review execution dispatch: %w", err)
	}
	return nil
}

func readToolCompilePreviewReviewExecutionDispatchFile(path string) (ToolCompilePreviewReviewExecutionDispatch, error) {
	if err := rejectUnsafeAuthoredToolFile(path, "compile preview review execution dispatch file"); err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("read compile preview review execution dispatch %s: %w", path, err)
	}
	var dispatch ToolCompilePreviewReviewExecutionDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return ToolCompilePreviewReviewExecutionDispatch{}, fmt.Errorf("decode compile preview review execution dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func ensureToolCompilePreviewReviewExecutionDispatchDirectory(root string) (string, error) {
	if err := requireAuthoredToolDirectory(filepath.Join(root, "toolchain"), "toolchain directory"); err != nil {
		return "", err
	}
	dir := toolCompilePreviewReviewExecutionDispatchDir(root)
	if err := rejectUnsafeAuthoredToolDirectory(dir, "compile preview review execution dispatch directory"); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create compile preview review execution dispatch directory: %w", err)
	}
	return dir, nil
}

func readToolCompilePreviewReviewSelectionForExecution(root string) (ToolCompilePreviewReviewSelection, bool, error) {
	path := toolCompilePreviewReviewSelectionPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolCompilePreviewReviewSelection{}, false, nil
		}
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("read compile preview review selection %s: %w", path, err)
	}
	var selection ToolCompilePreviewReviewSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("decode compile preview review selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func ListToolCompilePreviewReviewExecutions(root string) ([]ToolCompilePreviewReviewExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	if err := rejectUnsafeAuthoredToolDirectory(toolCompilePreviewReviewExecutionDir(root), "compile preview review execution directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob compile preview review executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ToolCompilePreviewReviewExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readToolCompilePreviewReviewExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadToolCompilePreviewReviewExecution(root, executionPath string) (ToolCompilePreviewReviewExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolCompilePreviewReviewExecutionPath(root, executionPath)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	return readToolCompilePreviewReviewExecutionFile(path)
}

func writeToolCompilePreviewReviewExecution(root string, execution *ToolCompilePreviewReviewExecution) error {
	dir, err := ensureToolCompilePreviewReviewExecutionDirectory(root)
	if err != nil {
		return err
	}
	name := safeInstallRequestName(strings.Join([]string{
		"compile-preview-review-execution",
		execution.ReviewPath,
		execution.RunID,
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "|"))
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compile preview review execution: %w", err)
	}
	data = append(data, '\n')
	if err := rejectUnsafeAuthoredToolFile(execution.Path, "compile preview review execution file"); err != nil {
		return err
	}
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write compile preview review execution: %w", err)
	}
	return nil
}

func readToolCompilePreviewReviewExecutionFile(path string) (ToolCompilePreviewReviewExecution, error) {
	if err := rejectUnsafeAuthoredToolFile(path, "compile preview review execution file"); err != nil {
		return ToolCompilePreviewReviewExecution{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("read compile preview review execution %s: %w", path, err)
	}
	var execution ToolCompilePreviewReviewExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ToolCompilePreviewReviewExecution{}, fmt.Errorf("decode compile preview review execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func ensureToolCompilePreviewReviewExecutionDirectory(root string) (string, error) {
	if err := requireAuthoredToolDirectory(filepath.Join(root, "toolchain"), "toolchain directory"); err != nil {
		return "", err
	}
	dir := toolCompilePreviewReviewExecutionDir(root)
	if err := rejectUnsafeAuthoredToolDirectory(dir, "compile preview review execution directory"); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create compile preview review execution directory: %w", err)
	}
	if err := requireAuthoredToolDirectory(dir, "compile preview review execution directory"); err != nil {
		return "", err
	}
	return dir, nil
}

func readToolCompilePreviewReviewFile(path string) (ToolCompilePreviewReview, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("read compile preview review %s: %w", path, err)
	}
	var review ToolCompilePreviewReview
	if err := json.Unmarshal(data, &review); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("decode compile preview review %s: %w", path, err)
	}
	review.Path = path
	return review, nil
}

func compareToolCompilePreviewReviewValues(base, candidate ToolCompilePreviewReview) ToolCompilePreviewReviewComparison {
	comparison := ToolCompilePreviewReviewComparison{
		Status: "compared",
		Mode:   "metadata-only",
		Base: ToolCompilePreviewReviewComparisonSide{
			Path:     base.Path,
			Action:   base.Preview.Action,
			Nodes:    len(base.Preview.Nodes),
			Warnings: len(base.Preview.Warnings),
			Decision: compilePreviewReviewDecision(base),
		},
		Candidate: ToolCompilePreviewReviewComparisonSide{
			Path:     candidate.Path,
			Action:   candidate.Preview.Action,
			Nodes:    len(candidate.Preview.Nodes),
			Warnings: len(candidate.Preview.Warnings),
			Decision: compilePreviewReviewDecision(candidate),
		},
	}
	comparison.Summary.NodeDelta = len(candidate.Preview.Nodes) - len(base.Preview.Nodes)
	comparison.Summary.WarningDelta = len(candidate.Preview.Warnings) - len(base.Preview.Warnings)
	baseNodes := map[string]compiler.PlanNode{}
	baseIndexes := map[string]int{}
	for index, node := range base.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		baseNodes[key] = node
		baseIndexes[key] = index
	}
	candidateNodes := map[string]compiler.PlanNode{}
	candidateIndexes := map[string]int{}
	for index, node := range candidate.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		candidateNodes[key] = node
		candidateIndexes[key] = index
	}
	for index, node := range candidate.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		baseNode, ok := baseNodes[key]
		if !ok {
			comparison.Summary.AddedNodes++
			comparison.NodeDiffs = append(comparison.NodeDiffs, ToolCompilePreviewReviewNodeDiff{
				Change:         "added",
				ManifestName:   node.ManifestName,
				ToolID:         node.ToolID,
				AdapterID:      node.AdapterID,
				Operation:      node.Operation,
				ParserID:       node.ParserID,
				TargetKind:     node.TargetKind,
				DependencyName: node.DependencyName,
				DependencyKind: node.DependencyKind,
				Capability:     node.Capability,
				CandidateID:    node.ID,
				BaseIndex:      -1,
				CandidateIndex: index,
				CandidateScore: node.SelectionScore,
				CandidateSignals: append(
					[]string(nil),
					node.SelectionSignals...,
				),
			})
			continue
		}
		changes := []string{}
		baseIndex := baseIndexes[key]
		if baseIndex != index {
			changes = append(changes, "reordered")
			comparison.Summary.ReorderedNodes++
		}
		if baseNode.SelectionScore != node.SelectionScore {
			changes = append(changes, "score")
			comparison.Summary.ScoreChangedNodes++
		}
		if strings.Join(baseNode.SelectionSignals, "\x00") != strings.Join(node.SelectionSignals, "\x00") {
			changes = append(changes, "signals")
			comparison.Summary.SignalChangedNodes++
		}
		if len(changes) == 0 {
			continue
		}
		comparison.Summary.ChangedNodes++
		comparison.NodeDiffs = append(comparison.NodeDiffs, ToolCompilePreviewReviewNodeDiff{
			Change:           strings.Join(changes, ","),
			ManifestName:     node.ManifestName,
			ToolID:           node.ToolID,
			AdapterID:        node.AdapterID,
			Operation:        node.Operation,
			ParserID:         node.ParserID,
			TargetKind:       node.TargetKind,
			DependencyName:   node.DependencyName,
			DependencyKind:   node.DependencyKind,
			Capability:       node.Capability,
			BaseID:           baseNode.ID,
			CandidateID:      node.ID,
			BaseIndex:        baseIndex,
			CandidateIndex:   index,
			BaseScore:        baseNode.SelectionScore,
			CandidateScore:   node.SelectionScore,
			BaseSignals:      append([]string(nil), baseNode.SelectionSignals...),
			CandidateSignals: append([]string(nil), node.SelectionSignals...),
		})
	}
	for index, node := range base.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		if _, ok := candidateNodes[key]; ok {
			continue
		}
		comparison.Summary.RemovedNodes++
		comparison.NodeDiffs = append(comparison.NodeDiffs, ToolCompilePreviewReviewNodeDiff{
			Change:         "removed",
			ManifestName:   node.ManifestName,
			ToolID:         node.ToolID,
			AdapterID:      node.AdapterID,
			Operation:      node.Operation,
			ParserID:       node.ParserID,
			TargetKind:     node.TargetKind,
			DependencyName: node.DependencyName,
			DependencyKind: node.DependencyKind,
			Capability:     node.Capability,
			BaseID:         node.ID,
			BaseIndex:      index,
			CandidateIndex: -1,
			BaseScore:      node.SelectionScore,
			BaseSignals:    append([]string(nil), node.SelectionSignals...),
		})
	}
	comparison.WarningDiffs = compilePreviewReviewWarningDiffs(base.Preview.Warnings, candidate.Preview.Warnings)
	return comparison
}

func compilePreviewReviewDecision(review ToolCompilePreviewReview) string {
	if review.Annotation == nil {
		return ""
	}
	return strings.TrimSpace(review.Annotation.Decision)
}

func compilePreviewReviewNodeKey(node compiler.PlanNode) string {
	return strings.Join([]string{
		strings.TrimSpace(node.ManifestName),
		strings.TrimSpace(node.ToolID),
		strings.TrimSpace(node.AdapterID),
		strings.TrimSpace(node.Operation),
		strings.TrimSpace(node.ParserID),
		strings.TrimSpace(node.TargetKind),
		strings.TrimSpace(node.DependencyName),
		strings.TrimSpace(node.DependencyKind),
		strings.TrimSpace(node.Capability),
		strings.TrimSpace(node.Runner),
		strings.TrimSpace(node.Parser),
		strings.Join(node.RunCommand, "\x00"),
	}, "\x00")
}

func compilePreviewReviewWarningDiffs(baseWarnings, candidateWarnings []string) []ToolCompilePreviewReviewWarningDiff {
	baseCounts := map[string]int{}
	for _, warning := range baseWarnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		baseCounts[trimmed]++
	}
	candidateCounts := map[string]int{}
	for _, warning := range candidateWarnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		candidateCounts[trimmed]++
	}
	keys := make([]string, 0, len(baseCounts)+len(candidateCounts))
	seen := map[string]bool{}
	for warning := range baseCounts {
		keys = append(keys, warning)
		seen[warning] = true
	}
	for warning := range candidateCounts {
		if !seen[warning] {
			keys = append(keys, warning)
		}
	}
	sort.Strings(keys)
	diffs := []ToolCompilePreviewReviewWarningDiff{}
	for _, warning := range keys {
		if candidateCounts[warning] > baseCounts[warning] {
			for i := 0; i < candidateCounts[warning]-baseCounts[warning]; i++ {
				diffs = append(diffs, ToolCompilePreviewReviewWarningDiff{Change: "added", Warning: warning})
			}
		}
		if baseCounts[warning] > candidateCounts[warning] {
			for i := 0; i < baseCounts[warning]-candidateCounts[warning]; i++ {
				diffs = append(diffs, ToolCompilePreviewReviewWarningDiff{Change: "removed", Warning: warning})
			}
		}
	}
	return diffs
}

func normalizeToolCompilePreviewReviewAnnotation(options ToolCompilePreviewReviewAnnotationOptions) (ToolCompilePreviewReviewAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return ToolCompilePreviewReviewAnnotation{}, fmt.Errorf("compile preview review annotation decision is required")
	default:
		return ToolCompilePreviewReviewAnnotation{}, fmt.Errorf("unsupported compile preview review annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return ToolCompilePreviewReviewAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func toolCompilePreviewSignalsPath(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-signals.json")
}

func normalizeToolCompilePreviewSignals(signals ToolCompilePreviewSignals) (ToolCompilePreviewSignals, error) {
	if signals.SchemaVersion == 0 {
		signals.SchemaVersion = 1
	}
	if signals.SchemaVersion != 1 {
		return ToolCompilePreviewSignals{}, fmt.Errorf("unsupported compile preview signals schema_version: %d", signals.SchemaVersion)
	}
	if signals.Budget.MaxPlanNodes < 0 {
		return ToolCompilePreviewSignals{}, fmt.Errorf("compile preview signals budget max_plan_nodes must be non-negative")
	}
	return signals, nil
}

func toolCompilePreviewSignalsFromReplayEvents(events []eventlog.Event, manifests []toolchain.Manifest, options ToolCompilePreviewSignalDeriveOptions) (ToolCompilePreviewSignals, ToolCompilePreviewSignalDerivationSummary, error) {
	signals := ToolCompilePreviewSignals{
		SchemaVersion: 1,
		Budget:        ToolCompilePreviewSignalBudget{MaxPlanNodes: options.MaxPlanNodes},
	}
	summary := ToolCompilePreviewSignalDerivationSummary{}
	historyByKey := map[string]ToolCompilePreviewSignalHistory{}
	frontierByKey := map[string]ToolCompilePreviewSignalFrontierScore{}
	for _, event := range events {
		switch event.Type {
		case eventlog.EventTool:
			var execution struct {
				Tool            string `json:"tool"`
				ExecutionStatus string `json:"execution_status"`
				Capability      string `json:"capability"`
			}
			if err := json.Unmarshal(event.Payload, &execution); err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("decode replay tool event %d: %w", event.ID, err)
			}
			status := strings.ToLower(strings.TrimSpace(execution.ExecutionStatus))
			if status == "" {
				continue
			}
			summary.ToolExecutions++
			manifestName := strings.TrimSpace(execution.Tool)
			capability := strings.TrimSpace(execution.Capability)
			if manifestName == "" || capability == "" {
				summary.Ignored++
				continue
			}
			key := manifestName + "\x00" + capability
			stat := historyByKey[key]
			stat.ManifestName = manifestName
			stat.Capability = capability
			switch status {
			case "completed":
				stat.Successes++
				summary.Successes++
			case "failed":
				stat.Failures++
				summary.Failures++
			default:
				summary.Ignored++
				continue
			}
			historyByKey[key] = stat
		case eventlog.EventFact:
			var candidatePath toolCompilePreviewReplayCandidatePathPayload
			if err := json.Unmarshal(event.Payload, &candidatePath); err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("decode replay fact event %d: %w", event.ID, err)
			}
			scores, err := toolCompilePreviewFrontierScoresFromReplayCandidatePath(candidatePath, manifests)
			if err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("derive frontier score from fact event %d: %w", event.ID, err)
			}
			for _, score := range scores {
				key := strings.TrimSpace(score.ManifestName) + "\x00" + strings.TrimSpace(score.Capability)
				if strings.TrimSpace(key) == "\x00" {
					continue
				}
				if current, ok := frontierByKey[key]; ok && frontier.Score(current.Path.toCandidatePath()) >= frontier.Score(score.Path.toCandidatePath()) {
					continue
				}
				frontierByKey[key] = score
			}
		case eventlog.EventIntent:
			var intent toolCompilePreviewReplayIntentPayload
			if err := json.Unmarshal(event.Payload, &intent); err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("decode replay intent event %d: %w", event.ID, err)
			}
			scores, err := toolCompilePreviewFrontierScoresFromReplayIntent(intent, manifests)
			if err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("derive frontier score from intent event %d: %w", event.ID, err)
			}
			for _, score := range scores {
				key := strings.TrimSpace(score.ManifestName) + "\x00" + strings.TrimSpace(score.Capability)
				if strings.TrimSpace(key) == "\x00" {
					continue
				}
				if current, ok := frontierByKey[key]; ok && frontier.Score(current.Path.toCandidatePath()) >= frontier.Score(score.Path.toCandidatePath()) {
					continue
				}
				frontierByKey[key] = score
			}
		default:
			continue
		}
	}
	keys := make([]string, 0, len(historyByKey))
	for key := range historyByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		signals.HistoricalStats = append(signals.HistoricalStats, historyByKey[key])
	}
	frontierKeys := make([]string, 0, len(frontierByKey))
	for key := range frontierByKey {
		frontierKeys = append(frontierKeys, key)
	}
	sort.Strings(frontierKeys)
	for _, key := range frontierKeys {
		signals.FrontierScores = append(signals.FrontierScores, frontierByKey[key])
	}
	summary.FrontierScores = len(signals.FrontierScores)
	return signals, summary, nil
}

type toolCompilePreviewReplayIntentPayload struct {
	ID                 string  `json:"id"`
	IntentID           string  `json:"intent_id"`
	Kind               string  `json:"kind"`
	Goal               string  `json:"goal"`
	Target             string  `json:"target"`
	Rationale          string  `json:"rationale"`
	Action             string  `json:"action"`
	Domain             string  `json:"domain"`
	ManifestName       string  `json:"manifest_name"`
	Capability         string  `json:"capability"`
	Confidence         float64 `json:"confidence"`
	MissionValue       float64 `json:"mission_value"`
	UnlockPotential    float64 `json:"unlock_potential"`
	EvidenceConfidence float64 `json:"evidence_confidence"`
	Novelty            float64 `json:"novelty"`
	Cost               float64 `json:"cost"`
	DuplicateRisk      float64 `json:"duplicate_risk"`
	Noise              float64 `json:"noise"`
	ExhaustedPenalty   float64 `json:"exhausted_penalty"`
}

type toolCompilePreviewReplayCandidatePathPayload struct {
	ID                 string  `json:"id"`
	FactID             string  `json:"fact_id"`
	Kind               string  `json:"kind"`
	Statement          string  `json:"statement"`
	Target             string  `json:"target"`
	ExpectedUnlock     string  `json:"expected_unlock"`
	Action             string  `json:"action"`
	Domain             string  `json:"domain"`
	ManifestName       string  `json:"manifest_name"`
	Capability         string  `json:"capability"`
	Confidence         float64 `json:"confidence"`
	MissionValue       float64 `json:"mission_value"`
	UnlockPotential    float64 `json:"unlock_potential"`
	EvidenceConfidence float64 `json:"evidence_confidence"`
	Novelty            float64 `json:"novelty"`
	Cost               float64 `json:"cost"`
	DuplicateRisk      float64 `json:"duplicate_risk"`
	Noise              float64 `json:"noise"`
	ExhaustedPenalty   float64 `json:"exhausted_penalty"`
}

func toolCompilePreviewFrontierScoresFromReplayIntent(intent toolCompilePreviewReplayIntentPayload, manifests []toolchain.Manifest) ([]ToolCompilePreviewSignalFrontierScore, error) {
	if strings.TrimSpace(intent.Kind) != "hypothesis" {
		return nil, nil
	}
	path := toolCompilePreviewSignalPathFromIntent(intent)
	return toolCompilePreviewFrontierScoresFromMetadata(manifests, toolCompilePreviewFrontierMetadata{
		ManifestName: intent.ManifestName,
		Capability:   intent.Capability,
		Action:       intent.Action,
		Domain:       intent.Domain,
		Target:       intent.Target,
		ID:           firstNonEmptyString(intent.IntentID, intent.ID),
		Path:         path,
	})
}

func toolCompilePreviewFrontierScoresFromReplayCandidatePath(candidate toolCompilePreviewReplayCandidatePathPayload, manifests []toolchain.Manifest) ([]ToolCompilePreviewSignalFrontierScore, error) {
	if !toolCompilePreviewCandidatePathKind(candidate.Kind) {
		return nil, nil
	}
	path := toolCompilePreviewSignalPathFromCandidatePath(candidate)
	return toolCompilePreviewFrontierScoresFromMetadata(manifests, toolCompilePreviewFrontierMetadata{
		ManifestName: candidate.ManifestName,
		Capability:   candidate.Capability,
		Action:       candidate.Action,
		Domain:       candidate.Domain,
		Target:       candidate.Target,
		ID:           firstNonEmptyString(candidate.FactID, candidate.ID),
		Path:         path,
	})
}

type toolCompilePreviewFrontierMetadata struct {
	ManifestName string
	Capability   string
	Action       string
	Domain       string
	Target       string
	ID           string
	Path         ToolCompilePreviewSignalPath
}

func toolCompilePreviewFrontierScoresFromMetadata(manifests []toolchain.Manifest, metadata toolCompilePreviewFrontierMetadata) ([]ToolCompilePreviewSignalFrontierScore, error) {
	if toolCompilePreviewSignalPathScore(metadata.Path) == 0 {
		return nil, nil
	}
	manifestName := strings.TrimSpace(metadata.ManifestName)
	capability := strings.TrimSpace(metadata.Capability)
	if manifestName != "" && capability != "" {
		return []ToolCompilePreviewSignalFrontierScore{{
			ManifestName: manifestName,
			Capability:   capability,
			Path:         metadata.Path,
		}}, nil
	}
	action := compiler.SemanticAction(strings.TrimSpace(metadata.Action))
	if action == "" {
		return nil, nil
	}
	domain, ok := toolCompilePreviewDomainForFields(metadata.Domain, metadata.Target, metadata.ID)
	if !ok {
		return nil, nil
	}
	plan, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
		Action: action,
		Evidence: compiler.EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "frontier-" + strings.TrimSpace(metadata.Path.ID),
					Type:       evidence.NodeHypothesis,
					Label:      strings.TrimSpace(metadata.Path.Hypothesis),
					SourceKind: "replay-frontier",
				},
			},
		},
		Manifests: manifests,
	})
	if err != nil {
		return nil, nil
	}
	scores := []ToolCompilePreviewSignalFrontierScore{}
	for _, node := range plan.Nodes {
		nodeDomain, ok := toolCompilePreviewDomainForCapability(node.Capability)
		if !ok || nodeDomain != domain {
			continue
		}
		scores = append(scores, ToolCompilePreviewSignalFrontierScore{
			ManifestName: node.ManifestName,
			Capability:   node.Capability,
			Path:         metadata.Path,
		})
	}
	return scores, nil
}

func toolCompilePreviewCandidatePathKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "candidate_path", "frontier_path", "frontier_candidate":
		return true
	default:
		return false
	}
}

func toolCompilePreviewSignalPathFromIntent(intent toolCompilePreviewReplayIntentPayload) ToolCompilePreviewSignalPath {
	confidence := intent.Confidence
	path := ToolCompilePreviewSignalPath{
		ID:                 firstNonEmptyString(intent.IntentID, intent.ID),
		Target:             strings.TrimSpace(intent.Target),
		Hypothesis:         strings.TrimSpace(intent.Goal),
		ExpectedUnlock:     strings.TrimSpace(intent.Rationale),
		MissionValue:       intent.MissionValue,
		UnlockPotential:    intent.UnlockPotential,
		EvidenceConfidence: intent.EvidenceConfidence,
		Novelty:            intent.Novelty,
		Cost:               intent.Cost,
		DuplicateRisk:      intent.DuplicateRisk,
		Noise:              intent.Noise,
		ExhaustedPenalty:   intent.ExhaustedPenalty,
	}
	return toolCompilePreviewSignalPathWithConfidence(path, confidence)
}

func toolCompilePreviewSignalPathFromCandidatePath(candidate toolCompilePreviewReplayCandidatePathPayload) ToolCompilePreviewSignalPath {
	path := ToolCompilePreviewSignalPath{
		ID:                 firstNonEmptyString(candidate.FactID, candidate.ID),
		Target:             strings.TrimSpace(candidate.Target),
		Hypothesis:         strings.TrimSpace(candidate.Statement),
		ExpectedUnlock:     strings.TrimSpace(candidate.ExpectedUnlock),
		MissionValue:       candidate.MissionValue,
		UnlockPotential:    candidate.UnlockPotential,
		EvidenceConfidence: candidate.EvidenceConfidence,
		Novelty:            candidate.Novelty,
		Cost:               candidate.Cost,
		DuplicateRisk:      candidate.DuplicateRisk,
		Noise:              candidate.Noise,
		ExhaustedPenalty:   candidate.ExhaustedPenalty,
	}
	return toolCompilePreviewSignalPathWithConfidence(path, candidate.Confidence)
}

func toolCompilePreviewSignalPathWithConfidence(path ToolCompilePreviewSignalPath, confidence float64) ToolCompilePreviewSignalPath {
	if confidence > 0 {
		if path.MissionValue == 0 {
			path.MissionValue = confidence
		}
		if path.UnlockPotential == 0 {
			path.UnlockPotential = confidence
		}
		if path.EvidenceConfidence == 0 {
			path.EvidenceConfidence = confidence
		}
	}
	return path
}

func toolCompilePreviewSignalPathScore(path ToolCompilePreviewSignalPath) float64 {
	return frontier.Score(path.toCandidatePath())
}

func toolCompilePreviewDomainForFields(domainText, targetText, idText string) (frontier.Domain, bool) {
	if domain, err := parseToolCompilePreviewSignalDomain(domainText); err == nil {
		return domain, true
	}
	target := strings.TrimSpace(targetText)
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return frontier.DomainWebAPI, true
	}
	if _, err := netip.ParsePrefix(target); err == nil {
		return frontier.DomainInfra, true
	}
	if _, err := netip.ParseAddr(target); err == nil {
		return frontier.DomainInfra, true
	}
	id := strings.ToLower(strings.TrimSpace(idText))
	switch {
	case strings.HasPrefix(id, "web-"), strings.HasPrefix(id, "web_"):
		return frontier.DomainWebAPI, true
	case strings.HasPrefix(id, "infra-"), strings.HasPrefix(id, "infra_"):
		return frontier.DomainInfra, true
	case strings.HasPrefix(id, "ctf-"), strings.HasPrefix(id, "ctf_"):
		return frontier.DomainCTF, true
	}
	if target != "" {
		return frontier.DomainCTF, true
	}
	return "", false
}

func toolCompilePreviewDomainForCapability(capability string) (frontier.Domain, bool) {
	switch {
	case strings.HasPrefix(capability, "web."):
		return frontier.DomainWebAPI, true
	case strings.HasPrefix(capability, "infra."):
		return frontier.DomainInfra, true
	case strings.HasPrefix(capability, "ctf."):
		return frontier.DomainCTF, true
	default:
		return "", false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeToolCompilePreviewOptions(options ToolCompilePreviewOptions, signals ToolCompilePreviewSignals) (ToolCompilePreviewOptions, error) {
	persisted, err := toolCompilePreviewOptionsFromSignals(signals)
	if err != nil {
		return ToolCompilePreviewOptions{}, err
	}
	merged := options
	if merged.Budget.MaxPlanNodes == 0 {
		merged.Budget = persisted.Budget
	}
	merged.DomainPriorities = append(append([]compiler.DomainPriority(nil), persisted.DomainPriorities...), merged.DomainPriorities...)
	merged.FrontierScores = append(append([]compiler.CandidateFrontierScore(nil), persisted.FrontierScores...), merged.FrontierScores...)
	merged.HistoricalStats = append(append([]compiler.CandidateHistory(nil), persisted.HistoricalStats...), merged.HistoricalStats...)
	return merged, nil
}

func toolCompilePreviewOptionsFromSignals(signals ToolCompilePreviewSignals) (ToolCompilePreviewOptions, error) {
	options := ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: signals.Budget.MaxPlanNodes},
	}
	for _, priority := range signals.DomainPriorities {
		domain, err := parseToolCompilePreviewSignalDomain(priority.Domain)
		if err != nil {
			return ToolCompilePreviewOptions{}, err
		}
		options.DomainPriorities = append(options.DomainPriorities, compiler.DomainPriority{
			Domain: domain,
			Weight: priority.Weight,
		})
	}
	for _, score := range signals.FrontierScores {
		manifestName := strings.TrimSpace(score.ManifestName)
		capability := strings.TrimSpace(score.Capability)
		if manifestName == "" && capability == "" {
			continue
		}
		options.FrontierScores = append(options.FrontierScores, compiler.CandidateFrontierScore{
			ManifestName: manifestName,
			Capability:   capability,
			Path:         score.Path.toCandidatePath(),
		})
	}
	for _, history := range signals.HistoricalStats {
		manifestName := strings.TrimSpace(history.ManifestName)
		capability := strings.TrimSpace(history.Capability)
		if manifestName == "" && capability == "" {
			continue
		}
		options.HistoricalStats = append(options.HistoricalStats, compiler.CandidateHistory{
			ManifestName: manifestName,
			Capability:   capability,
			Successes:    history.Successes,
			Failures:     history.Failures,
		})
	}
	return options, nil
}

func parseToolCompilePreviewSignalDomain(value string) (frontier.Domain, error) {
	switch strings.TrimSpace(value) {
	case string(frontier.DomainWebAPI), "web":
		return frontier.DomainWebAPI, nil
	case string(frontier.DomainInfra), "infra":
		return frontier.DomainInfra, nil
	case string(frontier.DomainCTF), "ctf":
		return frontier.DomainCTF, nil
	default:
		return "", fmt.Errorf("unsupported compile preview signal domain: %s", value)
	}
}

func (path ToolCompilePreviewSignalPath) toCandidatePath() frontier.CandidatePath {
	return frontier.CandidatePath{
		ID:                 strings.TrimSpace(path.ID),
		Target:             strings.TrimSpace(path.Target),
		Hypothesis:         strings.TrimSpace(path.Hypothesis),
		ExpectedUnlock:     strings.TrimSpace(path.ExpectedUnlock),
		MissionValue:       path.MissionValue,
		UnlockPotential:    path.UnlockPotential,
		EvidenceConfidence: path.EvidenceConfidence,
		Novelty:            path.Novelty,
		Cost:               path.Cost,
		DuplicateRisk:      path.DuplicateRisk,
		Noise:              path.Noise,
		ExhaustedPenalty:   path.ExhaustedPenalty,
	}
}

func PlanApprovedToolchainInstalls(root string) (ToolchainInstallDispatchPlan, error) {
	requests, err := ListToolchainInstallRequests(root)
	if err != nil {
		return ToolchainInstallDispatchPlan{}, err
	}
	return toolchainInstallDispatchPlanFromRequests(requests), nil
}

func planToolchainInstallsForExecution(root string) (ToolchainInstallDispatchPlan, error) {
	requests, err := ListToolchainInstallRequests(root)
	if err != nil {
		return ToolchainInstallDispatchPlan{}, err
	}
	return toolchainInstallDispatchPlanFromRequestsForExecution(requests), nil
}

func doctorToolchainInstallRuntime(root string) (DoctorReport, error) {
	report, err := DoctorWorkspaceWithOptions(root, DoctorOptions{SkipContainerRuntimeCheck: true})
	if err != nil {
		return DoctorReport{}, err
	}
	blocking := make([]string, 0, len(report.Errors))
	for _, problem := range report.Errors {
		if toolchainInstallCompatibilityMetadataError(problem) {
			report.Warnings = append(report.Warnings, "toolchain install ignored compatibility metadata and is using unrestricted process defaults: "+problem)
			continue
		}
		blocking = append(blocking, problem)
	}
	report.Errors = blocking
	report.Ready = len(blocking) == 0
	return report, nil
}

func toolchainInstallCompatibilityMetadataError(problem string) bool {
	problem = strings.TrimSpace(problem)
	return problem == "required file missing: scope.yaml" ||
		problem == "required file is a directory: scope.yaml" ||
		strings.HasPrefix(problem, "invalid scope.yaml:") ||
		strings.HasPrefix(problem, "invalid authorization.yaml:")
}

func PlanFailedToolchainInstallRetries(root string) (ToolchainInstallRetryPlan, error) {
	executions, err := ListToolchainInstallDispatchExecutions(root)
	if err != nil {
		return ToolchainInstallRetryPlan{}, err
	}
	return toolchainInstallRetryPlanFromExecutions(executions), nil
}

func CreateToolchainInstallRetryDispatch(root string) (ToolchainInstallRetryDispatch, error) {
	plan, err := PlanFailedToolchainInstallRetries(root)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, err
	}
	dispatch := ToolchainInstallRetryDispatch{
		Status:  "dry-run",
		Mode:    "metadata-only",
		Summary: plan.Summary,
		Ready:   append([]ToolchainInstallRetryCandidate(nil), plan.Ready...),
		Skipped: append([]ToolchainInstallRetrySkip(nil), plan.Skipped...),
	}
	dir := toolchainInstallRetryDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("create install retry dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeInstallRetryDispatchName(plan)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("encode install retry dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("write install retry dispatch: %w", err)
	}
	return dispatch, nil
}

func ListToolchainInstallRetryDispatches(root string) ([]ToolchainInstallRetryDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallRetryDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install retry dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]ToolchainInstallRetryDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readToolchainInstallRetryDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadToolchainInstallRetryDispatch(root, dispatchPath string) (ToolchainInstallRetryDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallRetryDispatchPath(root, dispatchPath)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, err
	}
	return readToolchainInstallRetryDispatchFile(path)
}

func readToolchainInstallRetryDispatchFile(path string) (ToolchainInstallRetryDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("read install retry dispatch %s: %w", path, err)
	}
	var dispatch ToolchainInstallRetryDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("decode install retry dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func ExecuteToolchainInstallRetryDispatch(root, dispatchPath string, options ToolchainInstallRetryDispatchExecutionOptions) (ToolchainInstallRetryDispatchExecution, error) {
	if !options.Execute {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("install retry dispatch execution requires explicit execute")
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dispatch, err := ReadToolchainInstallRetryDispatch(root, dispatchPath)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	if installRequestDisplayStatus(dispatch.Status) != "dry-run" || installRequestDisplayStatus(dispatch.Mode) != "metadata-only" {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("install retry dispatch must be dry-run metadata-only before execution")
	}
	if err := revalidateRetryDispatchRequestsApproved(root, dispatch.Ready); err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	execution := ToolchainInstallRetryDispatchExecution{
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: dispatch.Path,
		Warnings:          append([]string(nil), doctor.Warnings...),
		Skipped:           append([]ToolchainInstallRetrySkip(nil), dispatch.Skipped...),
	}
	for _, candidate := range dispatch.Ready {
		result := ToolchainInstallRetryDispatchResult{
			ExecutionPath: candidate.ExecutionPath,
			DispatchPath:  candidate.DispatchPath,
			RequestPath:   candidate.RequestPath,
			Pack:          candidate.Pack,
			Dependency:    candidate.Dependency,
			Command:       append([]string(nil), candidate.Command...),
			LastStatus:    candidate.LastStatus,
			LastOutput:    candidate.LastOutput,
			LastError:     candidate.LastError,
			Status:        "completed",
		}
		output, err := runToolchainInstallRetryCommand(root, dispatch.Path, candidate)
		result.Output = strings.TrimSpace(output)
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			execution.Status = "failed"
			execution.Results = append(execution.Results, result)
			if writeErr := writeToolchainInstallRetryDispatchExecution(root, &execution); writeErr != nil {
				return execution, writeErr
			}
			return execution, fmt.Errorf("execute retry install command for %s/%s: %w", candidate.Pack, candidate.Dependency, err)
		}
		execution.Results = append(execution.Results, result)
	}
	if err := writeToolchainInstallRetryDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func revalidateRetryDispatchRequestsApproved(root string, candidates []ToolchainInstallRetryCandidate) error {
	for _, candidate := range candidates {
		request, err := readToolchainInstallRequestForRetry(root, candidate.RequestPath)
		if err != nil {
			return err
		}
		status := installRequestDisplayStatus(request.Status)
		if status != "approved" {
			return fmt.Errorf("install retry dispatch ready request must be approved: %s/%s status=%s", candidate.Pack, candidate.Dependency, status)
		}
		if strings.TrimSpace(request.Pack) != strings.TrimSpace(candidate.Pack) || strings.TrimSpace(request.Dependency) != strings.TrimSpace(candidate.Dependency) {
			return fmt.Errorf("install retry dispatch request mismatch: candidate=%s/%s request=%s/%s", candidate.Pack, candidate.Dependency, request.Pack, request.Dependency)
		}
	}
	return nil
}

func readToolchainInstallRequestForRetry(root string, requestPath string) (ToolchainInstallRequest, error) {
	path, err := resolveToolchainInstallRequestPath(root, requestPath)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("read install request %s: %w", path, err)
	}
	var request ToolchainInstallRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("decode install request %s: %w", path, err)
	}
	request.Path = path
	return request, nil
}

func runToolchainInstallRetryCommand(root, retryDispatchPath string, candidate ToolchainInstallRetryCandidate) (string, error) {
	if len(candidate.Command) == 0 {
		return "", fmt.Errorf("retry install command is empty for %s/%s", candidate.Pack, candidate.Dependency)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, candidate.Command[0], candidate.Command[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"OURO_WORKSPACE="+root,
		"OURO_INSTALL_RETRY_DISPATCH="+retryDispatchPath,
		"OURO_INSTALL_SOURCE_EXECUTION="+candidate.ExecutionPath,
		"OURO_INSTALL_DISPATCH="+candidate.DispatchPath,
		"OURO_INSTALL_REQUEST="+candidate.RequestPath,
		"OURO_INSTALL_PACK="+candidate.Pack,
		"OURO_INSTALL_DEPENDENCY="+candidate.Dependency,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("retry install command timed out")
	}
	return string(output), err
}

func writeToolchainInstallRetryDispatchExecution(root string, execution *ToolchainInstallRetryDispatchExecution) error {
	dir := toolchainInstallRetryExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install retry execution directory: %w", err)
	}
	name := safeInstallRequestName(strings.Join([]string{
		"install-retry-execution",
		execution.RetryDispatchPath,
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "|"))
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install retry execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write install retry execution: %w", err)
	}
	return nil
}

func ListToolchainInstallRetryDispatchExecutions(root string) ([]ToolchainInstallRetryDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallRetryExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install retry executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ToolchainInstallRetryDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readToolchainInstallRetryDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadToolchainInstallRetryDispatchExecution(root, executionPath string) (ToolchainInstallRetryDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallRetryExecutionPath(root, executionPath)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	return readToolchainInstallRetryDispatchExecutionFile(path)
}

func readToolchainInstallRetryDispatchExecutionFile(path string) (ToolchainInstallRetryDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("read install retry execution %s: %w", path, err)
	}
	var execution ToolchainInstallRetryDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("decode install retry execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func CreateToolchainInstallDispatch(root string) (ToolchainInstallDispatch, error) {
	plan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		return ToolchainInstallDispatch{}, err
	}
	return createToolchainInstallDispatch(root, plan)
}

func createToolchainInstallDispatch(root string, plan ToolchainInstallDispatchPlan) (ToolchainInstallDispatch, error) {
	dispatch := ToolchainInstallDispatch{
		Status:  "dry-run",
		Mode:    "metadata-only",
		Summary: plan.Summary,
		Ready:   append([]ToolchainInstallRequest(nil), plan.Ready...),
		Skipped: append([]ToolchainInstallDispatchSkip(nil), plan.Skipped...),
	}
	dir := toolchainInstallDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("create install dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeInstallDispatchName(plan)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("encode install dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("write install dispatch: %w", err)
	}
	return dispatch, nil
}

func ListToolchainInstallDispatches(root string) ([]ToolchainInstallDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]ToolchainInstallDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readToolchainInstallDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadToolchainInstallDispatch(root, dispatchPath string) (ToolchainInstallDispatch, error) {
	doctor, err := doctorToolchainInstallRuntime(root)
	if err != nil {
		return ToolchainInstallDispatch{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallDispatch{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallDispatchPath(root, dispatchPath)
	if err != nil {
		return ToolchainInstallDispatch{}, err
	}
	return readToolchainInstallDispatchFile(path)
}

func readToolchainInstallDispatchFile(path string) (ToolchainInstallDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("read install dispatch %s: %w", path, err)
	}
	var dispatch ToolchainInstallDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("decode install dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func ExecuteToolchainInstallDispatch(root, dispatchPath string, options ToolchainInstallDispatchExecutionOptions) (ToolchainInstallDispatchExecution, error) {
	doctor, err := doctorToolchainInstallRuntime(root)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dispatch, err := ReadToolchainInstallDispatch(root, dispatchPath)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	if installRequestDisplayStatus(dispatch.Status) != "dry-run" || installRequestDisplayStatus(dispatch.Mode) != "metadata-only" {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("install dispatch must be dry-run metadata-only before execution")
	}
	execution := ToolchainInstallDispatchExecution{
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: dispatch.Path,
		Warnings:     append([]string(nil), doctor.Warnings...),
		Skipped:      append([]ToolchainInstallDispatchSkip(nil), dispatch.Skipped...),
	}
	for _, request := range dispatch.Ready {
		result := ToolchainInstallDispatchExecutionResult{
			RequestPath: request.Path,
			Pack:        request.Pack,
			Dependency:  request.Dependency,
			Command:     append([]string(nil), request.Command...),
			Status:      "completed",
		}
		output, err := runToolchainInstallCommand(root, dispatch.Path, request)
		result.Output = strings.TrimSpace(output)
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			execution.Status = "failed"
			execution.Results = append(execution.Results, result)
			if writeErr := writeToolchainInstallDispatchExecution(root, &execution); writeErr != nil {
				return execution, writeErr
			}
			return execution, fmt.Errorf("execute install command for %s/%s: %w", request.Pack, request.Dependency, err)
		}
		execution.Results = append(execution.Results, result)
	}
	if err := writeToolchainInstallDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func runToolchainInstallCommand(root, dispatchPath string, request ToolchainInstallRequest) (string, error) {
	if len(request.Command) == 0 {
		return "", fmt.Errorf("install command is empty for %s/%s", request.Pack, request.Dependency)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, request.Command[0], request.Command[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"OURO_WORKSPACE="+root,
		"OURO_INSTALL_DISPATCH="+dispatchPath,
		"OURO_INSTALL_REQUEST="+request.Path,
		"OURO_INSTALL_PACK="+request.Pack,
		"OURO_INSTALL_DEPENDENCY="+request.Dependency,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("install command timed out")
	}
	return string(output), err
}

func writeToolchainInstallDispatchExecution(root string, execution *ToolchainInstallDispatchExecution) error {
	dir := toolchainInstallExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install execution directory: %w", err)
	}
	name := safeInstallRequestName(strings.Join([]string{
		"install-execution",
		execution.DispatchPath,
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "|"))
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write install execution: %w", err)
	}
	return nil
}

func ListToolchainInstallDispatchExecutions(root string) ([]ToolchainInstallDispatchExecution, error) {
	doctor, err := doctorToolchainInstallRuntime(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ToolchainInstallDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readToolchainInstallDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadToolchainInstallDispatchExecution(root, executionPath string) (ToolchainInstallDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallExecutionPath(root, executionPath)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	return readToolchainInstallDispatchExecutionFile(path)
}

func readToolchainInstallDispatchExecutionFile(path string) (ToolchainInstallDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("read install execution %s: %w", path, err)
	}
	var execution ToolchainInstallDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("decode install execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func toolchainInstallDispatchPlanFromRequests(requests []ToolchainInstallRequest) ToolchainInstallDispatchPlan {
	return toolchainInstallDispatchPlanFromRequestsWithApproval(requests, true)
}

func toolchainInstallDispatchPlanFromRequestsForExecution(requests []ToolchainInstallRequest) ToolchainInstallDispatchPlan {
	return toolchainInstallDispatchPlanFromRequestsWithApproval(requests, false)
}

func toolchainInstallDispatchPlanFromRequestsWithApproval(requests []ToolchainInstallRequest, requireApproval bool) ToolchainInstallDispatchPlan {
	plan := ToolchainInstallDispatchPlan{
		Summary: SummarizeToolchainInstallRequests(requests),
	}
	requestByDependency := map[string]ToolchainInstallRequest{}
	for _, request := range requests {
		requestByDependency[toolchainInstallRequestDependencyKey(request.Pack, request.Dependency)] = request
	}
	skipped := map[string]bool{}
	emitted := map[string]bool{}
	waiting := map[string]ToolchainInstallRequest{}
	for _, request := range requests {
		status := installRequestDisplayStatus(request.Status)
		key := toolchainInstallRequestKey(request)
		if requireApproval && status != "approved" {
			plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{
				Request: request,
				Reason:  "not-approved: " + status,
			})
			skipped[key] = true
			continue
		}
		if reason := toolchainInstallMissingOrUnapprovedDependencyReason(request, requestByDependency, requireApproval); reason != "" {
			plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{Request: request, Reason: reason})
			skipped[key] = true
			continue
		}
		waiting[key] = request
	}
	for len(waiting) > 0 {
		progress := false
		for _, request := range requests {
			key := toolchainInstallRequestKey(request)
			if _, ok := waiting[key]; !ok {
				continue
			}
			if reason := toolchainInstallBlockedDependencyReason(request, requestByDependency, skipped, emitted, requireApproval); reason != "" {
				plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{Request: request, Reason: reason})
				skipped[key] = true
				delete(waiting, key)
				progress = true
				continue
			}
			if !toolchainInstallDependenciesEmitted(request, requestByDependency, emitted) {
				continue
			}
			plan.Ready = append(plan.Ready, request)
			emitted[key] = true
			delete(waiting, key)
			progress = true
		}
		if progress {
			continue
		}
		for _, request := range requests {
			key := toolchainInstallRequestKey(request)
			if _, ok := waiting[key]; !ok {
				continue
			}
			reason := "dependency-unresolved: cycle"
			if requireApproval {
				reason = "dependency-not-approved: unresolved status=blocked"
			}
			if len(request.DependsOn) > 0 {
				if requireApproval {
					reason = fmt.Sprintf("dependency-not-approved: %s status=blocked", strings.TrimSpace(request.DependsOn[0]))
				} else {
					reason = fmt.Sprintf("dependency-unresolved: %s status=blocked", strings.TrimSpace(request.DependsOn[0]))
				}
			}
			plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{Request: request, Reason: reason})
			skipped[key] = true
			delete(waiting, key)
		}
	}
	return plan
}

func toolchainInstallMissingOrUnapprovedDependencyReason(request ToolchainInstallRequest, requestsByDependency map[string]ToolchainInstallRequest, requireApproval bool) string {
	for _, dependency := range request.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		prerequisite, ok := requestsByDependency[toolchainInstallRequestDependencyKey(request.Pack, dependency)]
		if !ok {
			return fmt.Sprintf("dependency-not-approved: %s status=missing-request", dependency)
		}
		status := installRequestDisplayStatus(prerequisite.Status)
		if requireApproval && status != "approved" {
			return fmt.Sprintf("dependency-not-approved: %s status=%s", dependency, status)
		}
	}
	return ""
}

func toolchainInstallBlockedDependencyReason(request ToolchainInstallRequest, requestsByDependency map[string]ToolchainInstallRequest, skipped map[string]bool, emitted map[string]bool, requireApproval bool) string {
	for _, dependency := range request.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		prerequisite, ok := requestsByDependency[toolchainInstallRequestDependencyKey(request.Pack, dependency)]
		if !ok {
			return fmt.Sprintf("dependency-not-approved: %s status=missing-request", dependency)
		}
		prerequisiteKey := toolchainInstallRequestKey(prerequisite)
		status := installRequestDisplayStatus(prerequisite.Status)
		switch {
		case requireApproval && status != "approved":
			return fmt.Sprintf("dependency-not-approved: %s status=%s", dependency, status)
		case skipped[prerequisiteKey]:
			return fmt.Sprintf("dependency-not-approved: %s status=blocked", dependency)
		case !emitted[prerequisiteKey]:
			continue
		}
	}
	return ""
}

func toolchainInstallDependenciesEmitted(request ToolchainInstallRequest, requestsByDependency map[string]ToolchainInstallRequest, emitted map[string]bool) bool {
	for _, dependency := range request.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		prerequisite, ok := requestsByDependency[toolchainInstallRequestDependencyKey(request.Pack, dependency)]
		if !ok {
			return false
		}
		if !emitted[toolchainInstallRequestKey(prerequisite)] {
			return false
		}
	}
	return true
}

func toolchainInstallRequestKey(request ToolchainInstallRequest) string {
	return strings.Join([]string{
		strings.TrimSpace(request.Path),
		strings.TrimSpace(request.Pack),
		strings.TrimSpace(request.Dependency),
	}, "\x00")
}

func toolchainInstallRequestDependencyKey(pack string, dependency string) string {
	return strings.TrimSpace(pack) + "\x00" + strings.TrimSpace(dependency)
}

func toolchainInstallRetryPlanFromExecutions(executions []ToolchainInstallDispatchExecution) ToolchainInstallRetryPlan {
	var plan ToolchainInstallRetryPlan
	for _, execution := range executions {
		for _, result := range execution.Results {
			status := installRequestDisplayStatus(result.Status)
			plan.Summary.Total++
			if status == "failed" {
				plan.Ready = append(plan.Ready, ToolchainInstallRetryCandidate{
					ExecutionPath: execution.Path,
					DispatchPath:  execution.DispatchPath,
					RequestPath:   result.RequestPath,
					Pack:          result.Pack,
					Dependency:    result.Dependency,
					Command:       append([]string(nil), result.Command...),
					LastStatus:    status,
					LastOutput:    result.Output,
					LastError:     result.Error,
				})
				plan.Summary.Ready++
				continue
			}
			plan.Skipped = append(plan.Skipped, ToolchainInstallRetrySkip{
				ExecutionPath: execution.Path,
				DispatchPath:  execution.DispatchPath,
				RequestPath:   result.RequestPath,
				Pack:          result.Pack,
				Dependency:    result.Dependency,
				Status:        status,
				Reason:        "not-failed: " + status,
			})
			plan.Summary.Skipped++
		}
	}
	return plan
}

func InspectToolchains(root string) ([]ToolchainStatus, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return nil, err
	}
	statuses := make([]ToolchainStatus, 0, len(manifests))
	healthRunner := toolchain.DefaultHealthRunner{}
	dependencyRunner := toolchain.DefaultDependencyRunner{}
	metadataProvider := toolchain.DefaultMetadataProvider{}
	for _, manifest := range manifests {
		dependencies := toolchain.CheckDependenciesWithMetadata(context.Background(), manifest, dependencyRunner, metadataProvider)
		installPlan := toolchain.PlanDependencyInstalls(manifest, dependencies)
		health := toolchain.CheckHealth(manifest, healthRunner)
		health = toolchain.ApplyDependencyHealth(health, dependencies)
		statuses = append(statuses, ToolchainStatus{
			Name:            manifest.Name,
			Version:         manifest.Version,
			Runner:          manifest.Runner,
			Parser:          manifest.Parser,
			CapabilityCount: len(manifest.Capabilities),
			Health:          health,
			Dependencies:    dependencies,
			InstallPlan:     installPlan,
		})
	}
	return statuses, nil
}

func CheckToolchainContracts(root string) (ToolchainContractCheck, error) {
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{SkipContainerRuntimeCheck: true})
	if err != nil {
		return ToolchainContractCheck{}, err
	}
	if !doctor.Ready {
		return ToolchainContractCheck{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolchainContractCheck{}, err
	}
	catalog := toolchain.BuiltInAppSecTools()
	packs := contracts.BuiltInDomainPacks()
	patternContracts := contracts.BuiltInPatternContracts()
	check := ToolchainContractCheck{
		Status:              "ready",
		Mode:                "read-only",
		ManifestCount:       len(manifests),
		ToolCount:           len(catalog),
		DomainPackCount:     len(packs),
		PatternCount:        len(patternContracts),
		SemanticActionCount: len(compiler.DeclaredSemanticActions()),
	}
	if err := contracts.Validate(contracts.Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: packs,
		Patterns:    patternContracts,
	}); err != nil {
		check.Status = "blocked"
		check.Errors = splitContractValidationErrors(err)
	}
	return check, nil
}

func splitContractValidationErrors(err error) []string {
	if err == nil {
		return nil
	}
	lines := strings.Split(err.Error(), "\n")
	errors := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		errors = append(errors, line)
	}
	return errors
}

func CreateAuthoredTool(root string, spec toolchain.AuthoredToolSpec) (AuthoredToolArtifact, error) {
	artifact, manifest, err := prepareAuthoredToolArtifact(root, spec, "written")
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	if err := rejectExistingAuthoredToolDir(artifact.Dir); err != nil {
		return AuthoredToolArtifact{}, err
	}

	specData, err := yaml.Marshal(spec)
	if err != nil {
		return AuthoredToolArtifact{}, fmt.Errorf("encode authored tool spec: %w", err)
	}
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		return AuthoredToolArtifact{}, fmt.Errorf("encode authored tool manifest: %w", err)
	}

	if err := os.Mkdir(artifact.Dir, 0o755); err != nil {
		return AuthoredToolArtifact{}, fmt.Errorf("create authored tool directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(artifact.Dir)
		}
	}()
	if err := requireAuthoredToolDirectory(artifact.Dir, "authored tool directory"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	if err := writeAuthoredToolArtifactFile(artifact.SpecPath, specData, "authored tool spec"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	if err := writeAuthoredToolArtifactFile(artifact.ManifestPath, manifestData, "authored tool manifest"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	published = true
	return artifact, nil
}

func ListAuthoredTools(root string) ([]AuthoredToolArtifact, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	dir, err := authoredToolchainRoot(root)
	if err != nil {
		return nil, err
	}
	if err := requireAuthoredToolDirectory(dir, "toolchain directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "authored-tool.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob authored tools: %w", err)
	}
	sort.Strings(paths)
	artifacts := make([]AuthoredToolArtifact, 0, len(paths))
	for _, path := range paths {
		artifact, err := readAuthoredToolFromDir(filepath.Dir(path), "available")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func ReadAuthoredTool(root, name string) (AuthoredToolArtifact, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return AuthoredToolArtifact{}, err
	}
	dir, err := authoredToolDir(root, name)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	return readAuthoredToolFromDir(dir, "available")
}

func prepareAuthoredToolArtifact(root string, spec toolchain.AuthoredToolSpec, status string) (AuthoredToolArtifact, toolchain.Manifest, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return AuthoredToolArtifact{}, toolchain.Manifest{}, err
	}
	manifest, err := toolchain.AuthoredToolToManifest(spec)
	if err != nil {
		return AuthoredToolArtifact{}, toolchain.Manifest{}, err
	}
	dir, err := authoredToolDir(root, manifest.Name)
	if err != nil {
		return AuthoredToolArtifact{}, toolchain.Manifest{}, err
	}
	return authoredToolArtifactFromSpec(status, dir, spec, manifest), manifest, nil
}

func readAuthoredToolFromDir(dir string, status string) (AuthoredToolArtifact, error) {
	if err := requireAuthoredToolDirectory(dir, "authored tool directory"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	specPath := filepath.Join(dir, "authored-tool.yaml")
	if err := rejectUnsafeAuthoredToolFile(specPath, "authored tool spec"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	spec, err := toolchain.LoadAuthoredToolSpec(specPath)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := rejectUnsafeAuthoredToolFile(manifestPath, "authored tool manifest"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	manifest, err := toolchain.LoadManifest(manifestPath)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	expected, err := toolchain.AuthoredToolToManifest(spec)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	actual := manifest
	actual.Path = ""
	if !reflect.DeepEqual(expected, actual) {
		return AuthoredToolArtifact{}, fmt.Errorf("authored tool manifest %s does not match authored spec", manifestPath)
	}
	return authoredToolArtifactFromSpec(status, dir, spec, manifest), nil
}

func authoredToolArtifactFromSpec(status string, dir string, spec toolchain.AuthoredToolSpec, manifest toolchain.Manifest) AuthoredToolArtifact {
	return AuthoredToolArtifact{
		Status:       status,
		Name:         manifest.Name,
		Version:      manifest.Version,
		Parser:       manifest.Parser,
		AdapterID:    strings.TrimSpace(spec.Adapter.ID),
		Operation:    strings.TrimSpace(spec.Adapter.Operation),
		Capabilities: append([]string(nil), manifest.Capabilities...),
		Dir:          dir,
		SpecPath:     filepath.Join(dir, "authored-tool.yaml"),
		ManifestPath: filepath.Join(dir, "manifest.yaml"),
	}
}

func authoredToolDir(root, name string) (string, error) {
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return "", err
	}
	dir, err := authoredToolchainRoot(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, safeName)
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve authored tool relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) || strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("authored tool path must stay directly under %s", dir)
	}
	return candidate, nil
}

func authoredToolchainRoot(root string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(root, "toolchain"))
	if err != nil {
		return "", fmt.Errorf("resolve toolchain directory: %w", err)
	}
	return dir, nil
}

func requireAuthoredToolWorkspace(root string) error {
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{SkipContainerRuntimeCheck: true})
	if err != nil {
		return err
	}
	if !doctor.Ready {
		return fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	return nil
}

func validateAuthoredToolName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "":
		return "", fmt.Errorf("authored tool name is required")
	case trimmed != name:
		return "", fmt.Errorf("authored tool name must not contain surrounding whitespace")
	case trimmed == "." || trimmed == "..":
		return "", fmt.Errorf("authored tool name is unsafe: %s", name)
	case filepath.IsAbs(trimmed):
		return "", fmt.Errorf("authored tool name must be a direct path segment: %s", name)
	case strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\"):
		return "", fmt.Errorf("authored tool name must be a direct path segment: %s", name)
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return "", fmt.Errorf("authored tool name contains unsupported character %q", r)
		}
	}
	return trimmed, nil
}

func rejectExistingAuthoredToolDir(dir string) error {
	parent := filepath.Dir(dir)
	if err := requireAuthoredToolDirectory(parent, "toolchain directory"); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect authored tool directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("authored tool directory must not be a symlink: %s", dir)
	}
	return fmt.Errorf("authored tool directory already exists: %s", dir)
}

func requireAuthoredToolDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s missing: %s", label, path)
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func rejectUnsafeAuthoredToolFile(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if info.IsDir() {
		return fmt.Errorf("write %s: path is a directory: %s", label, path)
	}
	return nil
}

func writeAuthoredToolArtifactFile(path string, data []byte, label string) error {
	if err := rejectUnsafeAuthoredToolFile(path, label); err != nil {
		return err
	}
	data = append(append([]byte(nil), data...), '\n')
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create %s temp file: %w", label, err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write %s: %w", label, err)
	}
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod %s temp file: %w", label, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close %s temp file: %w", label, err)
	}
	if err := rejectUnsafeAuthoredToolFile(path, label); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", label, err)
	}
	cleanup = false
	return nil
}

type managedToolBundleManifest struct {
	Name       string                               `yaml:"name"`
	Kind       string                               `yaml:"kind"`
	Version    string                               `yaml:"version"`
	Source     string                               `yaml:"source"`
	Payload    string                               `yaml:"payload"`
	SHA256     string                               `yaml:"sha256"`
	Provenance *managedToolBundleManifestProvenance `yaml:"provenance"`
	Corpus     *managedToolBundleManifestCorpus     `yaml:"corpus"`
	Signature  *managedToolBundleManifestSignature  `yaml:"signature"`
	Update     *managedToolBundleManifestUpdate     `yaml:"update"`
}

type managedToolBundleManifestProvenance struct {
	SourceURL       string `yaml:"source_url"`
	UpstreamVersion string `yaml:"upstream_version"`
	RetrievedAt     string `yaml:"retrieved_at"`
	VerifiedBy      string `yaml:"verified_by"`
}

type managedToolBundleManifestCorpus struct {
	Schema          string `yaml:"schema"`
	Name            string `yaml:"name"`
	Source          string `yaml:"source"`
	RetrievedAt     string `yaml:"retrieved_at"`
	FileCount       int    `yaml:"file_count"`
	SHA256          string `yaml:"sha256"`
	DigestAlgorithm string `yaml:"digest_algorithm"`
	DigestScope     string `yaml:"digest_scope"`
}

type managedToolBundleManifestUpdate struct {
	LatestVersion string `yaml:"latest_version"`
	LatestSHA256  string `yaml:"latest_sha256"`
	CheckedAt     string `yaml:"checked_at"`
	Message       string `yaml:"message"`
}

type managedToolBundleManifestSignature struct {
	Algorithm     string `yaml:"algorithm"`
	KeyID         string `yaml:"key_id"`
	PublicKey     string `yaml:"public_key"`
	PayloadSHA256 string `yaml:"payload_sha256"`
	Signature     string `yaml:"signature"`
	SignedAt      string `yaml:"signed_at"`
	SignedBy      string `yaml:"signed_by"`
}

type managedToolBundleProvenanceMetadata struct {
	Status          string
	SourceURL       string
	UpstreamVersion string
	RetrievedAt     string
	VerifiedBy      string
}

type managedToolBundleCorpusMetadata struct {
	Status          string
	Name            string
	Source          string
	RetrievedAt     string
	FileCount       int
	SHA256          string
	DigestStatus    string
	DigestAlgorithm string
	DigestScope     string
}

type managedToolBundleSignatureMetadata struct {
	Status    string
	Algorithm string
	KeyID     string
	SignedAt  string
	SignedBy  string
}

type managedToolBundleUpdateMetadata struct {
	Status        string
	LatestVersion string
	LatestSHA256  string
	CheckedAt     string
	Message       string
}

func ListManagedToolBundles(root string) ([]ManagedToolBundle, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	dir, err := managedToolBundleRoot(root)
	if err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle directory"); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return []ManagedToolBundle{}, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "bundle.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob managed tool bundles: %w", err)
	}
	sort.Strings(paths)
	bundles := make([]ManagedToolBundle, 0, len(paths))
	for _, path := range paths {
		bundle, err := readManagedToolBundle(root, filepath.Dir(path))
		if err != nil {
			bundles = append(bundles, managedToolBundleStatusFromError(filepath.Dir(path), path, err))
			continue
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func managedToolBundleStatusFromError(dir string, manifestPath string, err error) ManagedToolBundle {
	name := filepath.Base(dir)
	if strings.TrimSpace(name) == "." || strings.TrimSpace(name) == string(os.PathSeparator) {
		name = "unknown"
	}
	return ManagedToolBundle{
		Status:             "blocked",
		Name:               name,
		ProvenanceStatus:   "blocked",
		CorpusStatus:       "blocked",
		CorpusDigestStatus: "blocked",
		SignatureStatus:    "blocked",
		UpdateStatus:       "blocked",
		BundleDir:          dir,
		ManifestPath:       manifestPath,
		Error:              err.Error(),
	}
}

func ListReleaseManagedToolBundles(releaseRoot string) ([]ReleaseManagedToolBundle, error) {
	dir, err := releaseManagedToolBundleRoot(releaseRoot)
	if err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "release managed bundle root"); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return []ReleaseManagedToolBundle{}, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "bundle.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob release managed tool bundles: %w", err)
	}
	sort.Strings(paths)
	bundles := make([]ReleaseManagedToolBundle, 0, len(paths))
	for _, path := range paths {
		bundle, err := readReleaseManagedToolBundle(filepath.Dir(path))
		if err != nil {
			bundles = append(bundles, releaseManagedToolBundleStatusFromError(filepath.Dir(path), path, err))
			continue
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func ReadReleaseManagedToolBundle(releaseRoot, name string) (ReleaseManagedToolBundle, error) {
	dir, err := releaseManagedToolBundleDir(releaseRoot, name)
	if err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	return readReleaseManagedToolBundle(dir)
}

func ListReleaseManagedToolBundleStatuses(root, releaseRoot string) ([]ReleaseManagedToolBundleStatus, error) {
	return listReleaseManagedToolBundleStatusesWithExpectedSHA(root, releaseRoot, "", "")
}

func listReleaseManagedToolBundleStatusesWithExpectedSHA(root, releaseRoot, expectedBundle, expectedSHA string) ([]ReleaseManagedToolBundleStatus, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	expectedBundle = strings.TrimSpace(expectedBundle)
	if expectedBundle != "" {
		safeName, err := validateAuthoredToolName(expectedBundle)
		if err != nil {
			return nil, err
		}
		expectedBundle = safeName
	}
	expectedSHA = strings.TrimSpace(expectedSHA)
	if expectedBundle == "" && expectedSHA != "" {
		return nil, fmt.Errorf("release managed status source expected sha256 requires pin bundle")
	}
	if expectedBundle != "" {
		normalized, err := normalizeManagedToolBundleExpectedSHA256(expectedSHA, true)
		if err != nil {
			return nil, err
		}
		expectedSHA = normalized
	}
	bundles, err := ListReleaseManagedToolBundles(releaseRoot)
	if err != nil {
		return nil, err
	}
	statuses := make([]ReleaseManagedToolBundleStatus, 0, len(bundles))
	pinFound := expectedBundle == ""
	for _, bundle := range bundles {
		statusExpectedSHA := ""
		if expectedBundle != "" && bundle.Name == expectedBundle {
			statusExpectedSHA = expectedSHA
			pinFound = true
		}
		statuses = append(statuses, releaseManagedToolBundleStatusForWorkspaceWithExpectedSHA(root, bundle, statusExpectedSHA))
	}
	if !pinFound {
		return nil, fmt.Errorf("release managed status source pin bundle not found: %s", expectedBundle)
	}
	sort.SliceStable(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses, nil
}

func ReadReleaseManagedToolBundleStatus(root, releaseRoot, name string) (ReleaseManagedToolBundleStatus, error) {
	return ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, name, ReleaseManagedToolBundleStatusOptions{})
}

func ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, name string, options ReleaseManagedToolBundleStatusOptions) (ReleaseManagedToolBundleStatus, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ReleaseManagedToolBundleStatus{}, err
	}
	expectedSHA, err := normalizeManagedToolBundleExpectedSHA256(options.ExpectedSHA256, options.ExpectedSHA256Provided)
	if err != nil {
		return ReleaseManagedToolBundleStatus{}, err
	}
	dir, err := releaseManagedToolBundleDir(releaseRoot, name)
	if err != nil {
		return ReleaseManagedToolBundleStatus{}, err
	}
	bundle, err := readReleaseManagedToolBundle(dir)
	if err != nil {
		bundle = releaseManagedToolBundleStatusFromError(dir, filepath.Join(dir, "bundle.yaml"), err)
	}
	return releaseManagedToolBundleStatusForWorkspaceWithExpectedSHA(root, bundle, expectedSHA), nil
}

func releaseManagedStatusSourceExpectedPin(expectedBundle, expectedSHA string, expectedSHAProvided bool) (string, string, error) {
	expectedBundle = strings.TrimSpace(expectedBundle)
	expectedSHA = strings.TrimSpace(expectedSHA)
	hasExpectedSHA := expectedSHAProvided || expectedSHA != ""
	if expectedBundle == "" && !hasExpectedSHA {
		return "", "", nil
	}
	if expectedBundle == "" {
		return "", "", fmt.Errorf("release managed status source --expected-sha256 requires --pin-bundle")
	}
	if !hasExpectedSHA {
		return "", "", fmt.Errorf("release managed status source --pin-bundle requires --expected-sha256")
	}
	safeName, err := validateAuthoredToolName(expectedBundle)
	if err != nil {
		return "", "", err
	}
	normalizedSHA, err := normalizeManagedToolBundleExpectedSHA256(expectedSHA, true)
	if err != nil {
		return "", "", err
	}
	return safeName, normalizedSHA, nil
}

func releaseManagedStatusSourceExpectedSHAStatus(statuses []ReleaseManagedToolBundleStatus, expectedBundle string) string {
	expectedBundle = strings.TrimSpace(expectedBundle)
	if expectedBundle == "" {
		return ""
	}
	for _, status := range statuses {
		if status.Name == expectedBundle {
			return status.ExpectedSHA256Status
		}
	}
	return ""
}

func SetReleaseManagedToolBundleStatusSource(root, releaseRoot string, options ReleaseManagedToolBundleStatusSourceOptions) (ReleaseManagedToolBundleStatusSource, error) {
	if !options.Set {
		return ReleaseManagedToolBundleStatusSource{}, fmt.Errorf("release managed status source set requires --set")
	}
	expectedBundle, expectedSHA, err := releaseManagedStatusSourceExpectedPin(options.ExpectedSHA256Bundle, options.ExpectedSHA256, options.ExpectedSHA256Provided)
	if err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	statuses, err := listReleaseManagedToolBundleStatusesWithExpectedSHA(root, releaseRoot, expectedBundle, expectedSHA)
	if err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	path, err := releaseManagedToolBundleStatusSourcePath(root)
	if err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	source := releaseManagedToolBundleStatusSourceFromStatuses(path, releaseRoot, statuses)
	source.Status = "current"
	source.Mode = "metadata-only"
	source.ExpectedSHA256Bundle = expectedBundle
	source.ExpectedSHA256 = expectedSHA
	source.ExpectedSHA256Status = releaseManagedStatusSourceExpectedSHAStatus(statuses, expectedBundle)
	source.SetBy = installRequestDisplayValue(strings.TrimSpace(options.SetBy), "operator")
	source.SetAt = time.Now().UTC().Format(time.RFC3339)
	source.Reason = strings.TrimSpace(options.Reason)
	if err := writeReleaseManagedToolBundleStatusSource(source); err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	return source, nil
}

func ClearReleaseManagedToolBundleStatusSource(root string, options ReleaseManagedToolBundleStatusSourceClearOptions) (ReleaseManagedToolBundleStatusSource, error) {
	if !options.Clear {
		return ReleaseManagedToolBundleStatusSource{}, fmt.Errorf("release managed status source clear requires --clear")
	}
	path, err := releaseManagedToolBundleStatusSourcePath(root)
	if err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	source, found, err := ReadReleaseManagedToolBundleStatusSource(root)
	if err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	if !found {
		source = ReleaseManagedToolBundleStatusSource{
			Path: path,
			Mode: "metadata-only",
		}
	}
	source.Status = "cleared"
	source.Mode = "metadata-only"
	source.Path = path
	source.ClearedBy = installRequestDisplayValue(strings.TrimSpace(options.ClearedBy), "operator")
	source.ClearedAt = time.Now().UTC().Format(time.RFC3339)
	source.Reason = strings.TrimSpace(options.Reason)
	if err := writeReleaseManagedToolBundleStatusSource(source); err != nil {
		return ReleaseManagedToolBundleStatusSource{}, err
	}
	return source, nil
}

func ReadReleaseManagedToolBundleStatusSource(root string) (ReleaseManagedToolBundleStatusSource, bool, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ReleaseManagedToolBundleStatusSource{}, false, err
	}
	path, err := releaseManagedToolBundleStatusSourcePath(root)
	if err != nil {
		return ReleaseManagedToolBundleStatusSource{}, false, err
	}
	if err := rejectUnsafeAuthoredToolFile(path, "release managed status source marker"); err != nil {
		return ReleaseManagedToolBundleStatusSource{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReleaseManagedToolBundleStatusSource{}, false, nil
		}
		return ReleaseManagedToolBundleStatusSource{}, false, fmt.Errorf("read release managed status source marker %s: %w", path, err)
	}
	var source ReleaseManagedToolBundleStatusSource
	if err := json.Unmarshal(data, &source); err != nil {
		return ReleaseManagedToolBundleStatusSource{}, false, fmt.Errorf("parse release managed status source marker %s: %w", path, err)
	}
	if strings.TrimSpace(source.Path) == "" {
		source.Path = path
	}
	if strings.TrimSpace(source.Mode) == "" {
		source.Mode = "metadata-only"
	}
	return source, true, nil
}

func CreateManagedToolBundleUpdateRequest(root, releaseRoot, name string, options ManagedToolBundleUpdateRequestOptions) (ManagedToolBundleUpdateRequest, error) {
	if !options.Request {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("managed bundle update request requires --request")
	}
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	bundle, err := ReadReleaseManagedToolBundle(releaseRoot, safeName)
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	if bundle.Status != "available" {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("managed bundle update request requires available release bundle, got %s", installRequestDisplayValue(bundle.Status, "unknown"))
	}
	if strings.TrimSpace(bundle.UpdateStatus) != "update_available" {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("managed bundle update request requires update_status=update_available, got %s", installRequestDisplayValue(bundle.UpdateStatus, "unknown"))
	}
	releaseRootAbs, err := filepath.Abs(releaseRoot)
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("resolve release root: %w", err)
	}
	request := ManagedToolBundleUpdateRequest{
		Status:              "pending",
		Mode:                "metadata-only",
		Name:                bundle.Name,
		Kind:                bundle.Kind,
		Source:              bundle.Source,
		CurrentVersion:      bundle.Version,
		CurrentSHA256:       bundle.SHA256,
		LatestVersion:       bundle.UpdateLatestVersion,
		LatestSHA256:        bundle.UpdateLatestSHA256,
		UpdateStatus:        bundle.UpdateStatus,
		UpdateCheckedAt:     bundle.UpdateCheckedAt,
		UpdateMessage:       bundle.UpdateMessage,
		ReleaseRoot:         releaseRootAbs,
		ReleaseBundleDir:    bundle.ReleaseBundleDir,
		ReleaseManifestPath: bundle.ReleaseManifestPath,
		ReleasePayloadPath:  bundle.ReleasePayloadPath,
		RequestedBy:         installRequestDisplayValue(strings.TrimSpace(options.RequestedBy), "operator"),
		RequestedAt:         time.Now().UTC().Format(time.RFC3339),
		Reason:              strings.TrimSpace(options.Reason),
	}
	dir := managedToolBundleUpdateRequestDir(root)
	if err := ensureAuthoredToolDirectory(dir, "managed bundle update request directory"); err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	request.Path = filepath.Join(dir, safeManagedToolBundleUpdateRequestName(request)+".json")
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("encode managed bundle update request: %w", err)
	}
	if err := writeAuthoredToolArtifactFile(request.Path, data, "managed bundle update request"); err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	return request, nil
}

func ListManagedToolBundleUpdateRequests(root string) ([]ManagedToolBundleUpdateRequest, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(managedToolBundleUpdateRequestDir(root), "managed bundle update request directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(managedToolBundleUpdateRequestDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob managed bundle update requests: %w", err)
	}
	sort.Strings(paths)
	requests := make([]ManagedToolBundleUpdateRequest, 0, len(paths))
	for _, path := range paths {
		request, err := readManagedToolBundleUpdateRequestPath(path)
		if err != nil {
			return nil, err
		}
		request.Path = path
		requests = append(requests, request)
	}
	return requests, nil
}

func ReadManagedToolBundleUpdateRequest(root, requestPath string) (ManagedToolBundleUpdateRequest, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	path, err := resolveManagedToolBundleUpdateRequestPath(root, requestPath)
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	request, err := readManagedToolBundleUpdateRequestPath(path)
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	request.Path = path
	return request, nil
}

func SummarizeManagedToolBundleUpdateRequests(requests []ManagedToolBundleUpdateRequest) ManagedToolBundleUpdateRequestSummary {
	summary := ManagedToolBundleUpdateRequestSummary{Total: len(requests)}
	for _, request := range requests {
		if strings.TrimSpace(request.Status) == "pending" {
			summary.Pending++
		}
	}
	return summary
}

func (summary ManagedToolBundleUpdateRequestSummary) Line() string {
	return fmt.Sprintf("total=%d pending=%d", summary.Total, summary.Pending)
}

func readManagedToolBundleUpdateRequestPath(path string) (ManagedToolBundleUpdateRequest, error) {
	if err := rejectUnsafeAuthoredToolFile(path, "managed bundle update request"); err != nil {
		return ManagedToolBundleUpdateRequest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("read managed bundle update request %s: %w", path, err)
	}
	var request ManagedToolBundleUpdateRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return ManagedToolBundleUpdateRequest{}, fmt.Errorf("decode managed bundle update request %s: %w", path, err)
	}
	if strings.TrimSpace(request.Mode) == "" {
		request.Mode = "metadata-only"
	}
	return request, nil
}

func CreateManagedToolBundleUpdateDownloadPlan(root, requestPath string, options ManagedToolBundleUpdateDownloadPlanOptions) (ManagedToolBundleUpdateDownloadPlan, error) {
	if !options.Plan {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("managed bundle update download plan requires --plan")
	}
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	request, err := ReadManagedToolBundleUpdateRequest(root, requestPath)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	if strings.TrimSpace(request.Status) != "pending" {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("managed bundle update download plan requires pending request, got %s", installRequestDisplayValue(request.Status, "unknown"))
	}
	if strings.TrimSpace(request.Mode) != "" && strings.TrimSpace(request.Mode) != "metadata-only" {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("managed bundle update download plan requires metadata-only request, got %s", request.Mode)
	}
	if strings.TrimSpace(request.UpdateStatus) != "update_available" {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("managed bundle update download plan requires update_status=update_available, got %s", installRequestDisplayValue(request.UpdateStatus, "unknown"))
	}
	sourceURL, err := normalizeManagedToolBundleUpdateDownloadSourceURL(options.SourceURL)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	expectedSHA, err := normalizeManagedToolBundleUpdateDownloadExpectedSHA256(options.ExpectedSHA256)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	latestSHA := strings.ToLower(strings.TrimSpace(request.LatestSHA256))
	if latestSHA != "" && expectedSHA != latestSHA {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("managed bundle update download plan expected sha256 mismatch: expected %s latest %s", expectedSHA, latestSHA)
	}
	archiveFormat, archiveExpectedSHA, archiveExpectedStatus, err := normalizeManagedToolBundleUpdateArchivePlanMetadata(request.Kind, options.ArchiveFormat, options.ArchiveExpectedSHA256)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	plan := ManagedToolBundleUpdateDownloadPlan{
		Status:                      "pending",
		Mode:                        "metadata-only",
		Name:                        request.Name,
		Kind:                        request.Kind,
		Source:                      request.Source,
		CurrentVersion:              request.CurrentVersion,
		CurrentSHA256:               request.CurrentSHA256,
		LatestVersion:               request.LatestVersion,
		LatestSHA256:                request.LatestSHA256,
		UpdateStatus:                request.UpdateStatus,
		UpdateCheckedAt:             request.UpdateCheckedAt,
		UpdateMessage:               request.UpdateMessage,
		ExpectedSHA256:              expectedSHA,
		ExpectedSHA256Status:        releaseManagedExpectedSHA256Status(expectedSHA, latestSHA),
		ArchiveFormat:               archiveFormat,
		ArchiveExpectedSHA256:       archiveExpectedSHA,
		ArchiveExpectedSHA256Status: archiveExpectedStatus,
		SourceURL:                   sourceURL,
		RequestPath:                 request.Path,
		ReleaseRoot:                 request.ReleaseRoot,
		ReleaseBundleDir:            request.ReleaseBundleDir,
		ReleaseManifestPath:         request.ReleaseManifestPath,
		ReleasePayloadPath:          request.ReleasePayloadPath,
		PlannedBy:                   installRequestDisplayValue(strings.TrimSpace(options.PlannedBy), "operator"),
		PlannedAt:                   time.Now().UTC().Format(time.RFC3339),
		Reason:                      strings.TrimSpace(options.Reason),
	}
	dir := managedToolBundleUpdateDownloadPlanDir(root)
	if err := ensureAuthoredToolDirectory(dir, "managed bundle update download plan directory"); err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	plan.Path = filepath.Join(dir, safeManagedToolBundleUpdateDownloadPlanName(plan)+".json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("encode managed bundle update download plan: %w", err)
	}
	if err := writeAuthoredToolArtifactFile(plan.Path, data, "managed bundle update download plan"); err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	return plan, nil
}

func ListManagedToolBundleUpdateDownloadPlans(root string) ([]ManagedToolBundleUpdateDownloadPlan, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(managedToolBundleUpdateDownloadPlanDir(root), "managed bundle update download plan directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(managedToolBundleUpdateDownloadPlanDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob managed bundle update download plans: %w", err)
	}
	sort.Strings(paths)
	plans := make([]ManagedToolBundleUpdateDownloadPlan, 0, len(paths))
	for _, path := range paths {
		plan, err := readManagedToolBundleUpdateDownloadPlanPath(path)
		if err != nil {
			return nil, err
		}
		plan.Path = path
		plans = append(plans, plan)
	}
	return plans, nil
}

func ReadManagedToolBundleUpdateDownloadPlan(root, planPath string) (ManagedToolBundleUpdateDownloadPlan, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	path, err := resolveManagedToolBundleUpdateDownloadPlanPath(root, planPath)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	plan, err := readManagedToolBundleUpdateDownloadPlanPath(path)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	plan.Path = path
	return plan, nil
}

func SummarizeManagedToolBundleUpdateDownloadPlans(plans []ManagedToolBundleUpdateDownloadPlan) ManagedToolBundleUpdateDownloadPlanSummary {
	summary := ManagedToolBundleUpdateDownloadPlanSummary{Total: len(plans)}
	for _, plan := range plans {
		if strings.TrimSpace(plan.Status) == "pending" {
			summary.Pending++
		}
	}
	return summary
}

func (summary ManagedToolBundleUpdateDownloadPlanSummary) Line() string {
	return fmt.Sprintf("total=%d pending=%d", summary.Total, summary.Pending)
}

func readManagedToolBundleUpdateDownloadPlanPath(path string) (ManagedToolBundleUpdateDownloadPlan, error) {
	if err := rejectUnsafeAuthoredToolFile(path, "managed bundle update download plan"); err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("read managed bundle update download plan %s: %w", path, err)
	}
	var plan ManagedToolBundleUpdateDownloadPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return ManagedToolBundleUpdateDownloadPlan{}, fmt.Errorf("decode managed bundle update download plan %s: %w", path, err)
	}
	if strings.TrimSpace(plan.Mode) == "" {
		plan.Mode = "metadata-only"
	}
	return plan, nil
}

func normalizeManagedToolBundleUpdateDownloadSourceURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update download plan source url is required")
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return "", fmt.Errorf("managed bundle update download plan source url must not contain whitespace")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("managed bundle update download plan source url is invalid: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("managed bundle update download plan source url must use https")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", fmt.Errorf("managed bundle update download plan source url requires host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("managed bundle update download plan source url must not include userinfo")
	}
	if strings.TrimSpace(parsed.RawQuery) != "" {
		return "", fmt.Errorf("managed bundle update download plan source url must not include query")
	}
	if strings.TrimSpace(parsed.Fragment) != "" {
		return "", fmt.Errorf("managed bundle update download plan source url must not include fragment")
	}
	return parsed.String(), nil
}

func normalizeManagedToolBundleUpdateDownloadExpectedSHA256(value string) (string, error) {
	normalized, err := normalizeManagedToolBundleExpectedSHA256(value, true)
	if err != nil {
		return "", fmt.Errorf("managed bundle update download plan expected sha256 is invalid")
	}
	return normalized, nil
}

func normalizeManagedToolBundleUpdateArchivePlanMetadata(kind, archiveFormat, archiveExpectedSHA string) (string, string, string, error) {
	format := strings.ToLower(strings.TrimSpace(archiveFormat))
	expected := strings.TrimSpace(archiveExpectedSHA)
	if format == "" && expected == "" {
		return "", "", "none", nil
	}
	switch strings.TrimSpace(kind) {
	case "binary", "rule_bundle":
	default:
		return "", "", "", fmt.Errorf("managed bundle update archive metadata is supported only for binary or rule_bundle updates")
	}
	if format == "" {
		return "", "", "", fmt.Errorf("managed bundle update archive format is required")
	}
	if format != "tar.gz" && format != "zip" {
		return "", "", "", fmt.Errorf("managed bundle update archive format is unsupported: %s", format)
	}
	normalizedExpected, err := normalizeManagedToolBundleExpectedSHA256(expected, true)
	if err != nil {
		return "", "", "", fmt.Errorf("managed bundle update archive expected sha256 is invalid")
	}
	return format, normalizedExpected, "matched", nil
}

const managedToolBundleUpdateDownloadMaxBytes int64 = 512 * 1024 * 1024

func ExecuteManagedToolBundleUpdateDownloadPlan(root, planPath string, options ManagedToolBundleUpdateDownloadExecutionOptions) (ManagedToolBundleUpdateDownloadExecution, error) {
	if !options.Execute {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("managed bundle update download execution requires explicit execute")
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	if !doctor.Ready {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	plan, err := ReadManagedToolBundleUpdateDownloadPlan(root, planPath)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	if strings.TrimSpace(plan.Status) != "pending" {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("managed bundle update download execution requires pending plan, got %s", installRequestDisplayValue(plan.Status, "unknown"))
	}
	if strings.TrimSpace(plan.Mode) != "" && strings.TrimSpace(plan.Mode) != "metadata-only" {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("managed bundle update download execution requires metadata-only plan, got %s", plan.Mode)
	}
	sourceURL, err := normalizeManagedToolBundleUpdateDownloadSourceURL(plan.SourceURL)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	expectedSHA, err := normalizeManagedToolBundleUpdateDownloadExpectedSHA256(plan.ExpectedSHA256)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	archiveFormat, archiveExpectedSHA, _, err := normalizeManagedToolBundleUpdateArchivePlanMetadata(plan.Kind, plan.ArchiveFormat, plan.ArchiveExpectedSHA256)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	if strings.TrimSpace(plan.Kind) == "rule_bundle" && archiveFormat == "" {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("managed bundle update download execution for rule_bundle requires archive metadata")
	}
	latestSHA := strings.ToLower(strings.TrimSpace(plan.LatestSHA256))
	if latestSHA != "" && expectedSHA != latestSHA {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("managed bundle update download execution expected sha256 mismatch: expected %s latest %s", expectedSHA, latestSHA)
	}
	request, err := ReadManagedToolBundleUpdateRequest(root, plan.RequestPath)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	if err := validateManagedToolBundleUpdateDownloadPlanRequestStillMatches(plan, request, expectedSHA); err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}

	execution := ManagedToolBundleUpdateDownloadExecution{
		Status:                 "failed",
		Mode:                   "execute",
		Action:                 "download",
		Name:                   plan.Name,
		Kind:                   plan.Kind,
		Source:                 plan.Source,
		CurrentVersion:         plan.CurrentVersion,
		CurrentSHA256:          plan.CurrentSHA256,
		LatestVersion:          plan.LatestVersion,
		LatestSHA256:           plan.LatestSHA256,
		UpdateStatus:           plan.UpdateStatus,
		UpdateCheckedAt:        plan.UpdateCheckedAt,
		UpdateMessage:          plan.UpdateMessage,
		SourceURL:              sourceURL,
		ExpectedSHA256:         expectedSHA,
		DigestStatus:           "unknown",
		DigestAlgorithm:        "sha256",
		DigestScope:            managedBundleDigestScope(plan.Kind),
		ArchiveFormat:          archiveFormat,
		ArchiveExpectedSHA256:  archiveExpectedSHA,
		ArchiveDigestStatus:    archiveDigestStatusForDownloadExecution(archiveFormat),
		ArchiveDigestAlgorithm: archiveDigestAlgorithmForDownloadExecution(archiveFormat),
		ArchiveDigestScope:     archiveDigestScopeForDownloadExecution(archiveFormat),
		PlanPath:               plan.Path,
		RequestPath:            request.Path,
		ReleaseRoot:            plan.ReleaseRoot,
		ReleaseBundleDir:       plan.ReleaseBundleDir,
		ReleaseManifestPath:    plan.ReleaseManifestPath,
		ReleasePayloadPath:     plan.ReleasePayloadPath,
		Actor:                  installRequestDisplayValue(strings.TrimSpace(options.Actor), "operator"),
		ExecutedAt:             time.Now().UTC().Format(time.RFC3339),
		Reason:                 strings.TrimSpace(options.Reason),
	}

	client := http.DefaultClient
	if options.HTTPClient != nil {
		client = options.HTTPClient
	}
	requestContext, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodGet, sourceURL, nil)
	if err != nil {
		return execution, err
	}
	httpRequest.Header.Set("Accept-Encoding", "identity")
	response, err := managedToolBundleUpdateDownloadHTTPClient(client).Do(httpRequest)
	if err != nil {
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("download managed bundle update payload: %w", err))
	}
	defer response.Body.Close()
	execution.HTTPStatus = response.StatusCode
	if response.ContentLength >= 0 {
		execution.ContentLength = response.ContentLength
		if response.ContentLength > managedToolBundleUpdateDownloadMaxBytes {
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download payload is too large: content_length=%d max=%d", response.ContentLength, managedToolBundleUpdateDownloadMaxBytes))
		}
	}
	if response.StatusCode != http.StatusOK {
		if managedToolBundleUpdateDownloadHTTPStatusIsRedirect(response.StatusCode) {
			execution.RedirectPolicy = "rejected"
			execution.RedirectLocation, execution.RedirectLocationStatus = sanitizeManagedToolBundleUpdateRedirectLocation(response.Header.Get("Location"))
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download redirect rejected: HTTP %d", response.StatusCode))
		}
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download expected HTTP 200, got %d", response.StatusCode))
	}
	if archiveFormat != "" {
		archivePath, tmpPath, archiveActualSHA, bytesWritten, err := streamManagedToolBundleUpdateDownloadArchive(root, execution, response.Body)
		execution.ArchiveActualSHA256 = archiveActualSHA
		execution.BytesWritten = bytesWritten
		if err != nil {
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, err)
		}
		if archiveActualSHA != archiveExpectedSHA {
			execution.ArchiveDigestStatus = "mismatch"
			if tmpPath != "" {
				_ = os.Remove(tmpPath)
			}
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download archive sha256 mismatch: expected %s actual %s", archiveExpectedSHA, archiveActualSHA))
		}
		if _, err := os.Lstat(archivePath); err == nil {
			_ = os.Remove(tmpPath)
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download archive already exists: %s", archivePath))
		} else if err != nil && !os.IsNotExist(err) {
			_ = os.Remove(tmpPath)
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("inspect managed bundle update download archive: %w", err))
		}
		if err := os.Rename(tmpPath, archivePath); err != nil {
			_ = os.Remove(tmpPath)
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("publish managed bundle update download archive: %w", err))
		}
		execution.ArchivePath = archivePath
		execution.ArchiveDigestStatus = "matched"
		var payloadPath string
		var actualSHA string
		switch strings.TrimSpace(plan.Kind) {
		case "binary":
			payloadPath, actualSHA, err = extractManagedToolBundleUpdateBinaryArchive(root, execution, archivePath, expectedSHA)
		case "rule_bundle":
			payloadPath, actualSHA, err = extractManagedToolBundleUpdateRuleBundleArchive(root, execution, archivePath, expectedSHA)
		default:
			err = fmt.Errorf("managed bundle update archive kind is unsupported: %s", installRequestDisplayValue(plan.Kind, "unknown"))
		}
		execution.ActualSHA256 = actualSHA
		if err != nil {
			if actualSHA != "" && actualSHA != expectedSHA {
				execution.DigestStatus = "mismatch"
			}
			return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, err)
		}
		execution.Status = "completed"
		execution.DigestStatus = "matched"
		execution.PayloadPath = payloadPath
		if err := writeManagedToolBundleUpdateDownloadExecution(root, &execution); err != nil {
			return execution, err
		}
		return execution, nil
	}
	payloadPath, tmpPath, actualSHA, bytesWritten, err := streamManagedToolBundleUpdateDownloadPayload(root, execution, response.Body)
	execution.ActualSHA256 = actualSHA
	execution.BytesWritten = bytesWritten
	if err != nil {
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, err)
	}
	if actualSHA != expectedSHA {
		execution.DigestStatus = "mismatch"
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download sha256 mismatch: expected %s actual %s", expectedSHA, actualSHA))
	}
	if _, err := os.Lstat(payloadPath); err == nil {
		_ = os.Remove(tmpPath)
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("managed bundle update download payload already exists: %s", payloadPath))
	} else if err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("inspect managed bundle update download payload: %w", err))
	}
	if err := os.Rename(tmpPath, payloadPath); err != nil {
		_ = os.Remove(tmpPath)
		return writeManagedToolBundleUpdateDownloadExecutionError(root, &execution, fmt.Errorf("publish managed bundle update download payload: %w", err))
	}
	execution.Status = "completed"
	execution.DigestStatus = "matched"
	execution.PayloadPath = payloadPath
	if err := writeManagedToolBundleUpdateDownloadExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func validateManagedToolBundleUpdateDownloadPlanRequestStillMatches(plan ManagedToolBundleUpdateDownloadPlan, request ManagedToolBundleUpdateRequest, expectedSHA string) error {
	if strings.TrimSpace(request.Status) != "pending" {
		return fmt.Errorf("managed bundle update download execution requires pending request, got %s", installRequestDisplayValue(request.Status, "unknown"))
	}
	if strings.TrimSpace(request.Mode) != "" && strings.TrimSpace(request.Mode) != "metadata-only" {
		return fmt.Errorf("managed bundle update download execution requires metadata-only request, got %s", request.Mode)
	}
	if strings.TrimSpace(request.UpdateStatus) != "update_available" {
		return fmt.Errorf("managed bundle update download execution requires update_status=update_available, got %s", installRequestDisplayValue(request.UpdateStatus, "unknown"))
	}
	if request.Name != plan.Name || request.Kind != plan.Kind || request.Source != plan.Source || request.CurrentVersion != plan.CurrentVersion || request.CurrentSHA256 != plan.CurrentSHA256 || request.LatestVersion != plan.LatestVersion || request.LatestSHA256 != plan.LatestSHA256 || request.UpdateCheckedAt != plan.UpdateCheckedAt || request.UpdateMessage != plan.UpdateMessage {
		return fmt.Errorf("managed bundle update download execution plan no longer matches request metadata")
	}
	if request.ReleaseRoot != plan.ReleaseRoot || request.ReleaseBundleDir != plan.ReleaseBundleDir || request.ReleaseManifestPath != plan.ReleaseManifestPath || request.ReleasePayloadPath != plan.ReleasePayloadPath {
		return fmt.Errorf("managed bundle update download execution plan no longer matches request release references")
	}
	latestSHA := strings.ToLower(strings.TrimSpace(request.LatestSHA256))
	if latestSHA != "" && expectedSHA != latestSHA {
		return fmt.Errorf("managed bundle update download execution expected sha256 mismatch: expected %s latest %s", expectedSHA, latestSHA)
	}
	return nil
}

func managedToolBundleUpdateDownloadHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copyClient
}

func managedToolBundleUpdateDownloadHTTPStatusIsRedirect(status int) bool {
	return status >= http.StatusMultipleChoices && status < http.StatusBadRequest
}

func sanitizeManagedToolBundleUpdateRedirectLocation(location string) (string, string) {
	trimmed := strings.TrimSpace(location)
	if trimmed == "" {
		return "", "missing"
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return "", "invalid"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "invalid"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "invalid"
	}
	sanitized := *parsed
	redacted := parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != ""
	sanitized.User = nil
	sanitized.RawQuery = ""
	sanitized.ForceQuery = false
	sanitized.Fragment = ""
	return sanitized.String(), map[bool]string{true: "redacted", false: "recorded"}[redacted]
}

func archiveDigestStatusForDownloadExecution(format string) string {
	if strings.TrimSpace(format) == "" {
		return "none"
	}
	return "unknown"
}

func archiveDigestAlgorithmForDownloadExecution(format string) string {
	if strings.TrimSpace(format) == "" {
		return "none"
	}
	return "sha256"
}

func archiveDigestScopeForDownloadExecution(format string) string {
	if strings.TrimSpace(format) == "" {
		return "none"
	}
	return "file"
}

func streamManagedToolBundleUpdateDownloadPayload(root string, execution ManagedToolBundleUpdateDownloadExecution, body io.Reader) (string, string, string, int64, error) {
	return streamManagedToolBundleUpdateDownloadFile(root, execution, body, "payload")
}

func streamManagedToolBundleUpdateDownloadArchive(root string, execution ManagedToolBundleUpdateDownloadExecution, body io.Reader) (string, string, string, int64, error) {
	return streamManagedToolBundleUpdateDownloadFile(root, execution, body, "archive")
}

func streamManagedToolBundleUpdateDownloadFile(root string, execution ManagedToolBundleUpdateDownloadExecution, body io.Reader, finalName string) (string, string, string, int64, error) {
	payloadDir := filepath.Join(managedToolBundleUpdateDownloadStagingDir(root), safeManagedToolBundleUpdateDownloadExecutionName(execution))
	if err := ensureAuthoredToolDirectory(payloadDir, "managed bundle update download staging directory"); err != nil {
		return "", "", "", 0, err
	}
	payloadPath := filepath.Join(payloadDir, finalName)
	if _, err := os.Lstat(payloadPath); err == nil {
		return "", "", "", 0, fmt.Errorf("managed bundle update download %s already exists: %s", finalName, payloadPath)
	} else if err != nil && !os.IsNotExist(err) {
		return "", "", "", 0, fmt.Errorf("inspect managed bundle update download %s: %w", finalName, err)
	}
	tmpFile, err := os.CreateTemp(payloadDir, finalName+"-*.tmp")
	if err != nil {
		return "", "", "", 0, fmt.Errorf("create managed bundle update download %s temp file: %w", finalName, err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	hash := sha256.New()
	limited := &io.LimitedReader{R: body, N: managedToolBundleUpdateDownloadMaxBytes + 1}
	bytesWritten, copyErr := io.Copy(io.MultiWriter(tmpFile, hash), limited)
	if copyErr != nil {
		_ = tmpFile.Close()
		return "", "", "", bytesWritten, fmt.Errorf("write managed bundle update download %s: %w", finalName, copyErr)
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if bytesWritten > managedToolBundleUpdateDownloadMaxBytes {
		_ = tmpFile.Close()
		return "", "", actualSHA, bytesWritten, fmt.Errorf("managed bundle update download payload is too large: bytes=%d max=%d", bytesWritten, managedToolBundleUpdateDownloadMaxBytes)
	}
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return "", "", "", bytesWritten, fmt.Errorf("chmod managed bundle update download %s temp file: %w", finalName, err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", "", "", bytesWritten, fmt.Errorf("close managed bundle update download %s temp file: %w", finalName, err)
	}
	cleanup = false
	return payloadPath, tmpPath, actualSHA, bytesWritten, nil
}

func extractManagedToolBundleUpdateRuleBundleArchive(root string, execution ManagedToolBundleUpdateDownloadExecution, archivePath, expectedSHA string) (string, string, error) {
	payloadDir := filepath.Join(managedToolBundleUpdateDownloadStagingDir(root), safeManagedToolBundleUpdateDownloadExecutionName(execution))
	if err := ensureAuthoredToolDirectory(payloadDir, "managed bundle update download staging directory"); err != nil {
		return "", "", err
	}
	payloadPath := filepath.Join(payloadDir, "payload")
	if _, err := os.Lstat(payloadPath); err == nil {
		return "", "", fmt.Errorf("managed bundle update download payload already exists: %s", payloadPath)
	} else if err != nil && !os.IsNotExist(err) {
		return "", "", fmt.Errorf("inspect managed bundle update download payload: %w", err)
	}
	extractDir, err := os.MkdirTemp(payloadDir, "payload-extract-*.tmp")
	if err != nil {
		return "", "", fmt.Errorf("create managed bundle update archive extract directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(extractDir)
		}
	}()
	if err := extractManagedToolBundleUpdateArchiveByFormat(execution.ArchiveFormat, archivePath, extractDir); err != nil {
		return "", "", err
	}
	actualSHA, err := digestManagedBundleDirectory(extractDir)
	if err != nil {
		return "", "", err
	}
	if actualSHA != expectedSHA {
		return "", actualSHA, fmt.Errorf("managed bundle update archive extracted rule_bundle sha256 mismatch: expected %s actual %s", expectedSHA, actualSHA)
	}
	if err := os.Rename(extractDir, payloadPath); err != nil {
		return "", actualSHA, fmt.Errorf("publish managed bundle update archive extracted payload: %w", err)
	}
	cleanup = false
	return payloadPath, actualSHA, nil
}

func extractManagedToolBundleUpdateBinaryArchive(root string, execution ManagedToolBundleUpdateDownloadExecution, archivePath, expectedSHA string) (string, string, error) {
	payloadDir := filepath.Join(managedToolBundleUpdateDownloadStagingDir(root), safeManagedToolBundleUpdateDownloadExecutionName(execution))
	if err := ensureAuthoredToolDirectory(payloadDir, "managed bundle update download staging directory"); err != nil {
		return "", "", err
	}
	payloadPath := filepath.Join(payloadDir, "payload")
	if _, err := os.Lstat(payloadPath); err == nil {
		return "", "", fmt.Errorf("managed bundle update download payload already exists: %s", payloadPath)
	} else if err != nil && !os.IsNotExist(err) {
		return "", "", fmt.Errorf("inspect managed bundle update download payload: %w", err)
	}
	extractDir, err := os.MkdirTemp(payloadDir, "payload-extract-*.tmp")
	if err != nil {
		return "", "", fmt.Errorf("create managed bundle update archive extract directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(extractDir)
	}()
	if err := extractManagedToolBundleUpdateArchiveByFormat(execution.ArchiveFormat, archivePath, extractDir); err != nil {
		return "", "", err
	}
	sourcePath, sourceInfo, err := singleManagedToolBundleUpdateBinaryArchivePayload(extractDir)
	if err != nil {
		return "", "", err
	}
	actualSHA, err := digestManagedToolBundleImportFile(sourcePath, sourceInfo)
	if err != nil {
		return "", "", err
	}
	if actualSHA != expectedSHA {
		return "", actualSHA, fmt.Errorf("managed bundle update archive extracted binary sha256 mismatch: expected %s actual %s", expectedSHA, actualSHA)
	}
	if err := os.Rename(sourcePath, payloadPath); err != nil {
		return "", actualSHA, fmt.Errorf("publish managed bundle update archive extracted payload: %w", err)
	}
	if err := os.Chmod(payloadPath, 0o644); err != nil {
		return "", actualSHA, fmt.Errorf("chmod managed bundle update archive extracted payload: %w", err)
	}
	return payloadPath, actualSHA, nil
}

func singleManagedToolBundleUpdateBinaryArchivePayload(root string) (string, os.FileInfo, error) {
	var filePath string
	var fileInfo os.FileInfo
	regularFiles := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("inspect managed bundle update archive entry: %w", walkErr)
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect managed bundle update archive entry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle update archive entry must not be a symlink: %s", path)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed bundle update archive entry type is unsupported: %s", path)
		}
		regularFiles++
		if regularFiles > 1 {
			return fmt.Errorf("managed bundle update binary archive must contain exactly one regular file")
		}
		filePath = path
		fileInfo = info
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if regularFiles != 1 {
		return "", nil, fmt.Errorf("managed bundle update binary archive must contain exactly one regular file")
	}
	return filePath, fileInfo, nil
}

func extractManagedToolBundleUpdateArchiveByFormat(format, archivePath, dst string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "tar.gz":
		return extractManagedToolBundleUpdateTarGzipArchive(archivePath, dst)
	case "zip":
		return extractManagedToolBundleUpdateZipArchive(archivePath, dst)
	default:
		return fmt.Errorf("managed bundle update archive format is unsupported: %s", format)
	}
}

func extractManagedToolBundleUpdateTarGzipArchive(archivePath, dst string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open managed bundle update archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open managed bundle update gzip archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var extractedBytes int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read managed bundle update archive entry: %w", err)
		}
		cleanName, err := cleanManagedToolBundleUpdateArchiveEntryName(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(cleanName))
		if err := ensurePathWithinDirectory(dst, target, "managed bundle update archive entry"); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create managed bundle update archive directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("managed bundle update archive file has negative size: %s", cleanName)
			}
			extractedBytes += header.Size
			if extractedBytes > managedToolBundleUpdateDownloadMaxBytes {
				return fmt.Errorf("managed bundle update archive extracted payload is too large: bytes=%d max=%d", extractedBytes, managedToolBundleUpdateDownloadMaxBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create managed bundle update archive file directory: %w", err)
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return fmt.Errorf("create managed bundle update archive file: %w", err)
			}
			_, copyErr := io.Copy(out, tarReader)
			closeErr := out.Close()
			if copyErr != nil {
				return fmt.Errorf("copy managed bundle update archive file: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close managed bundle update archive file: %w", closeErr)
			}
			if err := os.Chmod(target, 0o644); err != nil {
				return fmt.Errorf("chmod managed bundle update archive file: %w", err)
			}
		case tar.TypeSymlink:
			return fmt.Errorf("managed bundle update archive entry must not be a symlink: %s", header.Name)
		case tar.TypeLink:
			return fmt.Errorf("managed bundle update archive entry must not be a hardlink: %s", header.Name)
		default:
			return fmt.Errorf("managed bundle update archive entry type is unsupported: %s", header.Name)
		}
	}
	return nil
}

func extractManagedToolBundleUpdateZipArchive(archivePath, dst string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open managed bundle update zip archive: %w", err)
	}
	defer reader.Close()
	var extractedBytes int64
	for _, file := range reader.File {
		cleanName, err := cleanManagedToolBundleUpdateArchiveEntryName(file.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(cleanName))
		if err := ensurePathWithinDirectory(dst, target, "managed bundle update archive entry"); err != nil {
			return err
		}
		info := file.FileInfo()
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle update archive entry must not be a symlink: %s", file.Name)
		}
		if info.IsDir() || strings.HasSuffix(file.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create managed bundle update archive directory: %w", err)
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("managed bundle update archive entry type is unsupported: %s", file.Name)
		}
		if file.UncompressedSize64 > uint64(managedToolBundleUpdateDownloadMaxBytes) {
			return fmt.Errorf("managed bundle update archive extracted payload is too large: bytes=%d max=%d", file.UncompressedSize64, managedToolBundleUpdateDownloadMaxBytes)
		}
		if extractedBytes > managedToolBundleUpdateDownloadMaxBytes-int64(file.UncompressedSize64) {
			return fmt.Errorf("managed bundle update archive extracted payload is too large: bytes=%d max=%d", extractedBytes+int64(file.UncompressedSize64), managedToolBundleUpdateDownloadMaxBytes)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create managed bundle update archive file directory: %w", err)
		}
		in, err := file.Open()
		if err != nil {
			return fmt.Errorf("open managed bundle update archive file: %w", err)
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			_ = in.Close()
			return fmt.Errorf("create managed bundle update archive file: %w", err)
		}
		remaining := managedToolBundleUpdateDownloadMaxBytes - extractedBytes
		limited := &io.LimitedReader{R: in, N: remaining + 1}
		copied, copyErr := io.Copy(out, limited)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return fmt.Errorf("copy managed bundle update archive file: %w", copyErr)
		}
		if closeOutErr != nil {
			return fmt.Errorf("close managed bundle update archive file: %w", closeOutErr)
		}
		if closeInErr != nil {
			return fmt.Errorf("close managed bundle update archive file input: %w", closeInErr)
		}
		if copied > remaining {
			return fmt.Errorf("managed bundle update archive extracted payload is too large: bytes=%d max=%d", extractedBytes+copied, managedToolBundleUpdateDownloadMaxBytes)
		}
		extractedBytes += copied
		if err := os.Chmod(target, 0o644); err != nil {
			return fmt.Errorf("chmod managed bundle update archive file: %w", err)
		}
	}
	return nil
}

func cleanManagedToolBundleUpdateArchiveEntryName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update archive entry name is required")
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("managed bundle update archive path traversal is not allowed: %s", name)
	}
	if path.IsAbs(trimmed) || filepath.IsAbs(trimmed) || hasWindowsDriveArchivePathPrefix(trimmed) {
		return "", fmt.Errorf("managed bundle update archive path traversal is not allowed: %s", name)
	}
	if hasParentDirectoryArchivePathSegment(trimmed) {
		return "", fmt.Errorf("managed bundle update archive path traversal is not allowed: %s", name)
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("managed bundle update archive path traversal is not allowed: %s", name)
	}
	return cleaned, nil
}

func hasParentDirectoryArchivePathSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func hasWindowsDriveArchivePathPrefix(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	first := value[0]
	return first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z'
}

func ensurePathWithinDirectory(root, candidate, context string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve %s root: %w", context, err)
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return fmt.Errorf("resolve %s path: %w", context, err)
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return fmt.Errorf("resolve %s relative path: %w", context, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%s must stay inside %s: %s", context, rootAbs, candidateAbs)
	}
	return nil
}

func writeManagedToolBundleUpdateDownloadExecutionError(root string, execution *ManagedToolBundleUpdateDownloadExecution, err error) (ManagedToolBundleUpdateDownloadExecution, error) {
	execution.Status = "failed"
	execution.Error = err.Error()
	if strings.TrimSpace(execution.DigestStatus) == "" {
		execution.DigestStatus = "unknown"
	}
	if writeErr := writeManagedToolBundleUpdateDownloadExecution(root, execution); writeErr != nil {
		return *execution, writeErr
	}
	return *execution, err
}

func writeManagedToolBundleUpdateDownloadExecution(root string, execution *ManagedToolBundleUpdateDownloadExecution) error {
	dir := managedToolBundleUpdateDownloadExecutionDir(root)
	if err := ensureAuthoredToolDirectory(dir, "managed bundle update download execution directory"); err != nil {
		return err
	}
	name := safeManagedToolBundleUpdateDownloadExecutionName(*execution)
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode managed bundle update download execution: %w", err)
	}
	if err := writeAuthoredToolArtifactFile(execution.Path, data, "managed bundle update download execution"); err != nil {
		return err
	}
	return nil
}

func ListManagedToolBundleUpdateDownloadExecutions(root string) ([]ManagedToolBundleUpdateDownloadExecution, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(managedToolBundleUpdateDownloadExecutionDir(root), "managed bundle update download execution directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(managedToolBundleUpdateDownloadExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob managed bundle update download executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ManagedToolBundleUpdateDownloadExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readManagedToolBundleUpdateDownloadExecutionPath(path)
		if err != nil {
			return nil, err
		}
		execution.Path = path
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadManagedToolBundleUpdateDownloadExecution(root, executionPath string) (ManagedToolBundleUpdateDownloadExecution, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	path, err := resolveManagedToolBundleUpdateDownloadExecutionPath(root, executionPath)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	execution, err := readManagedToolBundleUpdateDownloadExecutionPath(path)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	execution.Path = path
	return execution, nil
}

func SummarizeManagedToolBundleUpdateDownloadExecutions(executions []ManagedToolBundleUpdateDownloadExecution) ManagedToolBundleUpdateDownloadExecutionSummary {
	summary := ManagedToolBundleUpdateDownloadExecutionSummary{Total: len(executions)}
	for _, execution := range executions {
		switch strings.TrimSpace(execution.Status) {
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		}
	}
	return summary
}

func (summary ManagedToolBundleUpdateDownloadExecutionSummary) Line() string {
	return fmt.Sprintf("total=%d completed=%d failed=%d", summary.Total, summary.Completed, summary.Failed)
}

func readManagedToolBundleUpdateDownloadExecutionPath(path string) (ManagedToolBundleUpdateDownloadExecution, error) {
	if err := rejectUnsafeAuthoredToolFile(path, "managed bundle update download execution"); err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("read managed bundle update download execution %s: %w", path, err)
	}
	var execution ManagedToolBundleUpdateDownloadExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ManagedToolBundleUpdateDownloadExecution{}, fmt.Errorf("decode managed bundle update download execution %s: %w", path, err)
	}
	return execution, nil
}

func ApplyManagedToolBundleUpdateDownloadExecution(root, executionPath string, options ManagedToolBundleUpdateApplyOptions) (ManagedToolBundleUpdateApplyExecution, error) {
	if !options.Apply {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply requires --apply")
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if !doctor.Ready {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	download, err := ReadManagedToolBundleUpdateDownloadExecution(root, executionPath)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if strings.TrimSpace(download.Status) != "completed" || strings.TrimSpace(download.DigestStatus) != "matched" {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply requires completed download execution with matched digest, got status=%s digest_status=%s", installRequestDisplayValue(download.Status, "unknown"), installRequestDisplayValue(download.DigestStatus, "unknown"))
	}
	if strings.TrimSpace(download.Mode) != "execute" || strings.TrimSpace(download.Action) != "download" {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply requires download execution audit, got mode=%s action=%s", installRequestDisplayValue(download.Mode, "unknown"), installRequestDisplayValue(download.Action, "unknown"))
	}
	kind := strings.TrimSpace(download.Kind)
	switch kind {
	case "binary", "rule_bundle":
	default:
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply kind is unsupported: %s", installRequestDisplayValue(download.Kind, "unknown"))
	}
	expectedSHA, err := normalizeManagedToolBundleUpdateDownloadExpectedSHA256(download.ExpectedSHA256)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if actual := strings.ToLower(strings.TrimSpace(download.ActualSHA256)); actual != expectedSHA {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply download audit sha256 mismatch: expected %s actual %s", expectedSHA, actual)
	}
	stagedPayloadPath, err := resolveManagedToolBundleUpdateDownloadPayloadPath(root, download.PayloadPath)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	sourcePayload, sourceInfo, err := inspectManagedToolBundleImportPayload(stagedPayloadPath, kind)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	actualSHA, err := digestManagedToolBundleImportPayload(sourcePayload, sourceInfo)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if actualSHA != expectedSHA {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply sha256 mismatch: expected %s actual %s", expectedSHA, actualSHA)
	}
	plan, err := ReadManagedToolBundleUpdateDownloadPlan(root, download.PlanPath)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	request, err := ReadManagedToolBundleUpdateRequest(root, download.RequestPath)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if err := validateManagedToolBundleUpdateDownloadPlanRequestStillMatches(plan, request, expectedSHA); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if err := validateManagedToolBundleUpdateDownloadExecutionStillMatchesPlan(download, plan, request, expectedSHA); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if strings.TrimSpace(download.LatestVersion) == "" {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply latest version is required")
	}
	if strings.TrimSpace(download.Source) == "" {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply source is required")
	}

	rootDir, err := managedToolBundleRoot(root)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	if err := ensureAuthoredToolDirectory(rootDir, "managed bundle directory"); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	name, err := validateAuthoredToolName(download.Name)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	bundleDir := filepath.Join(rootDir, name)
	if err := rejectUnsafeAuthoredToolDirectory(bundleDir, "workspace managed bundle directory"); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	workspacePayloadPath := filepath.Join(bundleDir, "payload", name)
	if kind == "rule_bundle" {
		workspacePayloadPath = filepath.Join(bundleDir, "payload")
	}
	workspaceManifestPath := filepath.Join(bundleDir, "bundle.yaml")
	installPath, err := managedToolBundleInstallPath(root, managedToolBundleManifest{Name: name, Kind: kind})
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}

	tmpParent, tmpBundleDir, err := buildManagedToolBundleUpdateApplyBundle(root, rootDir, name, kind, download.LatestVersion, download.Source, expectedSHA, sourcePayload, sourceInfo)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.RemoveAll(tmpParent)
		}
	}()
	if err := replaceManagedToolBundleDirectory(bundleDir, tmpBundleDir); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	cleanupTmp = false
	_ = os.RemoveAll(tmpParent)
	bundle, err := ReadManagedToolBundle(root, name)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}

	apply := ManagedToolBundleUpdateApplyExecution{
		Status:                "completed",
		Mode:                  "apply",
		Action:                "workspace-copy",
		Name:                  bundle.Name,
		Kind:                  bundle.Kind,
		Source:                bundle.Source,
		CurrentVersion:        download.CurrentVersion,
		CurrentSHA256:         download.CurrentSHA256,
		LatestVersion:         download.LatestVersion,
		LatestSHA256:          download.LatestSHA256,
		UpdateStatus:          download.UpdateStatus,
		UpdateCheckedAt:       download.UpdateCheckedAt,
		UpdateMessage:         download.UpdateMessage,
		SourceURL:             download.SourceURL,
		ExpectedSHA256:        expectedSHA,
		ActualSHA256:          actualSHA,
		DigestStatus:          "matched",
		DigestAlgorithm:       "sha256",
		DigestScope:           managedBundleDigestScope(kind),
		DownloadExecutionPath: download.Path,
		PlanPath:              plan.Path,
		RequestPath:           request.Path,
		ReleaseRoot:           download.ReleaseRoot,
		ReleaseBundleDir:      download.ReleaseBundleDir,
		ReleaseManifestPath:   download.ReleaseManifestPath,
		ReleasePayloadPath:    download.ReleasePayloadPath,
		StagedPayloadPath:     stagedPayloadPath,
		WorkspaceBundleDir:    bundle.BundleDir,
		WorkspaceManifestPath: bundle.ManifestPath,
		WorkspacePayloadPath:  bundle.PayloadPath,
		InstallPath:           installPath,
		Actor:                 installRequestDisplayValue(strings.TrimSpace(options.Actor), "operator"),
		AppliedAt:             time.Now().UTC().Format(time.RFC3339),
		Reason:                strings.TrimSpace(options.Reason),
	}
	if apply.WorkspaceBundleDir != bundleDir || apply.WorkspaceManifestPath != workspaceManifestPath || apply.WorkspacePayloadPath != workspacePayloadPath || bundle.InstallPath != installPath {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("managed bundle update apply workspace paths changed during readback")
	}
	if err := writeManagedToolBundleUpdateApplyExecution(root, &apply); err != nil {
		return apply, err
	}
	return apply, nil
}

func validateManagedToolBundleUpdateDownloadExecutionStillMatchesPlan(download ManagedToolBundleUpdateDownloadExecution, plan ManagedToolBundleUpdateDownloadPlan, request ManagedToolBundleUpdateRequest, expectedSHA string) error {
	if download.Name != plan.Name || download.Kind != plan.Kind || download.Source != plan.Source || download.CurrentVersion != plan.CurrentVersion || download.CurrentSHA256 != plan.CurrentSHA256 || download.LatestVersion != plan.LatestVersion || download.LatestSHA256 != plan.LatestSHA256 || download.UpdateCheckedAt != plan.UpdateCheckedAt || download.UpdateMessage != plan.UpdateMessage {
		return fmt.Errorf("managed bundle update apply download execution no longer matches plan metadata")
	}
	if download.PlanPath != plan.Path || download.RequestPath != request.Path {
		return fmt.Errorf("managed bundle update apply download execution no longer matches plan/request references")
	}
	if download.ReleaseRoot != plan.ReleaseRoot || download.ReleaseBundleDir != plan.ReleaseBundleDir || download.ReleaseManifestPath != plan.ReleaseManifestPath || download.ReleasePayloadPath != plan.ReleasePayloadPath {
		return fmt.Errorf("managed bundle update apply download execution no longer matches release references")
	}
	sourceURL, err := normalizeManagedToolBundleUpdateDownloadSourceURL(download.SourceURL)
	if err != nil {
		return err
	}
	if sourceURL != plan.SourceURL {
		return fmt.Errorf("managed bundle update apply download execution source url no longer matches plan")
	}
	if strings.ToLower(strings.TrimSpace(download.ExpectedSHA256)) != expectedSHA || strings.ToLower(strings.TrimSpace(plan.ExpectedSHA256)) != expectedSHA {
		return fmt.Errorf("managed bundle update apply expected sha256 no longer matches plan")
	}
	if strings.TrimSpace(plan.ArchiveFormat) != "" || strings.TrimSpace(plan.ArchiveExpectedSHA256) != "" {
		archiveFormat, archiveExpectedSHA, _, err := normalizeManagedToolBundleUpdateArchivePlanMetadata(plan.Kind, plan.ArchiveFormat, plan.ArchiveExpectedSHA256)
		if err != nil {
			return err
		}
		if strings.TrimSpace(download.ArchiveFormat) != archiveFormat {
			return fmt.Errorf("managed bundle update apply archive format no longer matches plan")
		}
		if strings.ToLower(strings.TrimSpace(download.ArchiveExpectedSHA256)) != archiveExpectedSHA || strings.ToLower(strings.TrimSpace(download.ArchiveActualSHA256)) != archiveExpectedSHA {
			return fmt.Errorf("managed bundle update apply archive sha256 no longer matches plan")
		}
		if strings.TrimSpace(download.ArchiveDigestStatus) != "matched" {
			return fmt.Errorf("managed bundle update apply archive digest status is not matched")
		}
		if strings.TrimSpace(download.ArchiveDigestAlgorithm) != "sha256" || strings.TrimSpace(download.ArchiveDigestScope) != "file" {
			return fmt.Errorf("managed bundle update apply archive digest metadata no longer matches plan")
		}
	}
	latestSHA := strings.ToLower(strings.TrimSpace(download.LatestSHA256))
	if latestSHA != "" && latestSHA != expectedSHA {
		return fmt.Errorf("managed bundle update apply expected sha256 mismatch: expected %s latest %s", expectedSHA, latestSHA)
	}
	return nil
}

func buildManagedToolBundleUpdateApplyBundle(root, rootDir, name, kind, version, source, sha, sourcePayload string, sourceInfo os.FileInfo) (string, string, error) {
	tmpParent, err := os.MkdirTemp(rootDir, "."+name+"-update-apply-*.tmp")
	if err != nil {
		return "", "", fmt.Errorf("create managed bundle update apply temp directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpParent)
		}
	}()
	tmpBundleDir := filepath.Join(tmpParent, name)
	payloadPath := filepath.Join(tmpBundleDir, "payload", name)
	if kind == "rule_bundle" {
		payloadPath = filepath.Join(tmpBundleDir, "payload")
	}
	if sourceInfo.IsDir() {
		if err := copyManagedBundleDirectoryWithFileMode(sourcePayload, payloadPath, 0o644); err != nil {
			return "", "", err
		}
	} else {
		if err := copyManagedBundleFile(sourcePayload, payloadPath, 0o644, sourceInfo); err != nil {
			return "", "", err
		}
	}
	manifestPath := filepath.Join(tmpBundleDir, "bundle.yaml")
	manifestData := managedToolBundleImportManifest(name, kind, version, source, sha)
	if err := writeAuthoredToolArtifactFile(manifestPath, manifestData, "managed bundle manifest"); err != nil {
		return "", "", err
	}
	if _, err := readManagedToolBundle(root, tmpBundleDir); err != nil {
		return "", "", fmt.Errorf("validate managed bundle update apply bundle: %w", err)
	}
	cleanup = false
	return tmpParent, tmpBundleDir, nil
}

func replaceManagedToolBundleDirectory(targetDir, stagedDir string) error {
	parent := filepath.Dir(targetDir)
	if err := requireAuthoredToolDirectory(parent, "managed bundle directory"); err != nil {
		return err
	}
	if err := rejectUnsafeAuthoredToolDirectory(targetDir, "workspace managed bundle directory"); err != nil {
		return err
	}
	backupDir := ""
	if _, err := os.Lstat(targetDir); err == nil {
		backupDir, err = os.MkdirTemp(parent, "."+filepath.Base(targetDir)+"-update-backup-*.tmp")
		if err != nil {
			return fmt.Errorf("create managed bundle update backup directory: %w", err)
		}
		if err := os.Remove(backupDir); err != nil {
			return fmt.Errorf("prepare managed bundle update backup directory: %w", err)
		}
		if err := os.Rename(targetDir, backupDir); err != nil {
			return fmt.Errorf("backup workspace managed bundle: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect workspace managed bundle: %w", err)
	}
	if err := os.Rename(stagedDir, targetDir); err != nil {
		if backupDir != "" {
			_ = os.Rename(backupDir, targetDir)
		}
		return fmt.Errorf("publish managed bundle update apply bundle: %w", err)
	}
	if backupDir != "" {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func ListManagedToolBundleUpdateApplyExecutions(root string) ([]ManagedToolBundleUpdateApplyExecution, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(managedToolBundleUpdateApplyExecutionDir(root), "managed bundle update apply execution directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(managedToolBundleUpdateApplyExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob managed bundle update apply executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ManagedToolBundleUpdateApplyExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readManagedToolBundleUpdateApplyExecutionPath(path)
		if err != nil {
			return nil, err
		}
		execution.Path = path
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadManagedToolBundleUpdateApplyExecution(root, executionPath string) (ManagedToolBundleUpdateApplyExecution, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	path, err := resolveManagedToolBundleUpdateApplyExecutionPath(root, executionPath)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	execution, err := readManagedToolBundleUpdateApplyExecutionPath(path)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	execution.Path = path
	return execution, nil
}

func SummarizeManagedToolBundleUpdateApplyExecutions(executions []ManagedToolBundleUpdateApplyExecution) ManagedToolBundleUpdateApplyExecutionSummary {
	summary := ManagedToolBundleUpdateApplyExecutionSummary{Total: len(executions)}
	for _, execution := range executions {
		switch strings.TrimSpace(execution.Status) {
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		}
	}
	return summary
}

func (summary ManagedToolBundleUpdateApplyExecutionSummary) Line() string {
	return fmt.Sprintf("total=%d completed=%d failed=%d", summary.Total, summary.Completed, summary.Failed)
}

func readManagedToolBundleUpdateApplyExecutionPath(path string) (ManagedToolBundleUpdateApplyExecution, error) {
	if err := rejectUnsafeAuthoredToolFile(path, "managed bundle update apply execution"); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("read managed bundle update apply execution %s: %w", path, err)
	}
	var execution ManagedToolBundleUpdateApplyExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ManagedToolBundleUpdateApplyExecution{}, fmt.Errorf("decode managed bundle update apply execution %s: %w", path, err)
	}
	return execution, nil
}

func writeManagedToolBundleUpdateApplyExecution(root string, execution *ManagedToolBundleUpdateApplyExecution) error {
	dir := managedToolBundleUpdateApplyExecutionDir(root)
	if err := ensureAuthoredToolDirectory(dir, "managed bundle update apply execution directory"); err != nil {
		return err
	}
	name := safeManagedToolBundleUpdateApplyExecutionName(*execution)
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode managed bundle update apply execution: %w", err)
	}
	if err := writeAuthoredToolArtifactFile(execution.Path, data, "managed bundle update apply execution"); err != nil {
		return err
	}
	return nil
}

func ImportManagedToolBundle(root string, options ManagedToolBundleImportOptions) (ManagedToolBundleImportResult, error) {
	if !options.Import {
		return ManagedToolBundleImportResult{}, fmt.Errorf("managed tool bundle import requires --import")
	}
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	name, err := validateAuthoredToolName(options.Name)
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	kind := strings.TrimSpace(options.Kind)
	if kind == "" {
		kind = "binary"
	}
	switch kind {
	case "binary", "rule_bundle":
	default:
		return ManagedToolBundleImportResult{}, fmt.Errorf("managed bundle import kind is unsupported: %s", kind)
	}
	version := strings.TrimSpace(options.Version)
	if version == "" {
		return ManagedToolBundleImportResult{}, fmt.Errorf("managed bundle import version is required")
	}
	source := strings.TrimSpace(options.Source)
	if source == "" {
		return ManagedToolBundleImportResult{}, fmt.Errorf("managed bundle import source is required")
	}
	if strings.ContainsAny(source, "\r\n") {
		return ManagedToolBundleImportResult{}, fmt.Errorf("managed bundle import source must be a single line")
	}
	sourcePayload, sourceInfo, err := inspectManagedToolBundleImportPayload(options.PayloadPath, kind)
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	sha, err := digestManagedToolBundleImportPayload(sourcePayload, sourceInfo)
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	expectedSHA, err := normalizeManagedToolBundleExpectedSHA256(options.ExpectedSHA256, options.ExpectedSHA256Provided)
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	if expectedSHA != "" && expectedSHA != sha {
		return ManagedToolBundleImportResult{}, fmt.Errorf("managed bundle import expected sha256 mismatch: expected %s actual %s", expectedSHA, sha)
	}
	rootDir, err := managedToolBundleRoot(root)
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	if err := ensureAuthoredToolDirectory(rootDir, "managed bundle directory"); err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	bundleDir := filepath.Join(rootDir, name)
	if err := rejectUnsafeAuthoredToolDirectory(bundleDir, "workspace managed bundle directory"); err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	if _, err := os.Lstat(bundleDir); err == nil {
		return ManagedToolBundleImportResult{}, fmt.Errorf("workspace managed bundle already exists: %s", bundleDir)
	} else if !os.IsNotExist(err) {
		return ManagedToolBundleImportResult{}, fmt.Errorf("inspect workspace managed bundle: %w", err)
	}
	payloadPath := filepath.Join(bundleDir, "payload", name)
	if kind == "rule_bundle" {
		payloadPath = filepath.Join(bundleDir, "payload")
	}
	manifestPath := filepath.Join(bundleDir, "bundle.yaml")
	installPath, err := managedToolBundleInstallPath(root, managedToolBundleManifest{Name: name, Kind: kind})
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(bundleDir)
		}
	}()
	if sourceInfo.IsDir() {
		if err := copyManagedBundleDirectoryWithFileMode(sourcePayload, payloadPath, 0o644); err != nil {
			return ManagedToolBundleImportResult{}, err
		}
	} else {
		if err := copyManagedBundleFile(sourcePayload, payloadPath, sourceInfo.Mode().Perm(), sourceInfo); err != nil {
			return ManagedToolBundleImportResult{}, err
		}
	}
	manifestData := managedToolBundleImportManifest(name, kind, version, source, sha)
	if err := writeAuthoredToolArtifactFile(manifestPath, manifestData, "managed bundle manifest"); err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	bundle, err := ReadManagedToolBundle(root, name)
	if err != nil {
		return ManagedToolBundleImportResult{}, err
	}
	cleanup = false
	return ManagedToolBundleImportResult{
		Status:            "imported",
		Mode:              "workspace-copy",
		Action:            "copy",
		Name:              bundle.Name,
		Kind:              bundle.Kind,
		Version:           bundle.Version,
		Source:            bundle.Source,
		SHA256:            bundle.SHA256,
		ExpectedSHA256:    expectedSHA,
		SourcePayloadPath: sourcePayload,
		BundleDir:         bundle.BundleDir,
		ManifestPath:      bundle.ManifestPath,
		PayloadPath:       bundle.PayloadPath,
		InstallPath:       installPath,
	}, nil
}

func CreateManagedToolBundleInstallPlan(root, pack, dependency string) (ManagedToolBundleInstallPlan, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleInstallPlan{}, err
	}
	declared, err := findManagedInstallDependency(root, pack, dependency)
	if err != nil {
		return ManagedToolBundleInstallPlan{}, err
	}
	bundle, err := ReadManagedToolBundle(root, declared.Name)
	if err != nil {
		return ManagedToolBundleInstallPlan{}, err
	}
	if bundle.Kind != dependencyKindForManagedInstall(declared) {
		return ManagedToolBundleInstallPlan{}, fmt.Errorf("managed bundle kind %s does not match dependency kind %s", bundle.Kind, dependencyKindForManagedInstall(declared))
	}
	return ManagedToolBundleInstallPlan{
		Status:                    "ready",
		Mode:                      "metadata-only",
		Action:                    "link",
		Pack:                      strings.TrimSpace(pack),
		Dependency:                strings.TrimSpace(dependency),
		Kind:                      bundle.Kind,
		Version:                   bundle.Version,
		Source:                    bundle.Source,
		SHA256:                    bundle.SHA256,
		ProvenanceStatus:          bundle.ProvenanceStatus,
		ProvenanceSourceURL:       bundle.ProvenanceSourceURL,
		ProvenanceUpstreamVersion: bundle.ProvenanceUpstreamVersion,
		ProvenanceRetrievedAt:     bundle.ProvenanceRetrievedAt,
		ProvenanceVerifiedBy:      bundle.ProvenanceVerifiedBy,
		CorpusStatus:              bundle.CorpusStatus,
		CorpusName:                bundle.CorpusName,
		CorpusSource:              bundle.CorpusSource,
		CorpusRetrievedAt:         bundle.CorpusRetrievedAt,
		CorpusFileCount:           bundle.CorpusFileCount,
		CorpusSHA256:              bundle.CorpusSHA256,
		CorpusDigestStatus:        bundle.CorpusDigestStatus,
		CorpusDigestAlgorithm:     bundle.CorpusDigestAlgorithm,
		CorpusDigestScope:         bundle.CorpusDigestScope,
		SignatureStatus:           bundle.SignatureStatus,
		SignatureAlgorithm:        bundle.SignatureAlgorithm,
		SignatureKeyID:            bundle.SignatureKeyID,
		SignatureSignedAt:         bundle.SignatureSignedAt,
		SignatureSignedBy:         bundle.SignatureSignedBy,
		UpdateStatus:              bundle.UpdateStatus,
		UpdateLatestVersion:       bundle.UpdateLatestVersion,
		UpdateLatestSHA256:        bundle.UpdateLatestSHA256,
		UpdateCheckedAt:           bundle.UpdateCheckedAt,
		UpdateMessage:             bundle.UpdateMessage,
		PayloadPath:               bundle.PayloadPath,
		InstallPath:               bundle.InstallPath,
	}, nil
}

func ExecuteManagedToolBundleInstall(root, pack, dependency string, options ManagedToolBundleInstallOptions) (ManagedToolBundleInstallResult, error) {
	if !options.Execute {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("managed tool bundle install requires --execute")
	}
	plan, err := CreateManagedToolBundleInstallPlan(root, pack, dependency)
	if err != nil {
		return ManagedToolBundleInstallResult{}, err
	}
	installDir := filepath.Dir(plan.InstallPath)
	if err := ensureAuthoredToolDirectory(installDir, "managed install directory"); err != nil {
		return ManagedToolBundleInstallResult{}, err
	}
	if err := rejectUnsafeAuthoredToolFile(plan.InstallPath, "managed install link"); err != nil {
		return ManagedToolBundleInstallResult{}, err
	}
	if _, err := os.Lstat(plan.InstallPath); err == nil {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("managed install path already exists: %s", plan.InstallPath)
	} else if !os.IsNotExist(err) {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("inspect managed install path: %w", err)
	}
	if err := os.Symlink(plan.PayloadPath, plan.InstallPath); err != nil {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("link managed tool bundle payload: %w", err)
	}
	return ManagedToolBundleInstallResult{
		Status:                    "installed",
		Mode:                      "workspace-link",
		Action:                    plan.Action,
		Pack:                      plan.Pack,
		Dependency:                plan.Dependency,
		Kind:                      plan.Kind,
		Version:                   plan.Version,
		Source:                    plan.Source,
		SHA256:                    plan.SHA256,
		ProvenanceStatus:          plan.ProvenanceStatus,
		ProvenanceSourceURL:       plan.ProvenanceSourceURL,
		ProvenanceUpstreamVersion: plan.ProvenanceUpstreamVersion,
		ProvenanceRetrievedAt:     plan.ProvenanceRetrievedAt,
		ProvenanceVerifiedBy:      plan.ProvenanceVerifiedBy,
		CorpusStatus:              plan.CorpusStatus,
		CorpusName:                plan.CorpusName,
		CorpusSource:              plan.CorpusSource,
		CorpusRetrievedAt:         plan.CorpusRetrievedAt,
		CorpusFileCount:           plan.CorpusFileCount,
		CorpusSHA256:              plan.CorpusSHA256,
		CorpusDigestStatus:        plan.CorpusDigestStatus,
		CorpusDigestAlgorithm:     plan.CorpusDigestAlgorithm,
		CorpusDigestScope:         plan.CorpusDigestScope,
		SignatureStatus:           plan.SignatureStatus,
		SignatureAlgorithm:        plan.SignatureAlgorithm,
		SignatureKeyID:            plan.SignatureKeyID,
		SignatureSignedAt:         plan.SignatureSignedAt,
		SignatureSignedBy:         plan.SignatureSignedBy,
		UpdateStatus:              plan.UpdateStatus,
		UpdateLatestVersion:       plan.UpdateLatestVersion,
		UpdateLatestSHA256:        plan.UpdateLatestSHA256,
		UpdateCheckedAt:           plan.UpdateCheckedAt,
		UpdateMessage:             plan.UpdateMessage,
		PayloadPath:               plan.PayloadPath,
		InstallPath:               plan.InstallPath,
	}, nil
}

func CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, pack, dependency string) (ReleaseManagedToolBundleInstallPlan, error) {
	return CreateReleaseManagedToolBundleInstallPlanWithOptions(root, releaseRoot, pack, dependency, ReleaseManagedToolBundleInstallPlanOptions{})
}

func CreateReleaseManagedToolBundleInstallPlanWithOptions(root, releaseRoot, pack, dependency string, options ReleaseManagedToolBundleInstallPlanOptions) (ReleaseManagedToolBundleInstallPlan, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	expectedSHA, err := normalizeManagedToolBundleExpectedSHA256(options.ExpectedSHA256, options.ExpectedSHA256Provided)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	declared, err := findManagedInstallDependency(root, pack, dependency)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	releaseBundleDir, err := releaseManagedToolBundleDir(releaseRoot, declared.Name)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	bundle, err := readManagedToolBundle(root, releaseBundleDir)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	if bundle.Kind != dependencyKindForManagedInstall(declared) {
		return ReleaseManagedToolBundleInstallPlan{}, fmt.Errorf("release managed bundle kind %s does not match dependency kind %s", bundle.Kind, dependencyKindForManagedInstall(declared))
	}
	if expectedSHA != "" && expectedSHA != bundle.SHA256 {
		return ReleaseManagedToolBundleInstallPlan{}, fmt.Errorf("release managed bundle expected sha256 mismatch: expected %s actual %s", expectedSHA, bundle.SHA256)
	}
	workspaceBundleDir := filepath.Join(root, "toolchain", "managed-bundles", bundle.Name)
	workspacePayloadPath, err := releaseWorkspacePayloadPath(bundle.BundleDir, bundle.PayloadPath, workspaceBundleDir)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	return ReleaseManagedToolBundleInstallPlan{
		Status:                    "ready",
		Mode:                      "metadata-only",
		Action:                    "copy-and-link",
		Pack:                      strings.TrimSpace(pack),
		Dependency:                strings.TrimSpace(dependency),
		Kind:                      bundle.Kind,
		Version:                   bundle.Version,
		Source:                    bundle.Source,
		SHA256:                    bundle.SHA256,
		ExpectedSHA256:            expectedSHA,
		ExpectedSHA256Status:      releaseManagedExpectedSHA256Status(expectedSHA, bundle.SHA256),
		ProvenanceStatus:          bundle.ProvenanceStatus,
		ProvenanceSourceURL:       bundle.ProvenanceSourceURL,
		ProvenanceUpstreamVersion: bundle.ProvenanceUpstreamVersion,
		ProvenanceRetrievedAt:     bundle.ProvenanceRetrievedAt,
		ProvenanceVerifiedBy:      bundle.ProvenanceVerifiedBy,
		CorpusStatus:              bundle.CorpusStatus,
		CorpusName:                bundle.CorpusName,
		CorpusSource:              bundle.CorpusSource,
		CorpusRetrievedAt:         bundle.CorpusRetrievedAt,
		CorpusFileCount:           bundle.CorpusFileCount,
		CorpusSHA256:              bundle.CorpusSHA256,
		CorpusDigestStatus:        bundle.CorpusDigestStatus,
		CorpusDigestAlgorithm:     bundle.CorpusDigestAlgorithm,
		CorpusDigestScope:         bundle.CorpusDigestScope,
		SignatureStatus:           bundle.SignatureStatus,
		SignatureAlgorithm:        bundle.SignatureAlgorithm,
		SignatureKeyID:            bundle.SignatureKeyID,
		SignatureSignedAt:         bundle.SignatureSignedAt,
		SignatureSignedBy:         bundle.SignatureSignedBy,
		UpdateStatus:              bundle.UpdateStatus,
		UpdateLatestVersion:       bundle.UpdateLatestVersion,
		UpdateLatestSHA256:        bundle.UpdateLatestSHA256,
		UpdateCheckedAt:           bundle.UpdateCheckedAt,
		UpdateMessage:             bundle.UpdateMessage,
		ReleaseBundleDir:          bundle.BundleDir,
		ReleasePayloadPath:        bundle.PayloadPath,
		WorkspaceBundleDir:        workspaceBundleDir,
		WorkspacePayloadPath:      workspacePayloadPath,
		InstallPath:               bundle.InstallPath,
	}, nil
}

func ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, pack, dependency string, options ReleaseManagedToolBundleInstallOptions) (ReleaseManagedToolBundleInstallResult, error) {
	if !options.Execute {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("release managed tool bundle install requires --execute")
	}
	plan, err := CreateReleaseManagedToolBundleInstallPlanWithOptions(root, releaseRoot, pack, dependency, ReleaseManagedToolBundleInstallPlanOptions{
		ExpectedSHA256:         options.ExpectedSHA256,
		ExpectedSHA256Provided: options.ExpectedSHA256Provided,
	})
	if err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	installDir := filepath.Dir(plan.InstallPath)
	if err := ensureAuthoredToolDirectory(installDir, "managed install directory"); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if err := rejectUnsafeAuthoredToolFile(plan.InstallPath, "managed install link"); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if _, err := os.Lstat(plan.InstallPath); err == nil {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("managed install path already exists: %s", plan.InstallPath)
	} else if !os.IsNotExist(err) {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("inspect managed install path: %w", err)
	}
	if err := ensureAuthoredToolDirectory(filepath.Dir(plan.WorkspaceBundleDir), "managed bundle directory"); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(plan.WorkspaceBundleDir, "workspace managed bundle directory"); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if _, err := os.Lstat(plan.WorkspaceBundleDir); err == nil {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("workspace managed bundle already exists: %s", plan.WorkspaceBundleDir)
	} else if !os.IsNotExist(err) {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("inspect workspace managed bundle: %w", err)
	}
	if err := copyManagedBundleDirectory(plan.ReleaseBundleDir, plan.WorkspaceBundleDir); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	install, err := ExecuteManagedToolBundleInstall(root, pack, dependency, ManagedToolBundleInstallOptions{Execute: options.Execute})
	if err != nil {
		_ = os.RemoveAll(plan.WorkspaceBundleDir)
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	return ReleaseManagedToolBundleInstallResult{
		Status:                    install.Status,
		Mode:                      "workspace-copy-link",
		Action:                    plan.Action,
		Pack:                      plan.Pack,
		Dependency:                plan.Dependency,
		Kind:                      plan.Kind,
		Version:                   plan.Version,
		Source:                    plan.Source,
		SHA256:                    plan.SHA256,
		ExpectedSHA256:            plan.ExpectedSHA256,
		ExpectedSHA256Status:      plan.ExpectedSHA256Status,
		ProvenanceStatus:          plan.ProvenanceStatus,
		ProvenanceSourceURL:       plan.ProvenanceSourceURL,
		ProvenanceUpstreamVersion: plan.ProvenanceUpstreamVersion,
		ProvenanceRetrievedAt:     plan.ProvenanceRetrievedAt,
		ProvenanceVerifiedBy:      plan.ProvenanceVerifiedBy,
		CorpusStatus:              plan.CorpusStatus,
		CorpusName:                plan.CorpusName,
		CorpusSource:              plan.CorpusSource,
		CorpusRetrievedAt:         plan.CorpusRetrievedAt,
		CorpusFileCount:           plan.CorpusFileCount,
		CorpusSHA256:              plan.CorpusSHA256,
		CorpusDigestStatus:        plan.CorpusDigestStatus,
		CorpusDigestAlgorithm:     plan.CorpusDigestAlgorithm,
		CorpusDigestScope:         plan.CorpusDigestScope,
		SignatureStatus:           plan.SignatureStatus,
		SignatureAlgorithm:        plan.SignatureAlgorithm,
		SignatureKeyID:            plan.SignatureKeyID,
		SignatureSignedAt:         plan.SignatureSignedAt,
		SignatureSignedBy:         plan.SignatureSignedBy,
		UpdateStatus:              plan.UpdateStatus,
		UpdateLatestVersion:       plan.UpdateLatestVersion,
		UpdateLatestSHA256:        plan.UpdateLatestSHA256,
		UpdateCheckedAt:           plan.UpdateCheckedAt,
		UpdateMessage:             plan.UpdateMessage,
		ReleaseBundleDir:          plan.ReleaseBundleDir,
		ReleasePayloadPath:        plan.ReleasePayloadPath,
		WorkspaceBundleDir:        plan.WorkspaceBundleDir,
		WorkspacePayloadPath:      plan.WorkspacePayloadPath,
		InstallPath:               install.InstallPath,
	}, nil
}

func ReadManagedToolBundle(root, name string) (ManagedToolBundle, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundle{}, err
	}
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	dir, err := managedToolBundleRoot(root)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	return readManagedToolBundle(root, filepath.Join(dir, safeName))
}

func readReleaseManagedToolBundle(dir string) (ReleaseManagedToolBundle, error) {
	if err := requireAuthoredToolDirectory(dir, "release managed bundle directory"); err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	manifestPath := filepath.Join(dir, "bundle.yaml")
	if err := rejectUnsafeAuthoredToolFile(manifestPath, "release managed bundle manifest"); err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ReleaseManagedToolBundle{}, fmt.Errorf("read release managed bundle manifest %s: %w", manifestPath, err)
	}
	var manifest managedToolBundleManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return ReleaseManagedToolBundle{}, fmt.Errorf("parse release managed bundle manifest %s: %w", manifestPath, err)
	}
	if err := validateManagedToolBundleManifest(manifest, filepath.Base(dir)); err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	provenance, err := managedToolBundleProvenanceMetadataFromManifest(manifest)
	if err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	update, err := managedToolBundleUpdateMetadataFromManifest(manifest)
	if err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	payloadPath, err := resolveManagedBundlePayloadPath(dir, manifest.Payload)
	if err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	if err := validateManagedBundlePayload(payloadPath, manifest); err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	corpus, err := managedToolBundleCorpusMetadataFromManifest(manifest, payloadPath)
	if err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	signature, err := managedToolBundleSignatureMetadataFromManifest(manifest)
	if err != nil {
		return ReleaseManagedToolBundle{}, err
	}
	return ReleaseManagedToolBundle{
		Status:                    "available",
		Mode:                      "metadata-only",
		Name:                      strings.TrimSpace(manifest.Name),
		Kind:                      strings.TrimSpace(manifest.Kind),
		Version:                   strings.TrimSpace(manifest.Version),
		Source:                    strings.TrimSpace(manifest.Source),
		SHA256:                    strings.TrimSpace(manifest.SHA256),
		ProvenanceStatus:          provenance.Status,
		ProvenanceSourceURL:       provenance.SourceURL,
		ProvenanceUpstreamVersion: provenance.UpstreamVersion,
		ProvenanceRetrievedAt:     provenance.RetrievedAt,
		ProvenanceVerifiedBy:      provenance.VerifiedBy,
		CorpusStatus:              corpus.Status,
		CorpusName:                corpus.Name,
		CorpusSource:              corpus.Source,
		CorpusRetrievedAt:         corpus.RetrievedAt,
		CorpusFileCount:           corpus.FileCount,
		CorpusSHA256:              corpus.SHA256,
		CorpusDigestStatus:        corpus.DigestStatus,
		CorpusDigestAlgorithm:     corpus.DigestAlgorithm,
		CorpusDigestScope:         corpus.DigestScope,
		SignatureStatus:           signature.Status,
		SignatureAlgorithm:        signature.Algorithm,
		SignatureKeyID:            signature.KeyID,
		SignatureSignedAt:         signature.SignedAt,
		SignatureSignedBy:         signature.SignedBy,
		UpdateStatus:              update.Status,
		UpdateLatestVersion:       update.LatestVersion,
		UpdateLatestSHA256:        update.LatestSHA256,
		UpdateCheckedAt:           update.CheckedAt,
		UpdateMessage:             update.Message,
		ReleaseBundleDir:          dir,
		ReleaseManifestPath:       manifestPath,
		ReleasePayloadPath:        payloadPath,
	}, nil
}

func releaseManagedToolBundleStatusFromError(dir string, manifestPath string, err error) ReleaseManagedToolBundle {
	name := filepath.Base(dir)
	if strings.TrimSpace(name) == "." || strings.TrimSpace(name) == string(os.PathSeparator) {
		name = "unknown"
	}
	return ReleaseManagedToolBundle{
		Status:              "blocked",
		Mode:                "metadata-only",
		Name:                name,
		ProvenanceStatus:    "blocked",
		CorpusStatus:        "blocked",
		CorpusDigestStatus:  "blocked",
		SignatureStatus:     "blocked",
		UpdateStatus:        "blocked",
		ReleaseBundleDir:    dir,
		ReleaseManifestPath: manifestPath,
		Error:               err.Error(),
	}
}

func releaseManagedToolBundleStatusForWorkspaceWithExpectedSHA(root string, bundle ReleaseManagedToolBundle, expectedSHA string) ReleaseManagedToolBundleStatus {
	status := ReleaseManagedToolBundleStatus{
		Status:                    bundle.Status,
		Mode:                      "read-only",
		Name:                      bundle.Name,
		Kind:                      bundle.Kind,
		Version:                   bundle.Version,
		Source:                    bundle.Source,
		SHA256:                    bundle.SHA256,
		ExpectedSHA256:            expectedSHA,
		ExpectedSHA256Status:      releaseManagedExpectedSHA256Status(expectedSHA, bundle.SHA256),
		ProvenanceStatus:          bundle.ProvenanceStatus,
		ProvenanceSourceURL:       bundle.ProvenanceSourceURL,
		ProvenanceUpstreamVersion: bundle.ProvenanceUpstreamVersion,
		ProvenanceRetrievedAt:     bundle.ProvenanceRetrievedAt,
		ProvenanceVerifiedBy:      bundle.ProvenanceVerifiedBy,
		CorpusStatus:              bundle.CorpusStatus,
		CorpusName:                bundle.CorpusName,
		CorpusSource:              bundle.CorpusSource,
		CorpusRetrievedAt:         bundle.CorpusRetrievedAt,
		CorpusFileCount:           bundle.CorpusFileCount,
		CorpusSHA256:              bundle.CorpusSHA256,
		CorpusDigestStatus:        bundle.CorpusDigestStatus,
		CorpusDigestAlgorithm:     bundle.CorpusDigestAlgorithm,
		CorpusDigestScope:         bundle.CorpusDigestScope,
		SignatureStatus:           bundle.SignatureStatus,
		SignatureAlgorithm:        bundle.SignatureAlgorithm,
		SignatureKeyID:            bundle.SignatureKeyID,
		SignatureSignedAt:         bundle.SignatureSignedAt,
		SignatureSignedBy:         bundle.SignatureSignedBy,
		UpdateStatus:              bundle.UpdateStatus,
		UpdateLatestVersion:       bundle.UpdateLatestVersion,
		UpdateLatestSHA256:        bundle.UpdateLatestSHA256,
		UpdateCheckedAt:           bundle.UpdateCheckedAt,
		UpdateMessage:             bundle.UpdateMessage,
		DigestAlgorithm:           "sha256",
		DigestScope:               managedBundleDigestScope(bundle.Kind),
		ReleaseBundleDir:          bundle.ReleaseBundleDir,
		ReleaseManifestPath:       bundle.ReleaseManifestPath,
		ReleasePayloadPath:        bundle.ReleasePayloadPath,
		Error:                     bundle.Error,
	}
	if status.Status == "" {
		status.Status = "blocked"
	}
	if status.Status == "blocked" {
		if strings.TrimSpace(status.UpdateStatus) == "" {
			status.UpdateStatus = "blocked"
		}
		if strings.TrimSpace(status.CorpusStatus) == "" {
			status.CorpusStatus = "blocked"
		}
		if strings.TrimSpace(status.CorpusDigestStatus) == "" {
			status.CorpusDigestStatus = "blocked"
		}
		status.DigestStatus = "blocked"
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		return status
	}
	if expectedSHA != "" && expectedSHA != bundle.SHA256 {
		status.Status = "blocked"
		status.DigestStatus = "verified"
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("release managed bundle expected sha256 mismatch: expected %s actual %s", expectedSHA, bundle.SHA256)
		return status
	}
	status.DigestStatus = "verified"
	status.ImportStatus = "not_imported"
	status.LinkStatus = "missing"
	status.WorkspaceBundleDir = filepath.Join(root, "toolchain", "managed-bundles", bundle.Name)
	status.WorkspaceManifestPath = filepath.Join(status.WorkspaceBundleDir, "bundle.yaml")
	workspacePayloadPath, err := releaseWorkspacePayloadPath(bundle.ReleaseBundleDir, bundle.ReleasePayloadPath, status.WorkspaceBundleDir)
	if err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = err.Error()
		return status
	}
	status.WorkspacePayloadPath = workspacePayloadPath
	installPath, err := managedToolBundleInstallPath(root, managedToolBundleManifest{Name: bundle.Name, Kind: bundle.Kind})
	if err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = err.Error()
		return status
	}
	status.InstallPath = installPath
	return inspectReleaseManagedWorkspaceStatus(root, bundle, status)
}

func releaseManagedExpectedSHA256Status(expectedSHA string, actualSHA string) string {
	if strings.TrimSpace(expectedSHA) == "" {
		return ""
	}
	if expectedSHA == strings.TrimSpace(actualSHA) {
		return "matched"
	}
	return "mismatch"
}

func inspectReleaseManagedWorkspaceStatus(root string, releaseBundle ReleaseManagedToolBundle, status ReleaseManagedToolBundleStatus) ReleaseManagedToolBundleStatus {
	if err := rejectUnsafeAuthoredToolDirectory(filepath.Dir(status.WorkspaceBundleDir), "managed bundle directory"); err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = err.Error()
		return status
	}
	info, err := os.Lstat(status.WorkspaceBundleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return inspectReleaseManagedMissingWorkspaceInstall(status)
		}
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("inspect workspace managed bundle: %v", err)
		return status
	}
	if info.Mode()&os.ModeSymlink != 0 {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("workspace managed bundle directory must not be a symlink: %s", status.WorkspaceBundleDir)
		return status
	}
	if !info.IsDir() {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("workspace managed bundle is not a directory: %s", status.WorkspaceBundleDir)
		return status
	}
	workspaceBundle, err := readManagedToolBundle(root, status.WorkspaceBundleDir)
	if err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("workspace managed bundle blocked: %v", err)
		return status
	}
	status.WorkspaceManifestPath = workspaceBundle.ManifestPath
	status.WorkspacePayloadPath = workspaceBundle.PayloadPath
	if workspaceBundle.Kind != releaseBundle.Kind ||
		workspaceBundle.Version != releaseBundle.Version ||
		workspaceBundle.Source != releaseBundle.Source ||
		workspaceBundle.SHA256 != releaseBundle.SHA256 {
		status.ImportStatus = "conflict"
		status.LinkStatus = "conflict"
		status.Error = "workspace managed bundle metadata mismatch"
		return status
	}
	return inspectReleaseManagedImportedInstall(status)
}

func inspectReleaseManagedMissingWorkspaceInstall(status ReleaseManagedToolBundleStatus) ReleaseManagedToolBundleStatus {
	info, err := os.Lstat(status.InstallPath)
	if err != nil {
		if os.IsNotExist(err) {
			status.ImportStatus = "not_imported"
			status.LinkStatus = "missing"
			return status
		}
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("inspect managed install path: %v", err)
		return status
	}
	status.ImportStatus = "conflict"
	status.LinkStatus = "conflict"
	if info.Mode()&os.ModeSymlink != 0 {
		status.Error = fmt.Sprintf("managed install path already exists: %s", status.InstallPath)
		return status
	}
	status.Error = fmt.Sprintf("managed install path already exists: %s", status.InstallPath)
	return status
}

func inspectReleaseManagedImportedInstall(status ReleaseManagedToolBundleStatus) ReleaseManagedToolBundleStatus {
	if err := rejectUnsafeAuthoredToolDirectory(filepath.Dir(status.InstallPath), "managed install directory"); err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = err.Error()
		return status
	}
	info, err := os.Lstat(status.InstallPath)
	if err != nil {
		if os.IsNotExist(err) {
			status.ImportStatus = "imported"
			status.LinkStatus = "missing"
			status.Error = ""
			return status
		}
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("inspect managed install path: %v", err)
		return status
	}
	if info.Mode()&os.ModeSymlink == 0 {
		status.ImportStatus = "conflict"
		status.LinkStatus = "conflict"
		status.Error = fmt.Sprintf("managed install path already exists: %s", status.InstallPath)
		return status
	}
	target, err := os.Readlink(status.InstallPath)
	if err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("read managed install link: %v", err)
		return status
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(status.InstallPath), target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("resolve managed install link target: %v", err)
		return status
	}
	expectedAbs, err := filepath.Abs(status.WorkspacePayloadPath)
	if err != nil {
		status.ImportStatus = "blocked"
		status.LinkStatus = "blocked"
		status.Error = fmt.Sprintf("resolve expected managed install link target: %v", err)
		return status
	}
	if targetAbs != expectedAbs {
		status.ImportStatus = "conflict"
		status.LinkStatus = "conflict"
		status.Error = fmt.Sprintf("managed install link target mismatch: %s -> %s", status.InstallPath, targetAbs)
		return status
	}
	status.ImportStatus = "linked"
	status.LinkStatus = "linked"
	status.Error = ""
	return status
}

func managedBundleDigestScope(kind string) string {
	switch strings.TrimSpace(kind) {
	case "binary":
		return "file"
	case "rule_bundle":
		return "directory_canonical_records"
	default:
		return ""
	}
}

func releaseManagedToolBundleStatusSourcePath(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	path, err := filepath.Abs(filepath.Join(root, "toolchain", "release-managed-status-source.json"))
	if err != nil {
		return "", fmt.Errorf("resolve release managed status source marker: %w", err)
	}
	return path, nil
}

func writeReleaseManagedToolBundleStatusSource(source ReleaseManagedToolBundleStatusSource) error {
	if err := ensureAuthoredToolDirectory(filepath.Dir(source.Path), "release managed status source directory"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release managed status source marker: %w", err)
	}
	return writeAuthoredToolArtifactFile(source.Path, data, "release managed status source marker")
}

func releaseManagedToolBundleStatusSourceFromStatuses(path, releaseRoot string, statuses []ReleaseManagedToolBundleStatus) ReleaseManagedToolBundleStatusSource {
	source := ReleaseManagedToolBundleStatusSource{
		Path:        path,
		ReleaseRoot: releaseRoot,
		Total:       len(statuses),
	}
	for _, status := range statuses {
		switch strings.TrimSpace(status.Status) {
		case "available":
			source.Available++
		case "blocked":
			source.Blocked++
		}
		switch strings.TrimSpace(status.ImportStatus) {
		case "not_imported":
			source.NotImported++
		case "imported":
			source.Imported++
		case "linked":
			source.Linked++
		case "conflict":
			source.Conflict++
		}
	}
	return source
}

func inspectManagedToolBundleImportPayload(path string, kind string) (string, os.FileInfo, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", nil, fmt.Errorf("managed bundle import payload is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", nil, fmt.Errorf("resolve managed bundle import payload: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, fmt.Errorf("managed bundle import payload missing: %s", abs)
		}
		return "", nil, fmt.Errorf("inspect managed bundle import payload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("managed bundle import payload must not be a symlink: %s", abs)
	}
	if kind == "binary" && info.IsDir() {
		return "", nil, fmt.Errorf("managed binary import payload is a directory: %s", abs)
	}
	if kind == "rule_bundle" && !info.IsDir() {
		return "", nil, fmt.Errorf("managed rule_bundle import payload is not a directory: %s", abs)
	}
	if kind == "rule_bundle" {
		return abs, info, nil
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("managed bundle import payload must be a regular file: %s", abs)
	}
	return abs, info, nil
}

func digestManagedToolBundleImportPayload(path string, info os.FileInfo) (string, error) {
	if info.IsDir() {
		return digestManagedBundleDirectory(path)
	}
	return digestManagedToolBundleImportFile(path, info)
}

var managedBundleSourceAfterInspectHook func(path string)

func openManagedBundleSourceFileAfterInspect(path string, inspected os.FileInfo, label string) (*os.File, error) {
	if managedBundleSourceAfterInspectHook != nil {
		managedBundleSourceAfterInspectHook(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", label, err)
	}
	current, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat opened %s: %w", label, err)
	}
	if !os.SameFile(inspected, current) {
		_ = file.Close()
		return nil, fmt.Errorf("%s changed after inspection: %s", label, path)
	}
	if !current.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("%s must be a regular file: %s", label, path)
	}
	return file, nil
}

func digestManagedToolBundleImportFile(path string, inspected os.FileInfo) (string, error) {
	file, err := openManagedBundleSourceFileAfterInspect(path, inspected, "managed bundle import payload")
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("read managed bundle import payload for digest: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func normalizeManagedToolBundleExpectedSHA256(value string, provided bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" && !provided {
		return "", nil
	}
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle import expected sha256 is invalid")
	}
	decoded, err := hex.DecodeString(trimmed)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("managed bundle import expected sha256 is invalid")
	}
	return strings.ToLower(trimmed), nil
}

func managedToolBundleImportManifest(name, kind, version, source, sha string) []byte {
	payload := "payload/" + name
	if kind == "rule_bundle" {
		payload = "payload"
	}
	return []byte(
		"name: " + name + "\n" +
			"kind: " + kind + "\n" +
			"version: " + strconv.Quote(version) + "\n" +
			"source: " + strconv.Quote(source) + "\n" +
			"payload: " + payload + "\n" +
			"sha256: " + sha + "\n",
	)
}

func readManagedToolBundle(root, dir string) (ManagedToolBundle, error) {
	if err := requireAuthoredToolDirectory(dir, "managed bundle directory"); err != nil {
		return ManagedToolBundle{}, err
	}
	manifestPath := filepath.Join(dir, "bundle.yaml")
	if err := rejectUnsafeAuthoredToolFile(manifestPath, "managed bundle manifest"); err != nil {
		return ManagedToolBundle{}, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ManagedToolBundle{}, fmt.Errorf("read managed bundle manifest %s: %w", manifestPath, err)
	}
	var manifest managedToolBundleManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return ManagedToolBundle{}, fmt.Errorf("parse managed bundle manifest %s: %w", manifestPath, err)
	}
	if err := validateManagedToolBundleManifest(manifest, filepath.Base(dir)); err != nil {
		return ManagedToolBundle{}, err
	}
	provenance, err := managedToolBundleProvenanceMetadataFromManifest(manifest)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	update, err := managedToolBundleUpdateMetadataFromManifest(manifest)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	payloadPath, err := resolveManagedBundlePayloadPath(dir, manifest.Payload)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	if err := validateManagedBundlePayload(payloadPath, manifest); err != nil {
		return ManagedToolBundle{}, err
	}
	corpus, err := managedToolBundleCorpusMetadataFromManifest(manifest, payloadPath)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	signature, err := managedToolBundleSignatureMetadataFromManifest(manifest)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	installPath, err := managedToolBundleInstallPath(root, manifest)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	return ManagedToolBundle{
		Status:                    "available",
		Name:                      strings.TrimSpace(manifest.Name),
		Kind:                      strings.TrimSpace(manifest.Kind),
		Version:                   strings.TrimSpace(manifest.Version),
		Source:                    strings.TrimSpace(manifest.Source),
		SHA256:                    strings.TrimSpace(manifest.SHA256),
		ProvenanceStatus:          provenance.Status,
		ProvenanceSourceURL:       provenance.SourceURL,
		ProvenanceUpstreamVersion: provenance.UpstreamVersion,
		ProvenanceRetrievedAt:     provenance.RetrievedAt,
		ProvenanceVerifiedBy:      provenance.VerifiedBy,
		CorpusStatus:              corpus.Status,
		CorpusName:                corpus.Name,
		CorpusSource:              corpus.Source,
		CorpusRetrievedAt:         corpus.RetrievedAt,
		CorpusFileCount:           corpus.FileCount,
		CorpusSHA256:              corpus.SHA256,
		CorpusDigestStatus:        corpus.DigestStatus,
		CorpusDigestAlgorithm:     corpus.DigestAlgorithm,
		CorpusDigestScope:         corpus.DigestScope,
		SignatureStatus:           signature.Status,
		SignatureAlgorithm:        signature.Algorithm,
		SignatureKeyID:            signature.KeyID,
		SignatureSignedAt:         signature.SignedAt,
		SignatureSignedBy:         signature.SignedBy,
		UpdateStatus:              update.Status,
		UpdateLatestVersion:       update.LatestVersion,
		UpdateLatestSHA256:        update.LatestSHA256,
		UpdateCheckedAt:           update.CheckedAt,
		UpdateMessage:             update.Message,
		BundleDir:                 dir,
		ManifestPath:              manifestPath,
		PayloadPath:               payloadPath,
		InstallPath:               installPath,
	}, nil
}

func validateManagedToolBundleManifest(manifest managedToolBundleManifest, dirName string) error {
	name, err := validateAuthoredToolName(manifest.Name)
	if err != nil {
		return err
	}
	if name != dirName {
		return fmt.Errorf("managed bundle name %s does not match directory %s", name, dirName)
	}
	switch strings.TrimSpace(manifest.Kind) {
	case "binary", "rule_bundle":
	default:
		return fmt.Errorf("managed bundle kind is unsupported: %s", manifest.Kind)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("managed bundle version is required")
	}
	if strings.TrimSpace(manifest.Payload) == "" {
		return fmt.Errorf("managed bundle payload is required")
	}
	if strings.TrimSpace(manifest.Kind) == "binary" || strings.TrimSpace(manifest.Kind) == "rule_bundle" {
		digest := strings.TrimSpace(manifest.SHA256)
		if digest == "" {
			return fmt.Errorf("managed %s bundle sha256 is required", strings.TrimSpace(manifest.Kind))
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("managed %s bundle sha256 is invalid", strings.TrimSpace(manifest.Kind))
		}
	}
	return nil
}

func managedToolBundleProvenanceMetadataFromManifest(manifest managedToolBundleManifest) (managedToolBundleProvenanceMetadata, error) {
	if manifest.Provenance == nil {
		return managedToolBundleProvenanceMetadata{Status: "missing"}, nil
	}
	sourceURL := manifest.Provenance.SourceURL
	upstreamVersion := manifest.Provenance.UpstreamVersion
	retrievedAt := manifest.Provenance.RetrievedAt
	verifiedBy := manifest.Provenance.VerifiedBy
	if err := validateManagedToolBundleProvenanceToken("source_url", sourceURL); err != nil {
		return managedToolBundleProvenanceMetadata{}, err
	}
	if err := validateManagedToolBundleProvenanceToken("upstream_version", upstreamVersion); err != nil {
		return managedToolBundleProvenanceMetadata{}, err
	}
	if err := validateManagedToolBundleProvenanceToken("retrieved_at", retrievedAt); err != nil {
		return managedToolBundleProvenanceMetadata{}, err
	}
	if err := validateManagedToolBundleProvenanceToken("verified_by", verifiedBy); err != nil {
		return managedToolBundleProvenanceMetadata{}, err
	}
	provenance := managedToolBundleProvenanceMetadata{
		Status:          "recorded",
		SourceURL:       strings.TrimSpace(sourceURL),
		UpstreamVersion: strings.TrimSpace(upstreamVersion),
		RetrievedAt:     strings.TrimSpace(retrievedAt),
		VerifiedBy:      strings.TrimSpace(verifiedBy),
	}
	if provenance.SourceURL == "" && provenance.UpstreamVersion == "" && provenance.RetrievedAt == "" && provenance.VerifiedBy == "" {
		provenance.Status = "missing"
		return provenance, nil
	}
	if provenance.SourceURL != "" {
		parsed, err := url.Parse(provenance.SourceURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return managedToolBundleProvenanceMetadata{}, fmt.Errorf("managed bundle provenance source_url must be http or https")
		}
	}
	if provenance.RetrievedAt != "" {
		if _, err := time.Parse(time.RFC3339, provenance.RetrievedAt); err != nil {
			return managedToolBundleProvenanceMetadata{}, fmt.Errorf("managed bundle provenance retrieved_at must be RFC3339")
		}
	}
	return provenance, nil
}

func validateManagedToolBundleProvenanceToken(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("managed bundle provenance %s must be a single token", field)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("managed bundle provenance %s must be a single token", field)
		}
	}
	return nil
}

func managedToolBundleCorpusMetadataFromManifest(manifest managedToolBundleManifest, payloadPath string) (managedToolBundleCorpusMetadata, error) {
	if manifest.Corpus == nil {
		return managedToolBundleCorpusMetadata{Status: "missing", DigestStatus: "missing"}, nil
	}
	if strings.TrimSpace(manifest.Kind) != "rule_bundle" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus metadata is supported only for rule_bundle bundles")
	}
	corpus := manifest.Corpus
	schema := strings.TrimSpace(corpus.Schema)
	name := strings.TrimSpace(corpus.Name)
	source := strings.TrimSpace(corpus.Source)
	retrievedAt := strings.TrimSpace(corpus.RetrievedAt)
	sha := strings.ToLower(strings.TrimSpace(corpus.SHA256))
	digestAlgorithm := strings.TrimSpace(corpus.DigestAlgorithm)
	digestScope := strings.TrimSpace(corpus.DigestScope)
	if schema == "" && name == "" && source == "" && retrievedAt == "" && corpus.FileCount == 0 && sha == "" && digestAlgorithm == "" && digestScope == "" {
		return managedToolBundleCorpusMetadata{Status: "missing", DigestStatus: "missing"}, nil
	}
	for field, value := range map[string]string{
		"schema":           corpus.Schema,
		"name":             corpus.Name,
		"source":           corpus.Source,
		"retrieved_at":     corpus.RetrievedAt,
		"sha256":           corpus.SHA256,
		"digest_algorithm": corpus.DigestAlgorithm,
		"digest_scope":     corpus.DigestScope,
	} {
		if err := validateManagedToolBundleCorpusToken(field, value); err != nil {
			return managedToolBundleCorpusMetadata{}, err
		}
	}
	if schema != "ouroboros.rule_corpus.v1" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus schema must be ouroboros.rule_corpus.v1")
	}
	if name == "" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus name is required")
	}
	if source == "" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus source is required")
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus source must be http or https")
	}
	if retrievedAt == "" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus retrieved_at is required")
	}
	if _, err := time.Parse(time.RFC3339, retrievedAt); err != nil {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus retrieved_at must be RFC3339")
	}
	if corpus.FileCount <= 0 {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus file_count must be positive")
	}
	if digestAlgorithm != "sha256" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus digest_algorithm must be sha256")
	}
	if digestScope != "directory_canonical_records" {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus digest_scope must be directory_canonical_records")
	}
	decoded, err := hex.DecodeString(sha)
	if err != nil || len(decoded) != sha256.Size {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus sha256 must be a SHA-256 hex digest")
	}
	expectedSHA := strings.ToLower(strings.TrimSpace(manifest.SHA256))
	if sha != expectedSHA {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus sha256 mismatch: expected %s actual %s", expectedSHA, sha)
	}
	actualFileCount, err := countManagedBundleDirectoryRegularFiles(payloadPath)
	if err != nil {
		return managedToolBundleCorpusMetadata{}, err
	}
	if corpus.FileCount != actualFileCount {
		return managedToolBundleCorpusMetadata{}, fmt.Errorf("managed bundle corpus file_count mismatch: expected %d actual %d", corpus.FileCount, actualFileCount)
	}
	return managedToolBundleCorpusMetadata{
		Status:          "recorded",
		Name:            name,
		Source:          source,
		RetrievedAt:     retrievedAt,
		FileCount:       corpus.FileCount,
		SHA256:          sha,
		DigestStatus:    "matched",
		DigestAlgorithm: digestAlgorithm,
		DigestScope:     digestScope,
	}, nil
}

func validateManagedToolBundleCorpusToken(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("managed bundle corpus %s must be a single token", field)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("managed bundle corpus %s must be a single token", field)
		}
	}
	return nil
}

func managedToolBundleSignatureMetadataFromManifest(manifest managedToolBundleManifest) (managedToolBundleSignatureMetadata, error) {
	if manifest.Signature == nil {
		return managedToolBundleSignatureMetadata{Status: "missing"}, nil
	}
	signature := manifest.Signature
	algorithm := strings.TrimSpace(signature.Algorithm)
	keyID := strings.TrimSpace(signature.KeyID)
	publicKeyText := strings.TrimSpace(signature.PublicKey)
	payloadSHA256 := strings.ToLower(strings.TrimSpace(signature.PayloadSHA256))
	signatureText := strings.TrimSpace(signature.Signature)
	signedAt := strings.TrimSpace(signature.SignedAt)
	signedBy := strings.TrimSpace(signature.SignedBy)
	if algorithm == "" && keyID == "" && publicKeyText == "" && payloadSHA256 == "" && signatureText == "" && signedAt == "" && signedBy == "" {
		return managedToolBundleSignatureMetadata{Status: "missing"}, nil
	}
	for field, value := range map[string]string{
		"algorithm":      signature.Algorithm,
		"key_id":         signature.KeyID,
		"public_key":     signature.PublicKey,
		"payload_sha256": signature.PayloadSHA256,
		"signature":      signature.Signature,
		"signed_at":      signature.SignedAt,
		"signed_by":      signature.SignedBy,
	} {
		if err := validateManagedToolBundleSignatureToken(field, value); err != nil {
			return managedToolBundleSignatureMetadata{}, err
		}
	}
	if algorithm != "ed25519" {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature algorithm must be ed25519")
	}
	if publicKeyText == "" {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature public_key is required")
	}
	if payloadSHA256 == "" {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature payload_sha256 is required")
	}
	if signatureText == "" {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature is required")
	}
	decodedPayloadSHA, err := hex.DecodeString(payloadSHA256)
	if err != nil || len(decodedPayloadSHA) != sha256.Size {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature payload_sha256 must be a SHA-256 hex digest")
	}
	if payloadSHA256 != strings.ToLower(strings.TrimSpace(manifest.SHA256)) {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature payload_sha256 must match manifest sha256")
	}
	if signedAt != "" {
		if _, err := time.Parse(time.RFC3339, signedAt); err != nil {
			return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature signed_at must be RFC3339")
		}
	}
	publicKey, err := decodeManagedToolBundleSignatureBase64(publicKeyText, "public_key")
	if err != nil {
		return managedToolBundleSignatureMetadata{}, err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature public_key must be ed25519 public key bytes")
	}
	signatureBytes, err := decodeManagedToolBundleSignatureBase64(signatureText, "signature")
	if err != nil {
		return managedToolBundleSignatureMetadata{}, err
	}
	if len(signatureBytes) != ed25519.SignatureSize {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature must be ed25519 signature bytes")
	}
	message := managedToolBundleSignatureMessage(manifest, payloadSHA256)
	if !ed25519.Verify(ed25519.PublicKey(publicKey), message, signatureBytes) {
		return managedToolBundleSignatureMetadata{}, fmt.Errorf("managed bundle signature verification failed")
	}
	return managedToolBundleSignatureMetadata{
		Status:    "verified",
		Algorithm: algorithm,
		KeyID:     keyID,
		SignedAt:  signedAt,
		SignedBy:  signedBy,
	}, nil
}

func validateManagedToolBundleSignatureToken(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("managed bundle signature %s must be a single token", field)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("managed bundle signature %s must be a single token", field)
		}
	}
	return nil
}

func decodeManagedToolBundleSignatureBase64(value string, field string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("managed bundle signature %s must be base64", field)
}

func managedToolBundleSignatureMessage(manifest managedToolBundleManifest, payloadSHA256 string) []byte {
	return []byte("ouroboros-managed-bundle-signature-v1\nname:" + strings.TrimSpace(manifest.Name) + "\nkind:" + strings.TrimSpace(manifest.Kind) + "\nversion:" + strings.TrimSpace(manifest.Version) + "\npayload_sha256:" + strings.ToLower(strings.TrimSpace(payloadSHA256)) + "\n")
}

func managedToolBundleUpdateMetadataFromManifest(manifest managedToolBundleManifest) (managedToolBundleUpdateMetadata, error) {
	if manifest.Update == nil {
		return managedToolBundleUpdateMetadata{Status: "unknown"}, nil
	}
	latestVersion := manifest.Update.LatestVersion
	latestSHA256 := manifest.Update.LatestSHA256
	checkedAt := manifest.Update.CheckedAt
	message := manifest.Update.Message
	if err := validateManagedToolBundleUpdateToken("latest_version", latestVersion); err != nil {
		return managedToolBundleUpdateMetadata{}, err
	}
	if err := validateManagedToolBundleUpdateToken("latest_sha256", latestSHA256); err != nil {
		return managedToolBundleUpdateMetadata{}, err
	}
	normalizedLatestSHA256 := strings.TrimSpace(latestSHA256)
	if normalizedLatestSHA256 != "" {
		decoded, err := hex.DecodeString(normalizedLatestSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return managedToolBundleUpdateMetadata{}, fmt.Errorf("managed bundle update latest_sha256 must be a SHA-256 hex digest")
		}
		normalizedLatestSHA256 = strings.ToLower(normalizedLatestSHA256)
	}
	if err := validateManagedToolBundleUpdateToken("checked_at", checkedAt); err != nil {
		return managedToolBundleUpdateMetadata{}, err
	}
	if err := validateManagedToolBundleUpdateToken("message", message); err != nil {
		return managedToolBundleUpdateMetadata{}, err
	}
	update := managedToolBundleUpdateMetadata{
		Status:        "unknown",
		LatestVersion: strings.TrimSpace(latestVersion),
		LatestSHA256:  normalizedLatestSHA256,
		CheckedAt:     strings.TrimSpace(checkedAt),
		Message:       strings.TrimSpace(message),
	}
	if update.CheckedAt != "" {
		if _, err := time.Parse(time.RFC3339, update.CheckedAt); err != nil {
			return managedToolBundleUpdateMetadata{}, fmt.Errorf("managed bundle update checked_at must be RFC3339")
		}
	}
	currentVersion := strings.TrimSpace(manifest.Version)
	currentSHA256 := strings.ToLower(strings.TrimSpace(manifest.SHA256))
	versionKnown := update.LatestVersion != ""
	shaKnown := update.LatestSHA256 != ""
	switch {
	case versionKnown && update.LatestVersion != currentVersion:
		update.Status = "update_available"
	case shaKnown && update.LatestSHA256 != currentSHA256:
		update.Status = "update_available"
	case versionKnown || shaKnown:
		update.Status = "current"
	}
	return update, nil
}

func validateManagedToolBundleUpdateToken(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("managed bundle update %s must be a single token", field)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("managed bundle update %s must be a single token", field)
		}
	}
	return nil
}

func resolveManagedBundlePayloadPath(bundleDir, payload string) (string, error) {
	trimmed := strings.TrimSpace(payload)
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("managed bundle payload path must stay under %s", bundleDir)
	}
	base, err := filepath.Abs(bundleDir)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle directory: %w", err)
	}
	candidate, err := filepath.Abs(filepath.Join(base, trimmed))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle payload: %w", err)
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle payload relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle payload path must stay under %s", base)
	}
	return candidate, nil
}

func validateManagedBundlePayload(path string, manifest managedToolBundleManifest) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("managed bundle payload missing: %s", path)
		}
		return fmt.Errorf("inspect managed bundle payload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed bundle payload must not be a symlink: %s", path)
	}
	if strings.TrimSpace(manifest.Kind) == "binary" && info.IsDir() {
		return fmt.Errorf("managed binary payload is a directory: %s", path)
	}
	if strings.TrimSpace(manifest.Kind) == "rule_bundle" && !info.IsDir() {
		return fmt.Errorf("managed rule_bundle payload is not a directory: %s", path)
	}
	if strings.TrimSpace(manifest.SHA256) != "" && info.IsDir() {
		actual, err := digestManagedBundleDirectory(path)
		if err != nil {
			return err
		}
		if actual != strings.TrimSpace(manifest.SHA256) {
			return fmt.Errorf("managed bundle payload sha256 mismatch: %s", path)
		}
	}
	if strings.TrimSpace(manifest.SHA256) != "" && !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read managed bundle payload for digest: %w", err)
		}
		sum := sha256.Sum256(data)
		actual := hex.EncodeToString(sum[:])
		if actual != strings.TrimSpace(manifest.SHA256) {
			return fmt.Errorf("managed bundle payload sha256 mismatch: %s", path)
		}
	}
	return nil
}

func releaseManagedToolBundleDir(releaseRoot, name string) (string, error) {
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return "", err
	}
	root, err := releaseManagedToolBundleRoot(releaseRoot)
	if err != nil {
		return "", err
	}
	if err := rejectUnsafeAuthoredToolDirectory(root, "release managed bundle root"); err != nil {
		return "", err
	}
	return filepath.Join(root, safeName), nil
}

func releaseManagedToolBundleRoot(releaseRoot string) (string, error) {
	if strings.TrimSpace(releaseRoot) == "" {
		return "", fmt.Errorf("release root is required")
	}
	root, err := filepath.Abs(filepath.Join(releaseRoot, "toolchains", "managed-bundles"))
	if err != nil {
		return "", fmt.Errorf("resolve release managed bundle root: %w", err)
	}
	return root, nil
}

func releaseWorkspacePayloadPath(releaseBundleDir, releasePayloadPath, workspaceBundleDir string) (string, error) {
	rel, err := filepath.Rel(releaseBundleDir, releasePayloadPath)
	if err != nil {
		return "", fmt.Errorf("resolve release payload relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("release managed payload path must stay under %s", releaseBundleDir)
	}
	return filepath.Join(workspaceBundleDir, rel), nil
}

func copyManagedBundleDirectory(src, dst string) error {
	return copyManagedBundleDirectoryWithFileMode(src, dst, 0)
}

func copyManagedBundleDirectoryWithFileMode(src, dst string, fileMode os.FileMode) error {
	if err := requireAuthoredToolDirectory(src, "release managed bundle directory"); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create managed bundle target directory: %w", err)
	}
	if err := filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("resolve managed bundle copy path: %w", err)
		}
		if rel == "." {
			return nil
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("managed bundle copy path must stay under %s", src)
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect managed bundle source: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle source must not contain symlinks: %s", path)
		}
		if entry.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create managed bundle target directory: %w", err)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed bundle source must contain only regular files: %s", path)
		}
		mode := info.Mode().Perm()
		if fileMode != 0 {
			mode = fileMode
		}
		if err := copyManagedBundleFile(path, target, mode, info); err != nil {
			return err
		}
		return nil
	}); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}

func copyManagedBundleFile(src, dst string, mode os.FileMode, inspected os.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create managed bundle target file directory: %w", err)
	}
	in, err := openManagedBundleSourceFileAfterInspect(src, inspected, "managed bundle source file")
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create managed bundle target file: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy managed bundle target file: %w", err)
	}
	if err := out.Chmod(mode); err != nil {
		return fmt.Errorf("chmod managed bundle target file: %w", err)
	}
	return nil
}

const managedRuleBundleDirectoryDigestVersion = "ouroboros managed rule_bundle directory digest v1\n"

func countManagedBundleDirectoryRegularFiles(dir string) (int, error) {
	count := 0
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect managed bundle corpus path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle corpus path must not be a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed bundle corpus path must be a regular file: %s", path)
		}
		count++
		return nil
	}); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, fmt.Errorf("managed rule_bundle payload must contain at least one regular file: %s", dir)
	}
	return count, nil
}

func digestManagedBundleDirectory(dir string) (string, error) {
	paths := []string{}
	inspectedFiles := map[string]os.FileInfo{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect managed bundle digest path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle digest path must not be a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed bundle digest path must be a regular file: %s", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("resolve managed bundle digest path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		paths = append(paths, rel)
		inspectedFiles[rel] = info
		return nil
	}); err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("managed rule_bundle payload must contain at least one regular file: %s", dir)
	}
	sort.Strings(paths)
	hash := sha256.New()
	hash.Write([]byte(managedRuleBundleDirectoryDigestVersion))
	for _, rel := range paths {
		file, err := openManagedBundleSourceFileAfterInspect(filepath.Join(dir, filepath.FromSlash(rel)), inspectedFiles[rel], "managed bundle digest file")
		if err != nil {
			return "", err
		}
		contentHash := sha256.New()
		size, err := io.Copy(contentHash, file)
		closeErr := file.Close()
		if err != nil {
			return "", fmt.Errorf("read managed bundle digest file: %w", err)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close managed bundle digest file: %w", closeErr)
		}
		var field [8]byte
		relBytes := []byte(rel)
		binary.BigEndian.PutUint64(field[:], uint64(len(relBytes)))
		hash.Write(field[:])
		hash.Write(relBytes)
		binary.BigEndian.PutUint64(field[:], uint64(size))
		hash.Write(field[:])
		hash.Write(contentHash.Sum(nil))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func findManagedInstallDependency(root, pack, dependency string) (toolchain.Dependency, error) {
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return toolchain.Dependency{}, err
	}
	for _, manifest := range manifests {
		if manifest.Name != strings.TrimSpace(pack) {
			continue
		}
		for _, declared := range manifest.Dependencies {
			if strings.TrimSpace(declared.Name) == strings.TrimSpace(dependency) {
				return declared, nil
			}
		}
		return toolchain.Dependency{}, fmt.Errorf("toolchain dependency not found: %s/%s", pack, dependency)
	}
	return toolchain.Dependency{}, fmt.Errorf("toolchain pack not found: %s", pack)
}

func managedToolBundleRoot(root string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(root, "toolchain", "managed-bundles"))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle directory: %w", err)
	}
	return dir, nil
}

func managedToolBundleInstallPath(root string, manifest managedToolBundleManifest) (string, error) {
	name, err := validateAuthoredToolName(manifest.Name)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(manifest.Kind) {
	case "binary":
		return filepath.Join(root, "toolchain", "bin", name), nil
	case "rule_bundle":
		return filepath.Join(root, "toolchain", "rules", name), nil
	default:
		return "", fmt.Errorf("managed bundle kind is unsupported: %s", manifest.Kind)
	}
}

func dependencyKindForManagedInstall(dependency toolchain.Dependency) string {
	if strings.TrimSpace(dependency.Kind) == "" {
		return "binary"
	}
	return strings.TrimSpace(dependency.Kind)
}

func rejectUnsafeAuthoredToolDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func ensureAuthoredToolDirectory(path string, label string) error {
	if err := rejectUnsafeAuthoredToolDirectory(path, label); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", label, err)
	}
	return requireAuthoredToolDirectory(path, label)
}

func CreateToolchainInstallRequest(root, pack, dependency string) (ToolchainInstallRequest, error) {
	statuses, err := InspectToolchains(root)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}

	for _, status := range statuses {
		if status.Name != pack {
			continue
		}
		for _, step := range status.InstallPlan {
			if step.Name != dependency {
				continue
			}
			request := ToolchainInstallRequest{
				Status:      "pending",
				Pack:        status.Name,
				Dependency:  step.Name,
				Kind:        step.Kind,
				State:       string(step.State),
				Required:    step.Required,
				Command:     append([]string(nil), step.Command...),
				Hint:        step.Hint,
				Source:      step.Source,
				ImageDigest: step.ImageDigest,
				DependsOn:   append([]string(nil), step.DependsOn...),
			}
			dir := toolchainInstallRequestDir(root)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return ToolchainInstallRequest{}, fmt.Errorf("create install request directory: %w", err)
			}
			request.Path = filepath.Join(dir, safeInstallRequestName(pack+"-"+dependency)+".json")
			data, err := json.MarshalIndent(request, "", "  ")
			if err != nil {
				return ToolchainInstallRequest{}, fmt.Errorf("encode install request: %w", err)
			}
			data = append(data, '\n')
			if err := os.WriteFile(request.Path, data, 0o644); err != nil {
				return ToolchainInstallRequest{}, fmt.Errorf("write install request: %w", err)
			}
			return request, nil
		}
		return ToolchainInstallRequest{}, fmt.Errorf("install plan step not found for %s/%s", pack, dependency)
	}
	return ToolchainInstallRequest{}, fmt.Errorf("toolchain pack not found: %s", pack)
}

func ListToolchainInstallRequests(root string) ([]ToolchainInstallRequest, error) {
	doctor, err := doctorToolchainInstallRuntime(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallRequestDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install requests: %w", err)
	}
	sort.Strings(paths)
	requests := make([]ToolchainInstallRequest, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read install request %s: %w", path, err)
		}
		var request ToolchainInstallRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, fmt.Errorf("decode install request %s: %w", path, err)
		}
		request.Path = path
		requests = append(requests, request)
	}
	return requests, nil
}

func UpdateToolchainInstallRequestStatus(root, requestPath, status string) (ToolchainInstallRequest, error) {
	nextStatus, err := normalizeInstallRequestStatus(status)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRequest{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallRequestPath(root, requestPath)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("read install request %s: %w", path, err)
	}
	var request ToolchainInstallRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("decode install request %s: %w", path, err)
	}
	request.Status = nextStatus
	request.Path = path
	encoded, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("encode install request: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("write install request %s: %w", path, err)
	}
	return request, nil
}

func normalizeInstallRequestStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		return "approved", nil
	case "denied":
		return "denied", nil
	default:
		return "", fmt.Errorf("unsupported install request status: %s", status)
	}
}

func installRequestDisplayStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func resolveToolchainInstallRequestPath(root, requestPath string) (string, error) {
	trimmed := strings.TrimSpace(requestPath)
	if trimmed == "" {
		return "", fmt.Errorf("install request path is required")
	}
	dir, err := filepath.Abs(toolchainInstallRequestDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install request directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install request path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install request relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install request path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install request path must be a json file")
	}
	return candidate, nil
}

func resolveManagedToolBundleUpdateRequestPath(root, requestPath string) (string, error) {
	trimmed := strings.TrimSpace(requestPath)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update request path is required")
	}
	dir, err := filepath.Abs(managedToolBundleUpdateRequestDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update request directory: %w", err)
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle update request directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update request path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update request relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle update request path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("managed bundle update request path must be a json file")
	}
	return candidate, nil
}

func resolveManagedToolBundleUpdateDownloadPlanPath(root, planPath string) (string, error) {
	trimmed := strings.TrimSpace(planPath)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update download plan path is required")
	}
	dir, err := filepath.Abs(managedToolBundleUpdateDownloadPlanDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download plan directory: %w", err)
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle update download plan directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download plan path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download plan relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle update download plan path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("managed bundle update download plan path must be a json file")
	}
	return candidate, nil
}

func resolveManagedToolBundleUpdateDownloadExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update download execution path is required")
	}
	dir, err := filepath.Abs(managedToolBundleUpdateDownloadExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download execution directory: %w", err)
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle update download execution directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle update download execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("managed bundle update download execution path must be a json file")
	}
	return candidate, nil
}

func resolveManagedToolBundleUpdateApplyExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update apply execution path is required")
	}
	dir, err := filepath.Abs(managedToolBundleUpdateApplyExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update apply execution directory: %w", err)
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle update apply execution directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update apply execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update apply execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle update apply execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("managed bundle update apply execution path must be a json file")
	}
	return candidate, nil
}

func resolveManagedToolBundleUpdateDownloadPayloadPath(root, payloadPath string) (string, error) {
	trimmed := strings.TrimSpace(payloadPath)
	if trimmed == "" {
		return "", fmt.Errorf("managed bundle update apply staged payload path is required")
	}
	dir, err := filepath.Abs(managedToolBundleUpdateDownloadStagingDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update download staging directory: %w", err)
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle update download staging directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update apply staged payload path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle update apply staged payload relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle update apply staged payload path must stay under %s", dir)
	}
	if filepath.Base(candidate) != "payload" {
		return "", fmt.Errorf("managed bundle update apply staged payload path must end with payload")
	}
	return candidate, nil
}

func resolveToolchainInstallDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("install dispatch path is required")
	}
	dir, err := filepath.Abs(toolchainInstallDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveToolCompilePreviewReviewPath(root, reviewPath string) (string, error) {
	trimmed := strings.TrimSpace(reviewPath)
	if trimmed == "" {
		return "", fmt.Errorf("compile preview review path is required")
	}
	dir, err := filepath.Abs(toolCompilePreviewReviewDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("compile preview review path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("compile preview review path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("install execution path is required")
	}
	dir, err := filepath.Abs(toolchainInstallExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install execution path must be a json file")
	}
	return candidate, nil
}

func resolveToolCompilePreviewReviewExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("compile preview review execution path is required")
	}
	dir, err := filepath.Abs(toolCompilePreviewReviewExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("compile preview review execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("compile preview review execution path must be a json file")
	}
	return candidate, nil
}

func resolveToolCompilePreviewReviewExecutionDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("compile preview review execution dispatch path is required")
	}
	dir, err := filepath.Abs(toolCompilePreviewReviewExecutionDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review execution dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review execution dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review execution dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("compile preview review execution dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("compile preview review execution dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallRetryDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("install retry dispatch path is required")
	}
	dir, err := filepath.Abs(toolchainInstallRetryDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install retry dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install retry dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install retry dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallRetryExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("install retry execution path is required")
	}
	dir, err := filepath.Abs(toolchainInstallRetryExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install retry execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install retry execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install retry execution path must be a json file")
	}
	return candidate, nil
}

func toolchainInstallRequestDir(root string) string {
	return filepath.Join(root, "toolchain", "install-requests")
}

func managedToolBundleUpdateRequestDir(root string) string {
	return filepath.Join(root, "toolchain", "managed-update-requests")
}

func managedToolBundleUpdateDownloadPlanDir(root string) string {
	return filepath.Join(root, "toolchain", "managed-update-download-plans")
}

func managedToolBundleUpdateDownloadStagingDir(root string) string {
	return filepath.Join(root, "toolchain", "managed-update-downloads")
}

func managedToolBundleUpdateDownloadExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "managed-update-download-executions")
}

func managedToolBundleUpdateApplyExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "managed-update-apply-executions")
}

func toolCompilePreviewReviewDir(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-reviews")
}

func toolCompilePreviewReviewExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-review-executions")
}

func toolCompilePreviewReviewExecutionDispatchDir(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-review-execution-dispatches")
}

func toolCompilePreviewReviewSelectionPath(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-review-selection.json")
}

func toolchainInstallDispatchDir(root string) string {
	return filepath.Join(root, "toolchain", "install-dispatches")
}

func toolchainInstallExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "install-executions")
}

func toolchainInstallRetryDispatchDir(root string) string {
	return filepath.Join(root, "toolchain", "install-retry-dispatches")
}

func toolchainInstallRetryExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "install-retry-executions")
}

func safeInstallDispatchName(plan ToolchainInstallDispatchPlan) string {
	parts := []string{"install-dispatch", plan.Summary.Line()}
	for _, request := range plan.Ready {
		parts = append(parts, request.Path, request.Status, request.Pack, request.Dependency)
	}
	for _, skipped := range plan.Skipped {
		request := skipped.Request
		parts = append(parts, request.Path, request.Status, request.Pack, request.Dependency, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeToolCompilePreviewReviewName(review ToolCompilePreviewReview) string {
	parts := []string{
		"compile-preview-review",
		string(review.Preview.Action),
		fmt.Sprintf("nodes=%d warnings=%d", len(review.Preview.Nodes), len(review.Preview.Warnings)),
	}
	for _, warning := range review.Preview.Warnings {
		parts = append(parts, warning)
	}
	for _, node := range review.Preview.Nodes {
		parts = append(
			parts,
			node.ID,
			node.ManifestName,
			node.ToolID,
			node.AdapterID,
			node.Operation,
			node.ParserID,
			node.TargetKind,
			node.DependencyName,
			node.DependencyKind,
			node.Capability,
			node.Runner,
			node.Parser,
			strconv.FormatFloat(node.SelectionScore, 'g', -1, 64),
			strings.Join(node.SelectionSignals, ","),
			strings.Join(node.RunCommand, "\x00"),
		)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeInstallRetryDispatchName(plan ToolchainInstallRetryPlan) string {
	parts := []string{
		"install-retry-dispatch",
		fmt.Sprintf("total=%d ready=%d skipped=%d", plan.Summary.Total, plan.Summary.Ready, plan.Summary.Skipped),
	}
	for _, candidate := range plan.Ready {
		parts = append(parts, candidate.ExecutionPath, candidate.DispatchPath, candidate.RequestPath, candidate.Pack, candidate.Dependency, candidate.LastStatus, candidate.LastError)
	}
	for _, skipped := range plan.Skipped {
		parts = append(parts, skipped.ExecutionPath, skipped.DispatchPath, skipped.RequestPath, skipped.Pack, skipped.Dependency, skipped.Status, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeManagedToolBundleUpdateRequestName(request ManagedToolBundleUpdateRequest) string {
	parts := []string{
		"managed-update-request",
		request.Name,
		request.Kind,
		request.CurrentVersion,
		request.CurrentSHA256,
		request.LatestVersion,
		request.LatestSHA256,
		request.UpdateCheckedAt,
		request.ReleaseManifestPath,
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeManagedToolBundleUpdateDownloadPlanName(plan ManagedToolBundleUpdateDownloadPlan) string {
	parts := []string{
		"managed-update-download-plan",
		plan.Name,
		plan.Kind,
		plan.RequestPath,
		plan.SourceURL,
		plan.ExpectedSHA256,
		plan.LatestVersion,
		plan.LatestSHA256,
		plan.UpdateCheckedAt,
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeManagedToolBundleUpdateDownloadExecutionName(execution ManagedToolBundleUpdateDownloadExecution) string {
	parts := []string{
		"managed-update-download-execution",
		execution.PlanPath,
		execution.SourceURL,
		execution.ExpectedSHA256,
		execution.Actor,
		execution.ExecutedAt,
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeManagedToolBundleUpdateApplyExecutionName(execution ManagedToolBundleUpdateApplyExecution) string {
	parts := []string{
		"managed-update-apply-execution",
		execution.DownloadExecutionPath,
		execution.ExpectedSHA256,
		execution.WorkspaceBundleDir,
		execution.Actor,
		execution.AppliedAt,
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeInstallRequestName(value string) string {
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])[:12]

	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	slug := strings.Trim(builder.String(), "-._")
	if slug == "" {
		slug = "install-request"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-._")
	}
	return slug + "-" + digest
}
