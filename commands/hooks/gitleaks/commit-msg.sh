#!/bin/sh

# Commit-msg hook to append gitleaks version info in conventional commit footer format

# Gitleaks path (detected during installation)
GITLEAKS_PATH="{{.GitleaksPath}}"

COMMIT_MSG_FILE=$1

GITLEAKS_FOOTER_KEY="Scanned-by"

# Append gitleaks scan info to commit message
if [ -n "$COMMIT_MSG_FILE" ]; then
    # Check if the commit message already contains gitleaks scan info
    if grep -q "^$GITLEAKS_FOOTER_KEY:" "$COMMIT_MSG_FILE"; then
        echo "Gitleaks scan info already present in commit message, skipping"
    else
        # Get gitleaks version with error handling
        GITLEAKS_VERSION=$($GITLEAKS_PATH version 2>/dev/null | head -n1)
        if [ $? -eq 0 ] && [ -n "$GITLEAKS_VERSION" ]; then
            GITLEAKS_FOOTER="$GITLEAKS_FOOTER_KEY: gitleaks $GITLEAKS_VERSION"
        else
            echo "Warning: Failed to get gitleaks version, appending scan info without version" >&2
            GITLEAKS_FOOTER="$GITLEAKS_FOOTER_KEY: gitleaks"
        fi

        # Detect if commit message already has a footer section
        # Footer is identified by lines matching "Key: value" or "Key #value" format
        # Footer values can span multiple lines (RFC 822 folding) with continuation lines starting with whitespace
        HAS_FOOTER=false
        
        # Read the file, skip comments (lines starting with #), find last non-empty line
        LAST_NON_EMPTY=$(grep -v '^#' "$COMMIT_MSG_FILE" | grep -v '^$' | tail -n1)
        
        # Count total non-empty, non-comment lines
        LINE_COUNT=$(grep -v '^#' "$COMMIT_MSG_FILE" | grep -v '^$' | wc -l | tr -d ' ')
        
        # Check if last line matches footer format OR is a continuation line (starts with whitespace)
        # Footer patterns:
        #   - "Key: value" where Key is letters/digits/hyphens (no spaces)
        #   - "BREAKING CHANGE: value" (special case with space)
        #   - "Key #number" for issue references
        # Continuation lines: start with space or tab (multi-line footer values)
        # BUT: If there's only 1 line, it's the header, not a footer
        if [ "$LINE_COUNT" -gt 1 ] && { echo "$LAST_NON_EMPTY" | grep -qE '^([A-Za-z][-A-Za-z0-9]*|BREAKING CHANGE): .+|^[A-Za-z][-A-Za-z0-9]* #[0-9]+|^[ \t]+'; }; then
            HAS_FOOTER=true
        fi
        
        # Append footer appropriately
        if [ "$HAS_FOOTER" = true ]; then
            # Already has footer, append without blank line
            # Ensure the file ends with newline before appending
            [ -n "$(tail -c1 "$COMMIT_MSG_FILE")" ] && printf '\n' >> "$COMMIT_MSG_FILE"
            printf '%s' "$GITLEAKS_FOOTER" >> "$COMMIT_MSG_FILE"
        else
            # No footer yet, add blank line separator then footer
            # Ensure the file ends with newline before adding separator
            [ -n "$(tail -c1 "$COMMIT_MSG_FILE")" ] && printf '\n' >> "$COMMIT_MSG_FILE"
            printf '\n%s' "$GITLEAKS_FOOTER" >> "$COMMIT_MSG_FILE"
        fi
    fi
fi

exit 0

