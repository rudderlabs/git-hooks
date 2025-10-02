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
        GITLEAKS_VERSION=$($GITLEAKS_PATH version | head -n1)
        echo "" >> "$COMMIT_MSG_FILE"
        echo "## The following line was added automatically, please do not remove it" >> "$COMMIT_MSG_FILE"
        echo "$GITLEAKS_PHRASE $GITLEAKS_VERSION" >> "$COMMIT_MSG_FILE"
    fi
fi

exit 0

