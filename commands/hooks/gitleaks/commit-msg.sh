#!/bin/sh

# Commit-msg hook to append gitleaks version info

# Gitleaks path (detected during installation)
GITLEAKS_PATH="{{.GitleaksPath}}"

COMMIT_MSG_FILE=$1

GITLEAKS_PHRASE="🔒 Scanned for secrets using gitleaks"

# Append gitleaks scan info to commit message
if [ -n "$COMMIT_MSG_FILE" ]; then
    # Check if the commit message already contains gitleaks scan info
    if grep -q "$GITLEAKS_PHRASE" "$COMMIT_MSG_FILE"; then
        echo "Gitleaks scan info already present in commit message, skipping"
    else
        # Get gitleaks version with error handling
        GITLEAKS_VERSION=$($GITLEAKS_PATH version 2>/dev/null | head -n1)
        if [ $? -eq 0 ] && [ -n "$GITLEAKS_VERSION" ]; then
            echo "" >> "$COMMIT_MSG_FILE"
            echo "$GITLEAKS_PHRASE $GITLEAKS_VERSION" >> "$COMMIT_MSG_FILE"
        else
            echo "Warning: Failed to get gitleaks version, appending scan info without version" >&2
            echo "" >> "$COMMIT_MSG_FILE"
            echo "$GITLEAKS_PHRASE" >> "$COMMIT_MSG_FILE"
        fi
    fi
fi

exit 0

