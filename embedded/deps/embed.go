// Package deps provides embedded dependency packages (abapGit, etc.) for deployment.
package deps

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Embedded dependency ZIPs (placeholders - replace with actual ZIPs)
// To generate: vsp git-export --packages 'ZABAPGIT' --output abapgit-standalone.zip
// Or download from GitHub: https://github.com/abapGit/abapGit

// Placeholder: will be replaced with actual ZIP when available
// //go:embed abapgit-standalone.zip
// var AbapGitStandalone []byte

// //go:embed abapgit-dev.zip
// var AbapGitDev []byte

// DependencyInfo describes an available dependency package.
type DependencyInfo struct {
	Name        string // e.g., "abapgit-standalone", "abapgit-dev"
	Description string
	Package     string   // Target SAP package
	Available   bool     // Whether ZIP is embedded
	FileCount   int      // Number of files in ZIP
	Objects     []string // Object names (populated on load)
}

// GetAvailableDependencies returns list of embedded dependencies.
func GetAvailableDependencies() []DependencyInfo {
	return []DependencyInfo{
		{
			Name:        "abapgit-standalone",
			Description: "abapGit standalone program (single file ZABAPGIT)",
			Package:     "$ABAPGIT",
			Available:   false, // TODO: Set to true when ZIP is embedded
		},
		{
			Name:        "abapgit-dev",
			Description: "abapGit developer edition (full $ZGIT_DEV* packages)",
			Package:     "$ZGIT_DEV",
			Available:   false, // TODO: Set to true when ZIP is embedded
		},
	}
}

// GetDependencyZIP returns embedded ZIP bytes by dependency name.
// Current repository state ships placeholders only, so this returns nil until ZIPs are embedded.
func GetDependencyZIP(name string) []byte {
	// Currently no ZIPs are embedded; return nil for all lookups.
	_ = name
	return nil
}

// ABAPFile represents a parsed ABAP source file from abapGit ZIP.
type ABAPFile struct {
	// File info
	Path     string // Original path in ZIP (e.g., "src/zcl_example.clas.abap")
	Filename string // Just filename (e.g., "zcl_example.clas.abap")

	// Parsed info
	ObjectType  string // CLAS, INTF, PROG, FUGR, FUNC, DDLS, BDEF, SRVD, etc.
	ObjectName  string // ZCL_EXAMPLE, ZIF_EXAMPLE, etc.
	IncludeType string // For classes: "", "locals_def", "locals_imp", "testclasses", "macros"
	IsXML       bool   // True for .xml metadata files

	// Content
	Content string
}

// DeploymentOrder returns files sorted by deployment order.
// Interfaces first, then classes (with includes grouped), then others.
func DeploymentOrder(files []ABAPFile) []ABAPFile {
	// Priority order for object types
	typePriority := map[string]int{
		"INTF": 1,  // Interfaces first (no dependencies)
		"DOMA": 2,  // Domains
		"DTEL": 3,  // Data elements
		"TABL": 4,  // Tables/structures
		"DDLS": 5,  // CDS views
		"CLAS": 6,  // Classes (depend on interfaces)
		"PROG": 7,  // Programs
		"FUGR": 8,  // Function groups
		"FUNC": 9,  // Function modules
		"BDEF": 10, // Behavior definitions
		"SRVD": 11, // Service definitions
		"SRVB": 12, // Service bindings
	}

	// Include priority within a class
	includePriority := map[string]int{
		"":            1, // Main source first
		"locals_def":  2, // Local definitions
		"locals_imp":  3, // Local implementations
		"macros":      4, // Macros
		"testclasses": 5, // Test classes last
	}

	sorted := make([]ABAPFile, len(files))
	copy(sorted, files)

	sort.SliceStable(sorted, func(i, j int) bool {
		fi, fj := sorted[i], sorted[j]

		// XML files go with their source
		if fi.IsXML != fj.IsXML {
			return !fi.IsXML // Source before XML
		}

		// Sort by object type priority
		pi := typePriority[fi.ObjectType]
		pj := typePriority[fj.ObjectType]
		if pi == 0 {
			pi = 99
		}
		if pj == 0 {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}

		// Same type - sort by name
		if fi.ObjectName != fj.ObjectName {
			return fi.ObjectName < fj.ObjectName
		}

		// Same object - sort by include type
		ii := includePriority[fi.IncludeType]
		ij := includePriority[fj.IncludeType]
		return ii < ij
	})

	return sorted
}

