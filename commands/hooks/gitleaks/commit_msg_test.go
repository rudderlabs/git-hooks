package gitleaks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCommitMsgHook tests the commit-msg.sh hook for conventional commit compliance
func TestCommitMsgHook(t *testing.T) {
	// Setup: Create mock gitleaks binary
	tempDir := t.TempDir()
	mockGitleaksPath := filepath.Join(tempDir, "gitleaks")
	createMockGitleaks(t, mockGitleaksPath)

	// Create test version of commit-msg.sh with mock gitleaks path
	scriptPath := createTestScript(t, tempDir, mockGitleaksPath)

	tests := []struct {
		name           string
		inputMessage   string
		expectedOutput string
		description    string
	}{
		{
			name:         "simple header only",
			inputMessage: "feat: add new feature",
			expectedOutput: `feat: add new feature

Scanned-by: gitleaks v8.18.0`,
			description: "Should add blank line separator and footer for header-only commits",
		},
		{
			name: "header with body",
			inputMessage: `feat: add new feature

This is a detailed explanation of the feature.`,
			expectedOutput: `feat: add new feature

This is a detailed explanation of the feature.

Scanned-by: gitleaks v8.18.0`,
			description: "Should add blank line separator and footer after body",
		},
		{
			name: "header with existing footer",
			inputMessage: `feat: add new feature

Fixes: #123`,
			expectedOutput: `feat: add new feature

Fixes: #123
Scanned-by: gitleaks v8.18.0`,
			description: "Should append footer directly without extra blank line",
		},
		{
			name: "header body and footer",
			inputMessage: `feat: add new feature

This is a detailed explanation.

Closes: #456`,
			expectedOutput: `feat: add new feature

This is a detailed explanation.

Closes: #456
Scanned-by: gitleaks v8.18.0`,
			description: "Should append to existing footer section",
		},
		{
			name: "header and footer, release-please scenario",
			inputMessage: `feat: add new feature

Release-as: 0.25.1-alpha.1`,
			expectedOutput: `feat: add new feature

Release-as: 0.25.1-alpha.1
Scanned-by: gitleaks v8.18.0`,
			description: "Should append to existing footer section",
		},
		{
			name: "multiple existing footers",
			inputMessage: `fix: resolve bug

Fixes: #123
Refs: #456`,
			expectedOutput: `fix: resolve bug

Fixes: #123
Refs: #456
Scanned-by: gitleaks v8.18.0`,
			description: "Should append to footer group without blank line",
		},
		{
			name: "breaking change footer",
			inputMessage: `feat!: major change

BREAKING CHANGE: This changes the API`,
			expectedOutput: `feat!: major change

BREAKING CHANGE: This changes the API
Scanned-by: gitleaks v8.18.0`,
			description: "Should handle BREAKING CHANGE footer",
		},
		{
			name: "issue reference format",
			inputMessage: `fix: resolve issue

Fixes #789`,
			expectedOutput: `fix: resolve issue

Fixes #789
Scanned-by: gitleaks v8.18.0`,
			description: "Should handle issue reference format (Key #number)",
		},
		{
			name: "co-authored-by footer",
			inputMessage: `docs: update readme

Co-authored-by: John Smith <john@example.com>`,
			expectedOutput: `docs: update readme

Co-authored-by: John Smith <john@example.com>
Scanned-by: gitleaks v8.18.0`,
			description: "Should handle Co-authored-by footer",
		},
		{
			name: "signed-off-by footer",
			inputMessage: `chore: update dependencies

Signed-off-by: Developer <dev@example.com>`,
			expectedOutput: `chore: update dependencies

Signed-off-by: Developer <dev@example.com>
Scanned-by: gitleaks v8.18.0`,
			description: "Should handle Signed-off-by footer",
		},
		{
			name: "complex commit with multiple sections",
			inputMessage: `feat(api): add new endpoint

This adds a new REST API endpoint for users.

Details:
- Supports GET and POST
- Returns JSON
- Rate limited

Implements: #100
Co-authored-by: Jane Doe <jane@example.com>`,
			expectedOutput: `feat(api): add new endpoint

This adds a new REST API endpoint for users.

Details:
- Supports GET and POST
- Returns JSON
- Rate limited

Implements: #100
Co-authored-by: Jane Doe <jane@example.com>
Scanned-by: gitleaks v8.18.0`,
			description: "Should handle complex multi-section commits",
		},
		{
			name: "multi-line footer value for key",
			inputMessage: `feat(api): add new endpoint

This adds a new REST API endpoint for users.

Details:
- Supports GET and POST
- Returns JSON
- Rate limited

Implements: #100
Co-authored-by: Jane Doe <jane@example.com>
BREAKING CHANGE: this is a breaking change
  with multiple lines
  of description`,
			expectedOutput: `feat(api): add new endpoint

This adds a new REST API endpoint for users.

Details:
- Supports GET and POST
- Returns JSON
- Rate limited

Implements: #100
Co-authored-by: Jane Doe <jane@example.com>
BREAKING CHANGE: this is a breaking change
  with multiple lines
  of description
Scanned-by: gitleaks v8.18.0`,
			description: "Should handle multi-line footer value for key",
		},
		{
			name: "body text with colon not treated as footer",
			inputMessage: `feat: add feature

This is a sentence: with a colon in the middle.
Another line: with another colon here.`,
			expectedOutput: `feat: add feature

This is a sentence: with a colon in the middle.
Another line: with another colon here.

Scanned-by: gitleaks v8.18.0`,
			description: "Should not treat body text with spaces before colon as footer",
		},
		{
			name: "body ending with invalid footer key",
			inputMessage: `fix: resolve issue

The problem was: memory leak in component`,
			expectedOutput: `fix: resolve issue

The problem was: memory leak in component

Scanned-by: gitleaks v8.18.0`,
			description: "Should reject footer keys with spaces (except BREAKING CHANGE)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.description)

			// Create temporary commit message file
			msgFile := filepath.Join(tempDir, "commit_msg_"+strings.ReplaceAll(tt.name, " ", "_")+".txt")
			err := os.WriteFile(msgFile, []byte(tt.inputMessage), 0o644)
			require.NoError(t, err, "Failed to create commit message file")

			// Run the commit-msg hook
			cmd := exec.Command(scriptPath, msgFile)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "Hook script failed: %s", string(output))

			// Read the modified commit message
			actualContent, err := os.ReadFile(msgFile)
			require.NoError(t, err, "Failed to read modified commit message")
			actual := string(actualContent)

			// Assert the output matches expected
			require.Equal(t, tt.expectedOutput, actual, "Commit message format incorrect")

			// Clean up
			require.NoError(t, os.Remove(msgFile))
		})
	}
}

