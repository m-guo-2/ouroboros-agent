package sandbox

import "strings"

const (
	// OfficeWorkerTemplateID is the first production sandbox category:
	// a white-collar office suite for documents, spreadsheets, slides, and PDFs.
	OfficeWorkerTemplateID = "office-worker"
)

// SandboxTemplate declares the expected preinstalled capability set for a sandbox.
// It is intentionally small: the implementation still lives in the runtime files
// and deployment script, while this type gives sessions a stable capability identity.
type SandboxTemplate struct {
	ID               string   `json:"id"`
	Category         string   `json:"category"`
	DisplayName      string   `json:"display_name"`
	Description      string   `json:"description"`
	RuntimeCommands  []string `json:"runtime_commands"`
	PythonPackages   []string `json:"python_packages"`
	SystemBinaries   []string `json:"system_binaries"`
	VerificationHint string   `json:"verification_hint"`
}

var officeWorkerTemplate = SandboxTemplate{
	ID:          OfficeWorkerTemplateID,
	Category:    "white-collar-office-suite",
	DisplayName: "White-Collar Office Suite",
	Description: "Preinstalled office environment for Word, Excel, PowerPoint-oriented files, PDF export, and layout verification.",
	RuntimeCommands: []string{
		"office",
	},
	PythonPackages: []string{
		"python-docx",
		"openpyxl",
		"pandas",
		"XlsxWriter",
		"xlrd",
		"mammoth",
		"lxml",
		"Pillow",
	},
	SystemBinaries: []string{
		"soffice",
		"fc-match",
	},
	VerificationHint: "office capabilities && office fonts && office self-test --work-dir output/office-self-test",
}

func DefaultSandboxTemplate() SandboxTemplate {
	return officeWorkerTemplate
}

func ResolveSandboxTemplate(id string) (SandboxTemplate, bool) {
	switch strings.TrimSpace(id) {
	case "", OfficeWorkerTemplateID:
		return officeWorkerTemplate, true
	default:
		return SandboxTemplate{}, false
	}
}

func SandboxTemplates() []SandboxTemplate {
	return []SandboxTemplate{officeWorkerTemplate}
}
