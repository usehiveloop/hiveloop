// Package model houses gorm models for the Onyx-derived RAG
// subsystem.
//
// Enums are split across multiple files by the tables they describe
// (enums_document.go: DocumentSource + HierarchyNodeType; enums_index_attempt.go:
// indexing + sync status; enums_sync_state.go: connection lifecycle +
// access type) so each file reads as a small, self-contained enum
// group.
package model

// DocumentSource enumerates every upstream source a document can come
// from. Verbatim port of Onyx's DocumentSource string-enum at
// backend/onyx/configs/constants.py:205-262. Values MUST match byte-for-byte
// because they are persisted to the DB and because the ACL helper
// BuildExtGroupName lowercases the source name into ACL strings written to
// the vector index — any drift would produce silent zero-result filters.
type DocumentSource string

// DocumentSource constants — see Onyx constants.py:205-262 for the
// authoritative list. Comment beside each constant cites the exact Onyx
// line.
const (
	DocumentSourceIngestionAPI       DocumentSource = "ingestion_api"         // constants.py:207
	DocumentSourceSlack              DocumentSource = "slack"                 // constants.py:208
	DocumentSourceWeb                DocumentSource = "web"                   // constants.py:209
	DocumentSourceGoogleDrive        DocumentSource = "google_drive"          // constants.py:210
	DocumentSourceGmail              DocumentSource = "gmail"                 // constants.py:211
	DocumentSourceRequestTracker     DocumentSource = "requesttracker"        // constants.py:212
	DocumentSourceGithub             DocumentSource = "github"                // constants.py:213
	DocumentSourceGitbook            DocumentSource = "gitbook"               // constants.py:214
	DocumentSourceGitlab             DocumentSource = "gitlab"                // constants.py:215
	DocumentSourceGuru               DocumentSource = "guru"                  // constants.py:216
	DocumentSourceBookstack          DocumentSource = "bookstack"             // constants.py:217
	DocumentSourceOutline            DocumentSource = "outline"               // constants.py:218
	DocumentSourceConfluence         DocumentSource = "confluence"            // constants.py:219
	DocumentSourceJira               DocumentSource = "jira"                  // constants.py:220
	DocumentSourceSlab               DocumentSource = "slab"                  // constants.py:221
	DocumentSourceProductboard       DocumentSource = "productboard"          // constants.py:222
	DocumentSourceFile               DocumentSource = "file"                  // constants.py:223
	DocumentSourceCoda               DocumentSource = "coda"                  // constants.py:224
	DocumentSourceCanvas             DocumentSource = "canvas"                // constants.py:225
	DocumentSourceNotion             DocumentSource = "notion"                // constants.py:226
	DocumentSourceZulip              DocumentSource = "zulip"                 // constants.py:227
	DocumentSourceLinear             DocumentSource = "linear"                // constants.py:228
	DocumentSourceHubspot            DocumentSource = "hubspot"               // constants.py:229
	DocumentSourceDocument360        DocumentSource = "document360"           // constants.py:230
	DocumentSourceGong               DocumentSource = "gong"                  // constants.py:231
	DocumentSourceGoogleSites        DocumentSource = "google_sites"          // constants.py:232
	DocumentSourceZendesk            DocumentSource = "zendesk"               // constants.py:233
	DocumentSourceLoopio             DocumentSource = "loopio"                // constants.py:234
	DocumentSourceDropbox            DocumentSource = "dropbox"               // constants.py:235
	DocumentSourceSharepoint         DocumentSource = "sharepoint"            // constants.py:236
	DocumentSourceTeams              DocumentSource = "teams"                 // constants.py:237
	DocumentSourceSalesforce         DocumentSource = "salesforce"            // constants.py:238
	DocumentSourceDiscourse          DocumentSource = "discourse"             // constants.py:239
	DocumentSourceAxero              DocumentSource = "axero"                 // constants.py:240
	DocumentSourceClickup            DocumentSource = "clickup"               // constants.py:241
	DocumentSourceMediawiki          DocumentSource = "mediawiki"             // constants.py:242
	DocumentSourceWikipedia          DocumentSource = "wikipedia"             // constants.py:243
	DocumentSourceAsana              DocumentSource = "asana"                 // constants.py:244
	DocumentSourceS3                 DocumentSource = "s3"                    // constants.py:245
	DocumentSourceR2                 DocumentSource = "r2"                    // constants.py:246
	DocumentSourceGoogleCloudStorage DocumentSource = "google_cloud_storage"  // constants.py:247
	DocumentSourceOCIStorage         DocumentSource = "oci_storage"           // constants.py:248
	DocumentSourceXenforo            DocumentSource = "xenforo"               // constants.py:249
	DocumentSourceNotApplicable      DocumentSource = "not_applicable"        // constants.py:250
	DocumentSourceDiscord            DocumentSource = "discord"               // constants.py:251
	DocumentSourceFreshdesk          DocumentSource = "freshdesk"             // constants.py:252
	DocumentSourceFireflies          DocumentSource = "fireflies"             // constants.py:253
	DocumentSourceEgnyte             DocumentSource = "egnyte"                // constants.py:254
	DocumentSourceAirtable           DocumentSource = "airtable"              // constants.py:255
	DocumentSourceHighspot           DocumentSource = "highspot"              // constants.py:256
	DocumentSourceDrupalWiki         DocumentSource = "drupal_wiki"           // constants.py:257
	DocumentSourceIMAP               DocumentSource = "imap"                  // constants.py:259
	DocumentSourceBitbucket          DocumentSource = "bitbucket"             // constants.py:260
	DocumentSourceTestrail           DocumentSource = "testrail"              // constants.py:261
	DocumentSourceMockConnector      DocumentSource = "mock_connector"        // constants.py:264
	DocumentSourceUserFile           DocumentSource = "user_file"             // constants.py:266
	DocumentSourceCraftFile          DocumentSource = "craft_file"            // constants.py:269
)

