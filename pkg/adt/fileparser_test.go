package adt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseABAPFile_Class(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zcl_test.clas.abap")

	source := `CLASS zcl_test DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    METHODS test.
ENDCLASS.

CLASS zcl_test IMPLEMENTATION.
  METHOD test.
  ENDMETHOD.
ENDCLASS.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZCL_TEST" {
		t.Errorf("Expected ObjectName ZCL_TEST, got %s", info.ObjectName)
	}
	if info.ObjectType != ObjectTypeClass {
		t.Errorf("Expected ObjectType %s, got %s", ObjectTypeClass, info.ObjectType)
	}
	if !info.HasDefinition {
		t.Error("Expected HasDefinition to be true")
	}
	if !info.HasImplementation {
		t.Error("Expected HasImplementation to be true")
	}
}

func TestParseABAPFile_ClassWithDescription(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zcl_ml_iris.clas.abap")

	source := `*"* use this source file for the definition and implementation of
*"* local helper classes, interface definitions and type
*"* declarations
* Iris Flower Classification Model
CLASS zcl_ml_iris DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.
ENDCLASS.

CLASS zcl_ml_iris IMPLEMENTATION.
ENDCLASS.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZCL_ML_IRIS" {
		t.Errorf("Expected ObjectName ZCL_ML_IRIS, got %s", info.ObjectName)
	}
	if info.Description != "Iris Flower Classification Model" {
		t.Errorf("Expected description 'Iris Flower Classification Model', got %s", info.Description)
	}
}

func TestParseABAPFile_Program(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ztest_prog.prog.abap")

	source := `REPORT ztest_prog.

WRITE: / 'Hello, World!'.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZTEST_PROG" {
		t.Errorf("Expected ObjectName ZTEST_PROG, got %s", info.ObjectName)
	}
	if info.ObjectType != ObjectTypeProgram {
		t.Errorf("Expected ObjectType %s, got %s", ObjectTypeProgram, info.ObjectType)
	}
}

func TestParseABAPFile_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zif_test.intf.abap")

	source := `INTERFACE zif_test
  PUBLIC.

  METHODS test.
ENDINTERFACE.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZIF_TEST" {
		t.Errorf("Expected ObjectName ZIF_TEST, got %s", info.ObjectName)
	}
	if info.ObjectType != ObjectTypeInterface {
		t.Errorf("Expected ObjectType %s, got %s", ObjectTypeInterface, info.ObjectType)
	}
}

func TestParseABAPFile_InvalidExtension(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseABAPFile(filePath)
	if err == nil {
		t.Error("Expected error for invalid extension, got nil")
	}
}

func TestParseABAPFile_NoObjectName(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.clas.abap")

	source := `* Just a comment
* No class definition
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseABAPFile(filePath)
	if err == nil {
		t.Error("Expected error for file with no object name, got nil")
	}
}

func TestParseABAPFile_ClassWithTestClasses(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zcl_test_with_tests.clas.abap")

	source := `CLASS zcl_test_with_tests DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.
ENDCLASS.

CLASS zcl_test_with_tests IMPLEMENTATION.
ENDCLASS.

CLASS ltc_test DEFINITION FINAL FOR TESTING
  DURATION SHORT
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    METHODS test FOR TESTING.
ENDCLASS.

CLASS ltc_test IMPLEMENTATION.
  METHOD test.
  ENDMETHOD.
ENDCLASS.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if !info.HasTestClasses {
		t.Error("Expected HasTestClasses to be true")
	}
}

// TestExtractClassNameFromFilename tests the helper function for extracting
// class names from abapGit-style filenames.
func TestExtractClassNameFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"zcl_foo.clas.abap", "ZCL_FOO"},
		{"zcl_foo.clas.testclasses.abap", "ZCL_FOO"},
		{"zcl_foo.clas.locals_def.abap", "ZCL_FOO"},
		{"zcl_foo.clas.locals_imp.abap", "ZCL_FOO"},
		{"zcl_foo.clas.macros.abap", "ZCL_FOO"},
		{"ZCL_ZORK_00_IO_CONSOLE.clas.testclasses.abap", "ZCL_ZORK_00_IO_CONSOLE"},
		{"/path/to/zcl_test.clas.testclasses.abap", "ZCL_TEST"},
		{"my_class.clas.abap", "MY_CLASS"},
		{"test.prog.abap", ""}, // Not a class file
		{"random.txt", ""},     // Not a class file
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := extractClassNameFromFilename(tt.filename)
			if result != tt.expected {
				t.Errorf("extractClassNameFromFilename(%q) = %q, want %q", tt.filename, result, tt.expected)
			}
		})
	}
}