// TestCommitMsgHook_Idempotent verifies the hook doesn't add duplicate footers
func TestCommitMsgHook_IdempotentExecution(t *testing.T) {
	t.Log("Testing that running hook multiple times doesn't add duplicate footers")

	// Setup
	tempDir := t.TempDir()
	mockGitleaksPath := filepath.Join(tempDir, "gitleaks")
	createMockGitleaks(t, mockGitleaksPath)
	scriptPath := createTestScript(t, tempDir, mockGitleaksPath)

	msgFile := filepath.Join(tempDir, "commit_msg.txt")
	initialMessage := "feat: add feature"
	expectedAfterFirstRun := `feat: add feature

Scanned-by: gitleaks v8.18.0`

	// Write initial message
	err := os.WriteFile(msgFile, []byte(initialMessage), 0o644)
	require.NoError(t, err)

	// First run
	cmd := exec.Command(scriptPath, msgFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "First run failed: %s", string(output))

	content, err := os.ReadFile(msgFile)
	require.NoError(t, err)
	require.Equal(t, expectedAfterFirstRun, string(content))

	// Second run - should not add another footer
	cmd = exec.Command(scriptPath, msgFile)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Second run failed: %s", string(output))

	content, err = os.ReadFile(msgFile)
	require.NoError(t, err)
	require.Equal(t, expectedAfterFirstRun, string(content), "Should not add duplicate footer")

	// Verify the output message indicates skipping
	require.Contains(t, string(output), "already present", "Should output skip message")
}

// TestCommitMsgHook_NoBlankLineBetweenFooters ensures footers are consecutive
func TestCommitMsgHook_NoBlankLineBetweenFooters(t *testing.T) {
	t.Log("Testing that no blank line exists between existing footer and new footer")

	// Setup
	tempDir := t.TempDir()
	mockGitleaksPath := filepath.Join(tempDir, "gitleaks")
	createMockGitleaks(t, mockGitleaksPath)
	scriptPath := createTestScript(t, tempDir, mockGitleaksPath)

	msgFile := filepath.Join(tempDir, "commit_msg.txt")
	inputMessage := `feat: add feature

Fixes: #123`

	err := os.WriteFile(msgFile, []byte(inputMessage), 0o644)
	require.NoError(t, err)

	// Run hook
	cmd := exec.Command(scriptPath, msgFile)
	_, err = cmd.CombinedOutput()
	require.NoError(t, err)

	// Read result
	content, err := os.ReadFile(msgFile)
	require.NoError(t, err)
	actual := string(content)

	// Verify no double blank line between footers
	require.NotContains(t, actual, "Fixes: #123\n\nScanned-by:", "Should not have blank line between footers")
	require.Contains(t, actual, "Fixes: #123\nScanned-by:", "Should have footers on consecutive lines")
}

