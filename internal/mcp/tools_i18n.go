// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_i18n.go registers translation (i18n) tools.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// registerI18NTools registers i18n/translation tools.
func (s *Server) registerI18NTools(shouldRegister func(string) bool) {
	if shouldRegister("GetObjectTextsInLanguage") {
		s.mcpServer.AddTool(mcp.NewTool("GetObjectTextsInLanguage",
			mcp.WithDescription("Get the source/content of an ABAP object in a specific language. The language parameter overrides the session language for this request only, allowing you to read texts in any installed language."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT source URL (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR, ES, JA)"),
			),
		), s.handleGetObjectTextsInLanguage)
	}

	if shouldRegister("GetDataElementLabels") {
		s.mcpServer.AddTool(mcp.NewTool("GetDataElementLabels",
			mcp.WithDescription("Get the text labels (short/medium/long/heading) of a data element in a specific language. Useful for checking translations of DDIC texts."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Data element name (e.g., MATNR, ZTEST_DTEL)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
		), s.handleGetDataElementLabels)
	}

	if shouldRegister("GetMessageClassTexts") {
		s.mcpServer.AddTool(mcp.NewTool("GetMessageClassTexts",
			mcp.WithDescription("Get all messages of a message class in a specific language. Returns message number and text for each entry."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Message class name (e.g., ZTEST_MC)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
		), s.handleGetMessageClassTexts)
	}

	if shouldRegister("WriteMessageClassTexts") {
		s.mcpServer.AddTool(mcp.NewTool("WriteMessageClassTexts",
			mcp.WithDescription("Update message class texts in a specific language. Takes and releases its own lock unless a lock handle from LockObject is given. Use for translating message class entries."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Message class name"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
			mcp.WithString("lock_handle",
				mcp.Description("Optional lock handle. Omit it and this call takes and releases its own lock (#169)."),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for $TMP objects)"),
			),
			mcp.WithArray("texts",
				mcp.Required(),
				mcp.Description(`Messages to write: [{"number":"001","text":"Order & created"}]`),
				mcp.Items(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"number": map[string]any{"type": "string", "description": "Three-digit message number"},
						"text":   map[string]any{"type": "string", "description": "Message text for this language"},
					},
					"required": []string{"number", "text"},
				}),
			),
		), s.handleWriteMessageClassTexts)
	}

	if shouldRegister("GetTextPool") {
		s.mcpServer.AddTool(mcp.NewTool("GetTextPool",
			mcp.WithDescription("Get the text pool (text elements/symbols) of a program in a specific language. Returns ID, key, and text for each entry."),
			mcp.WithString("program_name",
				mcp.Required(),
				mcp.Description("Program name (e.g., ZTEST)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
		), s.handleGetTextPoolInLanguage)
	}

	if shouldRegister("CompareLanguages") {
		s.mcpServer.AddTool(mcp.NewTool("CompareLanguages",
			mcp.WithDescription("Compare the text content of an ABAP object in two languages. Shows which lines differ or are missing in the target language. Useful for identifying untranslated or outdated translations."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT source URL (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
			),
			mcp.WithString("source_language",
				mcp.Required(),
				mcp.Description("Source language code (e.g., EN)"),
			),
			mcp.WithString("target_language",
				mcp.Required(),
				mcp.Description("Target language code (e.g., DE, FR)"),
			),
		), s.handleCompareObjectLanguages)
	}
}
