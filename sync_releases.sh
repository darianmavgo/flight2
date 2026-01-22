#!/bin/bash

# List of repos to manage
REPOS=("banquet" "mksqlite" "sqliter" "TableTypeMaster")
# BASE_DIR should be the directory containing all the repos
# Script is in REPO/scripts/, so "$(dirname "$0")/../.." is the directory above REPO.
BASE_DIR=/Users/darianhickman/Documents/

echo "🚀 Starting release synchronization..."

for repo in "${REPOS[@]}"; do
    echo "----------------------------"
    echo "📦 Checking $repo..."
    REPO_PATH="$BASE_DIR/$repo"
    
    if [ ! -d "$REPO_PATH" ]; then
        echo "⚠️  Repo directory not found at $REPO_PATH, skipping."
        continue
    fi

    cd "$REPO_PATH" || continue

    # Ensure we have the latest tags from remote
    git fetch --tags

    # Get latest tag
    LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null)
    if [ -z "$LATEST_TAG" ]; then
        echo "ℹ️  No tags found in $repo. Starting from v1.0.0"
        LATEST_TAG="v1.0.0"
        # If no tag exists, we check if there are any commits at all
        NEW_TAG="v1.0.0"
        HAS_CHANGES=true
    else
        # Check for commits since the last tag
        COMMITS_SINCE=$(git log "$LATEST_TAG..HEAD" --oneline)
        if [ -z "$COMMITS_SINCE" ]; then
            echo "✅ $repo is up to date (no changes since $LATEST_TAG)."
            HAS_CHANGES=false
        else
            echo "📝 Found changes in $repo since last release ($LATEST_TAG)."
            HAS_CHANGES=true
            
            # Increment patch version (assuming vX.Y.Z format)
            VERSION=${LATEST_TAG#v}
            IFS='.' read -r major minor patch <<< "$VERSION"
            NEW_TAG="v$major.$minor.$((patch + 1))"
        fi
    fi

    if [ "$HAS_CHANGES" = true ]; then
        echo "✨ Creating new release $NEW_TAG for $repo..."
        
        # Tag and push
        if git tag "$NEW_TAG" && git push origin "$NEW_TAG"; then
            # Create GitHub release using gh CLI
            gh release create "$NEW_TAG" --generate-notes --title "Release $NEW_TAG"
            if [ $? -eq 0 ]; then
                echo "🎉 Successfully released $repo $NEW_TAG"
            else
                echo "❌ Failed to create GitHub release for $repo"
            fi
        else
            echo "❌ Failed to push tag $NEW_TAG for $repo"
        fi
    fi
done

echo "----------------------------"
echo "🛠️  Updating flight2 dependencies..."
FLIGHT2_PATH="$BASE_DIR/flight2"

if [ -d "$FLIGHT2_PATH" ]; then
    cd "$FLIGHT2_PATH" || exit
    for repo in "${REPOS[@]}"; do
        echo "🔄 Fetching latest github.com/darianmavgo/$repo..."
        go get "github.com/darianmavgo/$repo@latest"
    done
    go mod tidy
    echo "✅ flight2 updated successfully."
else
    echo "⚠️  flight2 not found at $FLIGHT2_PATH, skipping update."
fi

echo "----------------------------"
echo "🏁 Sync complete."