// TestCommitMsgHook_BlankLineSeparatorForHeaderOnly ensures proper separator
func TestCommitMsgHook_BlankLineSeparatorForHeaderOnly(t *testing.T) {
	t.Log("Testing that exactly one blank line separates header from footer")

	// Setup
	tempDir := t.TempDir()
	mockGitleaksPath := filepath.Join(tempDir, "gitleaks")
	createMockGitleaks(t, mockGitleaksPath)
	scriptPath := createTestScript(t, tempDir, mockGitleaksPath)

	msgFile := filepath.Join(tempDir, "commit_msg.txt")
	inputMessage := "feat: add feature"

	err := os.WriteFile(msgFile, []byte(inputMessage), 0o644)
	require.NoError(t, err)

	// Run hook
	cmd := exec.Command(scriptPath, msgFile)
	_, err = cmd.CombinedOutput()
	require.NoError(t, err)

	// Read result
	content, err := os.ReadFile(msgFile)
	require.NoError(t, err)
	actual := string(content)

	// Verify exactly one blank line
	require.Contains(t, actual, "feat: add feature\n\nScanned-by:", "Should have exactly one blank line")
	require.NotContains(t, actual, "feat: add feature\n\n\nScanned-by:", "Should not have multiple blank lines")
}

// TestCommitMsgHook_GitleaksVersionFailure tests fallback when version command fails
func TestCommitMsgHook_GitleaksVersionFailure(t *testing.T) {
	t.Log("Testing graceful handling when gitleaks version command fails")

	// Setup with failing mock
	tempDir := t.TempDir()
	mockGitleaksPath := filepath.Join(tempDir, "gitleaks-fail")
	createFailingMockGitleaks(t, mockGitleaksPath)
	scriptPath := createTestScript(t, tempDir, mockGitleaksPath)

	msgFile := filepath.Join(tempDir, "commit_msg.txt")
	inputMessage := "feat: add feature"

	err := os.WriteFile(msgFile, []byte(inputMessage), 0o644)
	require.NoError(t, err)

	// Run hook
	cmd := exec.Command(scriptPath, msgFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Hook should not fail even if gitleaks version fails")

	// Verify warning is shown
	require.Contains(t, string(output), "Warning: Failed to get gitleaks version")

	// Read result - should still add footer without version
	content, err := os.ReadFile(msgFile)
	require.NoError(t, err)
	actual := string(content)

	expectedOutput := `feat: add feature

Scanned-by: gitleaks`
	require.Equal(t, expectedOutput, actual, "Should add footer without version on failure")
}

// Helper functions

// createMockGitleaks creates a mock gitleaks binary that returns a version
func createMockGitleaks(t *testing.T, path string) {
	t.Helper()

	mockScript := `#!/bin/sh
echo "v8.18.0"
`
	err := os.WriteFile(path, []byte(mockScript), 0o755)
	require.NoError(t, err, "Failed to create mock gitleaks")
}

// createFailingMockGitleaks creates a mock gitleaks that fails
func createFailingMockGitleaks(t *testing.T, path string) {
	t.Helper()

	mockScript := `#!/bin/sh
exit 1
`
	err := os.WriteFile(path, []byte(mockScript), 0o755)
	require.NoError(t, err, "Failed to create failing mock gitleaks")
}

// createTestScript creates a test version of commit-msg.sh with gitleaks path replaced
func createTestScript(t *testing.T, tempDir, gitleaksPath string) string {
	t.Helper()

	// Read the original script
	originalScript := filepath.Join("..", "..", "..", "commands", "hooks", "gitleaks", "commit-msg.sh")
	content, err := os.ReadFile(originalScript)
	require.NoError(t, err, "Failed to read original commit-msg.sh")

	// Replace the template with actual path
	modifiedContent := strings.ReplaceAll(string(content), "{{.GitleaksPath}}", gitleaksPath)

	// Write test script
	testScriptPath := filepath.Join(tempDir, "commit-msg-test.sh")
	err = os.WriteFile(testScriptPath, []byte(modifiedContent), 0o755)
	require.NoError(t, err, "Failed to create test script")

	return testScriptPath
}
