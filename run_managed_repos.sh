#!/bin/bash

# Configuration
# Assuming a directory structure where banquet, mksqlite, and sqliter are siblings of flight2 (in ../)
# Adjust BASE_DIR if your directory structure is different.
# We use 'cd' to verify the path relative to this script location or absolute paths.
# Since this script is likely in flight2/, the parent dir is one level up.

BASE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
REPOS=("banquet" "mksqlite" "sqliter")

echo "Starting test run for managed repositories..."
echo "Base Directory: $BASE_DIR"

for REPO in "${REPOS[@]}"; do
    REPO_PATH="$BASE_DIR/$REPO"
    echo -e "\n=================================================="
    echo "Processing $REPO..."
    echo "Path: $REPO_PATH"
    
    if [ ! -d "$REPO_PATH" ]; then
        echo "❌ Directory not found: $REPO_PATH"
        continue
    fi

    # Run in a subshell to avoid changing the script's working directory permanently
    (
        cd "$REPO_PATH" || exit

        echo "Running tests ($REPO)..."
        if go test ./...; then
            echo "✅ Tests Passed for $REPO"
            
            # Check if there are changes to commit
            # We check both staged and unstaged changes
            if [[ -n $(git status --porcelain) ]]; then
                echo "Changes detected. Proceeding to commit and push..."
                
                # Git add, commit, push
                git add .
                git commit -m "Auto-commit: Tests passed $(date '+%Y-%m-%d %H:%M:%S')"
                
                echo "Pushing changes..."
                if git push; then
                    echo "🚀 Successfully pushed $REPO"
                else
                    echo "❌ Failed to push $REPO"
                fi
            else
                echo "ℹ️ No changes to commit for $REPO"
            fi
        else
            echo "❌ Tests Failed for $REPO. Aborting commit/push."
        fi
    )
done

echo -e "\n=================================================="
echo "All operations completed."
