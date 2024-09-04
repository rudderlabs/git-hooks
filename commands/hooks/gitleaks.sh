#!/bin/sh

# Pre-commit hook to run Gitleaks on staged changes

# Run Gitleaks
gitleaks protect --no-banner --staged . 

# Check the exit code of Gitleaks
if [ $? -ne 0 ]; then
    echo "Gitleaks has detected potential secrets in your changes."
    echo "Please run:"
    echo "\`\`\`\n    go run github.com/zricethezav/gitleaks/v8@v8.18.4 protect --no-banner --staged -v . \n\n\`\`\`"
    echo "to see the Gitleaks output above and remove any sensitive information before committing."
    exit 1
fi

# If Gitleaks passes, allow the commit
exit 0
