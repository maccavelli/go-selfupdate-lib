package selfupdate

// resultDocumentSchema is the schema_version of ResultDocument. Version 2
// adds service_started and warnings (0010-PLAN-v1-6-0 S2 and S3); version 3
// adds rolled_back and probes_skipped (0015-MADR G3, C9).
const resultDocumentSchema = 3

// ResultDocument is the JSON form of a Result, with stable snake_case keys
// and Operation as a string. The enums keep their own JSON encoding; this
// document is the string form (0004-MADR §3, "Structured output").
type ResultDocument struct {
	SchemaVersion     int      `json:"schema_version"`
	Product           string   `json:"product"`
	CurrentVersion    string   `json:"current_version"`
	TargetVersion     string   `json:"target_version,omitempty"`
	ReleaseURL        string   `json:"release_url,omitempty"`
	AssetName         string   `json:"asset_name,omitempty"`
	Operation         string   `json:"operation"`
	Checked           bool     `json:"checked"`
	Applied           bool     `json:"applied"`
	Declined          bool     `json:"declined"`
	DryRun            bool     `json:"dry_run"`
	ReleaseDigest     string   `json:"release_digest,omitempty"`
	InstalledDigest   string   `json:"installed_digest,omitempty"`
	ServiceInstalled  bool     `json:"service_installed"`
	ServiceWasRunning bool     `json:"service_was_running"`
	ServiceStarted    bool     `json:"service_started"`
	PendingBackup     string   `json:"pending_backup,omitempty"`
	Previous          string   `json:"previous,omitempty"`
	Warnings          Warnings `json:"warnings,omitempty"`
	RolledBack        bool     `json:"rolled_back"`
	ProbesSkipped     bool     `json:"probes_skipped"`
}

// Document returns the JSON form of r. It has no side effects.
func (r Result) Document() ResultDocument {
	return ResultDocument{
		SchemaVersion:     resultDocumentSchema,
		Product:           r.Product,
		CurrentVersion:    r.CurrentVersion,
		TargetVersion:     r.TargetVersion,
		ReleaseURL:        r.ReleaseURL,
		AssetName:         r.AssetName,
		Operation:         r.Operation.String(),
		Checked:           r.Checked,
		Applied:           r.Applied,
		Declined:          r.Declined,
		DryRun:            r.DryRun,
		ReleaseDigest:     r.ReleaseDigest,
		InstalledDigest:   r.InstalledDigest,
		ServiceInstalled:  r.ServiceInstalled,
		ServiceWasRunning: r.ServiceWasRunning,
		ServiceStarted:    r.ServiceStarted,
		PendingBackup:     r.PendingBackup,
		Previous:          r.Previous,
		Warnings:          r.Warnings,
		RolledBack:        r.RolledBack,
		ProbesSkipped:     r.ProbesSkipped,
	}
}