// allDocumentSources is an internal allowlist used by IsValid. Kept in a
// var (not a generated function) so the set lives next to the const block
// above for auditability against Onyx.
var allDocumentSources = map[DocumentSource]struct{}{
	DocumentSourceIngestionAPI:       {},
	DocumentSourceSlack:              {},
	DocumentSourceWeb:                {},
	DocumentSourceGoogleDrive:        {},
	DocumentSourceGmail:              {},
	DocumentSourceRequestTracker:     {},
	DocumentSourceGithub:             {},
	DocumentSourceGitbook:            {},
	DocumentSourceGitlab:             {},
	DocumentSourceGuru:               {},
	DocumentSourceBookstack:          {},
	DocumentSourceOutline:            {},
	DocumentSourceConfluence:         {},
	DocumentSourceJira:               {},
	DocumentSourceSlab:               {},
	DocumentSourceProductboard:       {},
	DocumentSourceFile:               {},
	DocumentSourceCoda:               {},
	DocumentSourceCanvas:             {},
	DocumentSourceNotion:             {},
	DocumentSourceZulip:              {},
	DocumentSourceLinear:             {},
	DocumentSourceHubspot:            {},
	DocumentSourceDocument360:        {},
	DocumentSourceGong:               {},
	DocumentSourceGoogleSites:        {},
	DocumentSourceZendesk:            {},
	DocumentSourceLoopio:             {},
	DocumentSourceDropbox:            {},
	DocumentSourceSharepoint:         {},
	DocumentSourceTeams:              {},
	DocumentSourceSalesforce:         {},
	DocumentSourceDiscourse:          {},
	DocumentSourceAxero:              {},
	DocumentSourceClickup:            {},
	DocumentSourceMediawiki:          {},
	DocumentSourceWikipedia:          {},
	DocumentSourceAsana:              {},
	DocumentSourceS3:                 {},
	DocumentSourceR2:                 {},
	DocumentSourceGoogleCloudStorage: {},
	DocumentSourceOCIStorage:         {},
	DocumentSourceXenforo:            {},
	DocumentSourceNotApplicable:      {},
	DocumentSourceDiscord:            {},
	DocumentSourceFreshdesk:          {},
	DocumentSourceFireflies:          {},
	DocumentSourceEgnyte:             {},
	DocumentSourceAirtable:           {},
	DocumentSourceHighspot:           {},
	DocumentSourceDrupalWiki:         {},
	DocumentSourceIMAP:               {},
	DocumentSourceBitbucket:          {},
	DocumentSourceTestrail:           {},
	DocumentSourceMockConnector:      {},
	DocumentSourceUserFile:           {},
	DocumentSourceCraftFile:          {},
}

// IsValid reports whether s is a known DocumentSource. Pure function,
// intended for admin-API input validation so typo'd source names are
// rejected before any DB write — mirrors Onyx's pydantic Enum validation
// which refuses unknown strings at construction time.
func (s DocumentSource) IsValid() bool {
	_, ok := allDocumentSources[s]
	return ok
}

// HierarchyNodeType enumerates every structural node type a connector can
// emit. Verbatim port of Onyx's HierarchyNodeType string-enum at
// backend/onyx/db/enums.py:306-340. Comments beside constants cite the
// exact Onyx line.
type HierarchyNodeType string

const (
	HierarchyNodeTypeFolder      HierarchyNodeType = "folder"       // enums.py:310
	HierarchyNodeTypeSource      HierarchyNodeType = "source"       // enums.py:313
	HierarchyNodeTypeSharedDrive HierarchyNodeType = "shared_drive" // enums.py:316
	HierarchyNodeTypeMyDrive     HierarchyNodeType = "my_drive"     // enums.py:317
	HierarchyNodeTypeSpace       HierarchyNodeType = "space"        // enums.py:320
	HierarchyNodeTypePage        HierarchyNodeType = "page"         // enums.py:321
	HierarchyNodeTypeProject     HierarchyNodeType = "project"      // enums.py:324
	HierarchyNodeTypeDatabase    HierarchyNodeType = "database"     // enums.py:327
	HierarchyNodeTypeWorkspace   HierarchyNodeType = "workspace"    // enums.py:328
	HierarchyNodeTypeSite        HierarchyNodeType = "site"         // enums.py:331
	HierarchyNodeTypeDrive       HierarchyNodeType = "drive"        // enums.py:332
	HierarchyNodeTypeChannel     HierarchyNodeType = "channel"      // enums.py:335
)

var allHierarchyNodeTypes = map[HierarchyNodeType]struct{}{
	HierarchyNodeTypeFolder:      {},
	HierarchyNodeTypeSource:      {},
	HierarchyNodeTypeSharedDrive: {},
	HierarchyNodeTypeMyDrive:     {},
	HierarchyNodeTypeSpace:       {},
	HierarchyNodeTypePage:        {},
	HierarchyNodeTypeProject:     {},
	HierarchyNodeTypeDatabase:    {},
	HierarchyNodeTypeWorkspace:   {},
	HierarchyNodeTypeSite:        {},
	HierarchyNodeTypeDrive:       {},
	HierarchyNodeTypeChannel:     {},
}

// IsValid reports whether t is a known HierarchyNodeType. Pure, no DB.
func (t HierarchyNodeType) IsValid() bool {
	_, ok := allHierarchyNodeTypes[t]
	return ok
}