// TestParseABAPFile_TestClassesInclude tests parsing of .clas.testclasses.abap files.
// The class name should come from the FILENAME, not from local test classes in the content.
func TestParseABAPFile_TestClassesInclude(t *testing.T) {
	tmpDir := t.TempDir()
	// Filename: zcl_zork_00_io_console.clas.testclasses.abap
	// Parent class should be: ZCL_ZORK_00_IO_CONSOLE
	filePath := filepath.Join(tmpDir, "zcl_zork_00_io_console.clas.testclasses.abap")

	// Content contains local test class LTCL_IO_CONSOLE_TEST - should NOT be used as object name
	source := `*"* use this source file for your ABAP unit test classes
CLASS ltcl_io_console_test DEFINITION FINAL FOR TESTING
  DURATION SHORT
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    DATA mo_cut TYPE REF TO zcl_zork_00_io_console.
    METHODS test_read FOR TESTING.
    METHODS test_write FOR TESTING.
ENDCLASS.

CLASS ltcl_io_console_test IMPLEMENTATION.
  METHOD test_read.
    " Test implementation
  ENDMETHOD.
  METHOD test_write.
    " Test implementation
  ENDMETHOD.
ENDCLASS.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	// Object name should be from filename, NOT from content
	if info.ObjectName != "ZCL_ZORK_00_IO_CONSOLE" {
		t.Errorf("Expected ObjectName ZCL_ZORK_00_IO_CONSOLE (from filename), got %s", info.ObjectName)
	}
	if info.ObjectType != ObjectTypeClass {
		t.Errorf("Expected ObjectType %s, got %s", ObjectTypeClass, info.ObjectType)
	}
	if info.ClassIncludeType != ClassIncludeTestClasses {
		t.Errorf("Expected ClassIncludeType %s, got %s", ClassIncludeTestClasses, info.ClassIncludeType)
	}
	if !info.HasTestClasses {
		t.Error("Expected HasTestClasses to be true")
	}
}

// TestParseABAPFile_LocalsDefInclude tests parsing of .clas.locals_def.abap files.
func TestParseABAPFile_LocalsDefInclude(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zcl_my_class.clas.locals_def.abap")

	source := `*"* use this source file for any type of declarations
INTERFACE lif_helper.
  METHODS do_something.
ENDINTERFACE.

CLASS lcl_helper DEFINITION.
  PUBLIC SECTION.
    INTERFACES lif_helper.
ENDCLASS.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZCL_MY_CLASS" {
		t.Errorf("Expected ObjectName ZCL_MY_CLASS, got %s", info.ObjectName)
	}
	if info.ClassIncludeType != ClassIncludeDefinitions {
		t.Errorf("Expected ClassIncludeType %s, got %s", ClassIncludeDefinitions, info.ClassIncludeType)
	}
}

// TestParseABAPFile_LocalsImpInclude tests parsing of .clas.locals_imp.abap files.
func TestParseABAPFile_LocalsImpInclude(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zcl_my_class.clas.locals_imp.abap")

	source := `*"* use this source file for implementation of local classes
CLASS lcl_helper IMPLEMENTATION.
  METHOD lif_helper~do_something.
    " Implementation
  ENDMETHOD.
ENDCLASS.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZCL_MY_CLASS" {
		t.Errorf("Expected ObjectName ZCL_MY_CLASS, got %s", info.ObjectName)
	}
	if info.ClassIncludeType != ClassIncludeImplementations {
		t.Errorf("Expected ClassIncludeType %s, got %s", ClassIncludeImplementations, info.ClassIncludeType)
	}
}

// TestParseABAPFile_MacrosInclude tests parsing of .clas.macros.abap files.
func TestParseABAPFile_MacrosInclude(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "zcl_my_class.clas.macros.abap")

	source := `*"* use this source file for any macro definitions
DEFINE my_macro.
  WRITE: / &1.
END-OF-DEFINITION.
`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseABAPFile(filePath)
	if err != nil {
		t.Fatalf("ParseABAPFile failed: %v", err)
	}

	if info.ObjectName != "ZCL_MY_CLASS" {
		t.Errorf("Expected ObjectName ZCL_MY_CLASS, got %s", info.ObjectName)
	}
	if info.ClassIncludeType != ClassIncludeMacros {
		t.Errorf("Expected ClassIncludeType %s, got %s", ClassIncludeMacros, info.ClassIncludeType)
	}
}

func TestParseABAPFile_HeaderTemplateIsNotADescription(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zdemo_xfer.prog.abap")
	src := "*&---------------------------------------------------------------------*\n*& Report ZDEMO_XFER\n*&---------------------------------------------------------------------*\n*& DPL snapshot transfer: download / upload / transplant\n*&---------------------------------------------------------------------*\nREPORT zdemo_xfer.\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := ParseABAPFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Description != "DPL snapshot transfer: download / upload / transplant" {
		t.Errorf("description %q", info.Description)
	}
}
