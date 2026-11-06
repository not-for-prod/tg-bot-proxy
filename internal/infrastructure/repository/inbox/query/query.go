package query

import _ "embed"

//go:embed claim_inbox_processor.sql
var ClaimInboxProcessor string

//go:embed claim_ingestion.sql
var ClaimIngestion string

//go:embed create_inbox_items.sql
var CreateInboxItems string