// ParseAbapGitFilename extracts object info from abapGit filename.
// Examples:
//
//	zcl_example.clas.abap          → CLAS, ZCL_EXAMPLE, ""
//	zcl_example.clas.locals_def.abap → CLAS, ZCL_EXAMPLE, "locals_def"
//	zcl_example.clas.testclasses.abap → CLAS, ZCL_EXAMPLE, "testclasses"
//	zif_example.intf.abap          → INTF, ZIF_EXAMPLE, ""
//	zexample.prog.abap             → PROG, ZEXAMPLE, ""
//	zexample.fugr.abap             → FUGR, ZEXAMPLE, ""
//	zexample.ddls.asddls           → DDLS, ZEXAMPLE, ""
//	zcl_example.clas.xml           → CLAS, ZCL_EXAMPLE, "" (XML metadata)
func ParseAbapGitFilename(filename string) (objectType, objectName, includeType string, isXML bool) {
	// Remove directory path
	filename = filepath.Base(filename)

	// Check for XML
	isXML = strings.HasSuffix(filename, ".xml")
	if isXML {
		filename = strings.TrimSuffix(filename, ".xml")
	} else {
		// Remove source extension
		for _, ext := range []string{".abap", ".asddls", ".asbdef", ".srvdsrv"} {
			if strings.HasSuffix(filename, ext) {
				filename = strings.TrimSuffix(filename, ext)
				break
			}
		}
	}

	// Split by dots
	parts := strings.Split(filename, ".")
	if len(parts) < 2 {
		return "", "", "", isXML
	}

	objectName = strings.ToUpper(parts[0])

	// Map abapGit type suffix to SAP type
	typeMap := map[string]string{
		"clas": "CLAS",
		"intf": "INTF",
		"prog": "PROG",
		"fugr": "FUGR",
		"func": "FUNC",
		"ddls": "DDLS",
		"doma": "DOMA",
		"dtel": "DTEL",
		"tabl": "TABL",
		"view": "VIEW",
		"bdef": "BDEF",
		"srvd": "SRVD",
		"srvb": "SRVB",
		"ttyp": "TTYP",
		"msag": "MSAG",
		"enqu": "ENQU",
		"shlp": "SHLP",
		"tran": "TRAN",
		"devc": "DEVC",
	}

	if len(parts) >= 2 {
		if t, ok := typeMap[parts[1]]; ok {
			objectType = t
		}
	}

	// Check for include type (locals_def, locals_imp, testclasses, macros)
	if len(parts) >= 3 {
		switch parts[2] {
		case "locals_def", "locals_imp", "testclasses", "macros":
			includeType = parts[2]
		}
	}

	return objectType, objectName, includeType, isXML
}

// UnzipInMemory extracts all files from a ZIP archive in memory.
func UnzipInMemory(zipData []byte) ([]ABAPFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP: %w", err)
	}

	var files []ABAPFile

	for _, f := range reader.File {
		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		// Skip non-ABAP files
		filename := filepath.Base(f.Name)
		if !isAbapGitFile(filename) {
			continue
		}

		// Read file content
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		// Parse filename
		objectType, objectName, includeType, isXML := ParseAbapGitFilename(filename)

		files = append(files, ABAPFile{
			Path:        f.Name,
			Filename:    filename,
			ObjectType:  objectType,
			ObjectName:  objectName,
			IncludeType: includeType,
			IsXML:       isXML,
			Content:     string(content),
		})
	}

	return files, nil
}

// isAbapGitFile checks if filename is an abapGit source or metadata file.
func isAbapGitFile(filename string) bool {
	extensions := []string{
		".abap",
		".asddls",
		".asbdef",
		".srvdsrv",
		".xml",
	}

	for _, ext := range extensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

// GroupByObject groups files by object name for deployment.
// Returns map of objectName -> []ABAPFile (main + includes + xml).
func GroupByObject(files []ABAPFile) map[string][]ABAPFile {
	groups := make(map[string][]ABAPFile)
	for _, f := range files {
		key := f.ObjectType + "/" + f.ObjectName
		groups[key] = append(groups[key], f)
	}
	return groups
}

// ExtractDescription extracts object description from XML metadata.
func ExtractDescription(xmlContent string) string {
	// Simple regex to extract description
	// <DESCRIPT>Some description</DESCRIPT>
	re := regexp.MustCompile(`<DESCRIPT>([^<]+)</DESCRIPT>`)
	matches := re.FindStringSubmatch(xmlContent)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// DeploymentPlan represents the plan for deploying a dependency.
type DeploymentPlan struct {
	Dependency   string
	Package      string
	TotalFiles   int
	TotalObjects int
	Objects      []DeploymentObject
}

// DeploymentObject represents a single object to deploy.
type DeploymentObject struct {
	Type        string
	Name        string
	Description string
	MainSource  string
	Includes    map[string]string // includeType -> source
	XMLMetadata string
}

// CreateDeploymentPlan creates a deployment plan from parsed files.
func CreateDeploymentPlan(depName, packageName string, files []ABAPFile) *DeploymentPlan {
	sorted := DeploymentOrder(files)
	groups := GroupByObject(sorted)

	plan := &DeploymentPlan{
		Dependency:   depName,
		Package:      packageName,
		TotalFiles:   len(files),
		TotalObjects: len(groups),
	}

	// Track which objects we've added
	added := make(map[string]bool)

	for _, f := range sorted {
		key := f.ObjectType + "/" + f.ObjectName
		if added[key] {
			continue
		}
		added[key] = true

		objFiles := groups[key]
		obj := DeploymentObject{
			Type:     f.ObjectType,
			Name:     f.ObjectName,
			Includes: make(map[string]string),
		}

		for _, of := range objFiles {
			if of.IsXML {
				obj.XMLMetadata = of.Content
				obj.Description = ExtractDescription(of.Content)
			} else if of.IncludeType == "" {
				obj.MainSource = of.Content
			} else {
				obj.Includes[of.IncludeType] = of.Content
			}
		}

		plan.Objects = append(plan.Objects, obj)
	}

	return plan
}
