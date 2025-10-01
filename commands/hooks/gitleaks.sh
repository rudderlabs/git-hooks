#!/bin/sh

# Pre-commit hook to run Gitleaks on staged changes

# Gitleaks path (detected during installation)
GITLEAKS_PATH="{{.GitleaksPath}}"

# Run Gitleaks
$GITLEAKS_PATH protect --no-banner --staged .

# Check the exit code of Gitleaks
if [ $? -ne 0 ]; then
    echo "Gitleaks has detected potential secrets in your changes."
    echo "Please run:"
    echo "\`\`\`\n    $GITLEAKS_PATH protect --no-banner --staged -v . \n\n\`\`\`"
    echo "to see the Gitleaks output above and remove any sensitive information before committing."
    exit 1
fi

# If Gitleaks passes, prepare commit message with version info
GITLEAKS_VERSION=$($GITLEAKS_PATH version 2>/dev/null || echo "unknown")
COMMIT_MSG_FILE=".git/COMMIT_EDITMSG"

if [ -f "$COMMIT_MSG_FILE" ]; then
    echo "" >> "$COMMIT_MSG_FILE"
    echo "# Scanned by gitleaks $GITLEAKS_VERSION" >> "$COMMIT_MSG_FILE"
fi

# If Gitleaks passes, allow the commit
exit 0
